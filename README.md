# mcpollama — servidor MCP em Go para o Ollama da CSL

> Servidor **MCP (Model Context Protocol)** escrito em **Go**, que expõe o Ollama
> self-hosted da CSL (`qwen3-ia`) como ferramentas para o **Claude**. O Claude é o
> **orquestrador**: decide, planeja e verifica. O trabalho mecânico em volume é
> **delegado ao modelo local**, que custa **zero** da cota Anthropic.
>
> Estado (2026-10-28): **Fases 1 a 6 concluídas** (7 tools implementadas e validadas no
> Claude Code; economia medida na Fase 6) e **Fase 7 (documentos `.docx`, `.pptx` e `.pdf`)
> concluída**, com o OCR de PDF digitalizado e de imagens (etapa 7.9) **concluído no código**
> (opt-in; o binário e a configuração nos clientes seguem com o loop principal). Os
> fontes ficam em `src/`; o catálogo das tools está em `TOOLS.md`.

---

## 1. Objetivo

Reduzir o consumo de tokens Anthropic nas sessões de **Claude Code** (CLI e extensão
VS Code) e **Cursor**. Tarefas simples e verificáveis que hoje gastam contexto do
Claude passam para o Ollama, pela rede local:

| Fica no Claude (orquestrador) | Vai para o Ollama via `mcpollama` |
| --- | --- |
| decisões, arquitetura, código final | resumir/extrair de arquivos e logs |
| verificação crítica, revisão adversarial | coleta e leitura em volume |
| síntese final e conversa com o owner | tradução, conversão de formato, rascunhos |
| — | leitura de dados sensíveis (não saem da LAN) |
| — | embeddings (busca semântica / RAG) |

**Onde está a economia de verdade.** O ganho não vem só de "outro modelo responde".
Vem principalmente de o **servidor ler os arquivos do lado dele** e devolver ao Claude
apenas o resultado compacto (resumo ou extração). O conteúdo bruto nunca entra no
contexto do Claude. Uma tool que só recebe um prompt já montado pelo Claude economiza
pouco, porque o Claude teria de ler o arquivo antes.

As tools de arquivo leem texto, **`.docx`, `.pptx` e `.pdf` com camada de texto** (Fase 7; ver
a seção "Documentos" no §3). PDF digitalizado (sem texto) e imagens `.png`/`.jpg` só são lidos com o OCR ligado
(seção "OCR" no §3); sem ele, são recusados.

Este servidor é a evolução, via protocolo MCP, do nível **`local`** que hoje é chamado
por script (`%USERPROFILE%\.claude\scripts\ia-local.ps1`, via OpenClaude headless):
o Claude passa a chamar o modelo local como uma tool nativa, em vez de um comando de shell.

## 2. Integração Cursor / VS Code + Claude

```
 ┌──────────────────────────── estação Windows (dev) ────────────────────────────┐
 │  Claude Code (CLI / extensão VS Code)        Cursor (agente)                  │
 │        │  stdio (JSON-RPC 2.0 / MCP)               │  stdio                   │
 │        └──────────────┬────────────────────────────┘                          │
 │                 mcpollama-mcp.exe  ── lê arquivos locais (ex.: summarize_file)│
 └───────────────────────┬───────────────────────────────────────────────────────┘
                         │ HTTPS + Bearer (LAN: :8443 · internet: Cloudflare Tunnel)
             ┌───────────▼────────────┐
             │ nginx → ollama :11434  │  ia.cslsolucoes.com.br (10.100.2.2)
             │ qwen3-ia · 4× RTX 3070 │  100% GPU, num_ctx 65536 (MoE 30B; 150 W por GPU)
             └────────────────────────┘
```

O mesmo binário serve aos três clientes. Muda apenas o arquivo de registro:

| Cliente | Arquivo | Chave raiz |
| --- | --- | --- |
| Claude Code (projeto) | `.mcp.json` na raiz (**fora do git**) | `mcpServers` |
| Claude Code (usuário) | `claude mcp add` / `~/.claude.json` | `mcpServers` |
| Cursor | `.cursor/mcp.json` (projeto) ou `~/.cursor/mcp.json` (global) | `mcpServers` |
| VS Code (MCP nativo / Copilot) | `.vscode/mcp.json` | `servers` |

Exemplo (Claude Code / Cursor), no mesmo molde do `mcpvmware-mcp` e do `TruenasMcp`:

```json
{
  "mcpServers": {
    "mcpOllama": {
      "command": "D:\\MCP\\mcpollama-mcp.exe",
      "args": ["--ollama-url", "https://ia.cslsolucoes.com.br:8443",
               "--tls-ca", "D:\\Users\\<user>\\.openclaude\\certs\\cloudflare-origin-ca.pem",
               "--allow-root", "E:\\MCPOllam",
               "--api-key", "<token — ver nota de segurança>"]
    }
  }
}
```

> **Segurança:** o Bearer token **nunca** vai para o git, logs ou chat. A fonte está
> no servidor (`/root/API-Key`, 2ª linha) e já existe uma cópia local para o OpenClaude
> em `%USERPROFILE%\.openclaude\csl-ollama.env`. Por decisão do owner (2026-10-26), o
> token vai como **argumento** (`--api-key`) nas configs dos clientes. Cuidado: `args`
> aparecem na lista de processos da máquina. `.mcp.json`, `.cursor/mcp.json` e
> `.vscode/mcp.json` com o token ficam no `.gitignore`. O executável fica em
> **`D:\MCP\`**, junto dos outros servidores MCP da CSL, e não na pasta do projeto.

## 3. Arquitetura alvo (molde MCPVMWare / MCPNas)

```
mcpollama-mcp.exe         executável na RAIZ (gerado de src/, não versionado)
src/
  go.mod                  módulo github.com/cslsoftwares/mcpollama
  mcpollama-mcp/main.go   entrypoint stdio: flags → env, cria o cliente, registra as tools
  mcp/types.go            tipos JSON-RPC 2.0 + MCP (sem SDK externo, igual aos de referência)
  ollama/                 cliente HTTP do Ollama (/api/chat, /api/generate, /api/embed, /api/tags, /api/ps)
  tools/                  uma tool por arquivo + *_test.go ao lado
```

**Configuração (implementada na Fase 1; fonte da verdade: `src/mcpollama-mcp/config.go`).** Cada flag tem fallback para uma variável de ambiente:

| Flag | Env | Default |
| --- | --- | --- |
| `--ollama-url` | `OLLAMA_HOST` | — (sem default; o erro vem por chamada, a partir da Fase 2) |
| `--api-key` | `OLLAMA_API_KEY` | — (Bearer do nginx; nas configs dos clientes vai como argumento, decisão do owner; `args` aparecem na lista de processos) |
| `--tls-ca` | `OLLAMA_TLS_CA` | — (PEM da Cloudflare Origin CA) |
| `--insecure` | `OLLAMA_INSECURE` | `false` (gera WARNING a cada start) |
| `--model` | `OLLAMA_MODEL` | `qwen3-ia` |
| `--embed-model` | `OLLAMA_EMBED_MODEL` | `nomic-embed-text` |
| `--allowed-models` | `MCPOLLAMA_ALLOWED_MODELS` | `qwen3-ia;nomic-embed-text` (só os modelos otimizados para as GPUs do servidor; outro modelo exige incluí-lo aqui) |
| `--timeout` | `OLLAMA_TIMEOUT` | `120s` (decisão P-7; gerações longas com modelo frio podem estourar e recebem erro claro) |
| `--max-prompt-chars` | `MCPOLLAMA_MAX_PROMPT_CHARS` | `128000` (~48k tokens) |
| `--max-file-bytes` | `MCPOLLAMA_MAX_FILE_BYTES` | `4194304` |
| `--max-doc-bytes` | `MCPOLLAMA_MAX_DOC_BYTES` | `33554432` (32 MiB; teto em bytes de `.docx`, `.pptx` e `.pdf`, em vez do `--max-file-bytes`) |
| `--pdf-cache-dir` | `MCPOLLAMA_PDF_CACHE_DIR` | — (diretório **absoluto**, criado se não existir, para o cache de compilação do motor de PDF; sem ele nada é gravado em disco e o 1º PDF demora alguns segundos a mais) |
| `--max-line-bytes` | `MCPOLLAMA_MAX_LINE_BYTES` | `8388608` (linha de stdin; sem o teto de 64 KB do `bufio.Scanner`) |
| `--ocr-lib` | `MCPOLLAMA_OCR_LIB` | — (caminho **absoluto** de `libtesseract64.dll`; liga o OCR de páginas de PDF sem texto e de imagens `.png`/`.jpg`, só Windows; sem ele não há OCR e nada muda) |
| `--ocr-lib-sha256` | `MCPOLLAMA_OCR_LIB_SHA256` | — (obrigatório com `--ocr-lib`: SHA-256 da DLL; qualquer problema de OCR na partida deixa o servidor subir sem OCR, com WARNING no stderr) |
| `--ocr-tessdata` | `MCPOLLAMA_OCR_TESSDATA` | `<pasta da DLL>\tessdata\best` (diretório **absoluto** dos modelos) |
| `--ocr-workers` | `MCPOLLAMA_OCR_WORKERS` | `min(CPUs/2, 4)` (processos de OCR; 1 a 16) |
| `--ocr-page-timeout` | `MCPOLLAMA_OCR_PAGE_TIMEOUT` | `1m0s` (por página) |
| `--ocr-max-pages` | `MCPOLLAMA_OCR_MAX_PAGES` | `30` (páginas enviadas ao OCR por chamada) |
| `--ocr-max-mem` | `MCPOLLAMA_OCR_MAX_MEM` | `1073741824` (1 GiB por processo de OCR, via Job Object do Windows) |
| `--ocr-max-megapixels` | `MCPOLLAMA_OCR_MAX_MEGAPIXELS` | `40` (pixels por página; o dpi é reduzido até caber) |
| `--ocr-dpi` | `MCPOLLAMA_OCR_DPI` | `300` (resolução de renderização da página) |
| `--allow-root` | `MCPOLLAMA_ALLOW_ROOT` | — (sem ele, as tools de arquivo não existem) |
| `--max-chunks` | `MCPOLLAMA_MAX_CHUNKS` | `40` (teto de chamadas do map-reduce por arquivo) |
| `--tools-verbosity` | `MCPOLLAMA_TOOLS_VERBOSITY` | `full` (`compact` reduz o custo do `tools/list`) |
| `--debug` | `MCPOLLAMA_DEBUG` | `false` (só metadados; nunca argumentos nem o token) |
| `--version` | — | imprime a versão (injetada por `-ldflags "-X main.Version=..."`) |

**Tools.** O catálogo gerado do código fica em [`TOOLS.md`](TOOLS.md) (`go run ./cmd/dumptools`; um teste falha se divergir).
- **Base (Fase 3, implementadas):** `list_models`, `generate`, `chat` e `embed` (com `nomic-embed-text`; resumo por padrão e `include_vectors` opcional). Todas `readOnlyHint:true` e `openWorldHint:false`. A fila de GPU atende uma chamada por vez.
- **Orientadas a economia (Fase 4, implementadas):** `summarize_file`, `extract` e `translate` leem o arquivo **no servidor**, a partir de um `path`, e devolvem só o resultado. Elas só existem com `--allow-root`, e a leitura recusa:
  - caminho fora da raiz, inclusive via symlink, junction ou **hardlink**;
  - nomes da denylist fixa (`.env*`, chaves, certificados, `.mcp.json`, `*credential*`, `*secret*`, `.git/`...);
  - caminhos especiais do Windows (UNC, `\\?\`, ADS, dispositivos);
  - binário, arquivo grande demais e diretório.

  Arquivo acima do teto vira map-reduce automático (`--max-chunks`). Nada é gravado em disco.
- **Contadores de uso:** todo resultado termina com uma linha `[uso] modelo=… prompt_eval_count=… eval_count=… total_ms=… load_ms=… queue_ms=…`, também presente em `_meta`.
- **Sem tools administrativas.** O nginx devolve **403** em `/api/pull|push|create|delete|copy|blobs`.

**Documentos (Fase 7).** `summarize_file`, `extract` e `translate` também aceitam `.docx`, `.pptx` e `.pdf` com camada de texto. O formato é decidido por extensão **e** assinatura do arquivo. O texto sai com marcadores que dizem de onde veio cada trecho:

- **PDF:** `[página N]`.
- **PPTX:** `[slide N]`, `[slide N — oculto]` e `[notas do slide N]`.
- **DOCX:** `[cabeçalho]`, `[rodapé]`, `[nota de rodapé N]`, `[nota de fim N]`, `[comentário]` e `[quebra de página]`.
- **DOCX e PPTX:** `[oculto]` precede o texto marcado como oculto.

A numeração automática de cláusulas do Word (1., 1.1, a), I.) sai preservada no texto. Revisões pendentes entram já aceitas.

**Rodapé técnico.** Além do `[arquivo]` comum, a linha termina com campos que dizem o quanto confiar na resposta:

| Campo | Significado |
| --- | --- |
| `formato=` | `docx`, `pptx`, `pdf`, `png`, `jpg` ou `txt` (sempre presente) |
| `paginas=` e `paginas_sem_texto=` | só PDF: total de páginas e quantas não tinham camada de texto |
| `slides=` | só PPTX: total de slides |
| `revisoes_pendentes=` | só DOCX: `sim` se o arquivo tem alterações controladas não resolvidas, `nao` caso contrário |
| `numeracao=indisponivel` | só aparece quando a numeração automática não pôde ser carregada |
| `conteudo_importado=sim` | só aparece quando o DOCX importa conteúdo externo (`altChunk`) |
| `texto_oculto=sim` | só aparece quando há texto oculto (DOCX/PPTX), rotulado `[oculto]` |
| `paginas_ocr=`, `conf_ocr_media=` e `paginas_ocr_falhas=` | só quando o OCR rodou: páginas (ou 1 imagem) lidas por OCR, confiança média (0 a 100) e páginas cujo OCR falhou |
| `paginas_ocr_nao_processadas=` | só quando o tempo acabou antes de todas as páginas (valor maior que 0) |

Quando parte do conteúdo não pôde ser lida, vem logo depois a linha `[aviso] parte do conteúdo não pôde ser lida`. O `translate` com documento devolve a tradução **como texto** (nada é gravado) e avisa com `[aviso] tradução devolvida como texto; nenhum documento foi gravado`.

**Recusas (sem chamar o modelo):**

- `.doc`, `.ppt` e `.xls` (formato 97-2003: salvar como `.docx`/`.pptx`);
- documento protegido por senha de abertura;
- PDF sem camada de texto (digitalizado) e imagens `.png`/`.jpg`, quando o OCR não está ligado (`--ocr-lib`);
- documento acima de `--max-doc-bytes`, ou vazio.

**Limites.** 32 MiB por documento (`--max-doc-bytes`). O teto de texto continua sendo `--max-prompt-chars` × `--max-chunks` (map-reduce, como nos demais arquivos). O motor de PDF tem teto próprio de memória de **512 MiB**.

**Motor de PDF.** go-pdfium **v1.21.4** (licença MIT) em WebAssembly sobre o wazero **v1.12.0** (Apache-2.0), **sem cgo**. DOCX e PPTX usam só a biblioteca padrão do Go. O cache de compilação do WASM só existe com `--pdf-cache-dir`: o 1º PDF do processo leva ~0,6 s com o cache preenchido e ~8 s sem ele. O binário passou de ~10,9 MB para ~22,5 MB.

**Limitações conhecidas:**

- tabelas de PDF não têm garantia de fidelidade;
- texto invisível de PDF não é rotulado como oculto;
- o hífen de quebra silábica é mantido ("con-trato");
- o `.pptx` foi validado com 2 apresentações reais;
- a denylist casa por prefixo ou sufixo do nome inteiro (`prod.env.pdf` e `chave.pem.docx` não são barrados); política aceita pelo owner.

**OCR (PDF digitalizado e imagens).** Etapa 7.9, concluída no código.

- **Opt-in.** Só liga com `--ocr-lib` **e** `--ocr-lib-sha256`. Sem as flags, o PDF digitalizado é recusado como antes e `.png`/`.jpg` também. Se as flags estiverem erradas (caminho relativo, hash divergente, DLL ou modelos ausentes, DLL que não carrega), o servidor sobe sem OCR e escreve `WARNING: OCR disabled: ...` no stderr.
- **Motor.** Tesseract 5.3.3 (DLL `D:\MCP\libtesseract64.dll`, carregada sem cgo) e modelos em `D:\MCP\tessdata\{best,fast}`; o `best` é o padrão (`--ocr-tessdata` troca). Idioma `por`.
- **Isolamento.** O OCR roda em processos filhos do próprio executável (modo interno `--ocr-worker`, fora do `--help`), cada um num Job Object com limite de memória (`--ocr-max-mem`). O worker não recebe o token nem `--api-key`; o servidor segue de pé se um filho morrer.
- **O que passa por OCR.** Só as páginas de PDF **sem texto** (menos de 20 caracteres); páginas com texto não mudam. Imagens: `.png` e `.jpg`/`.jpeg` (extensão e assinatura), convertidas para cinza (transparência vira branco). Páginas em branco não vão ao motor.
- **Marcadores.**

  | Marcador | Quando |
  | --- | --- |
  | `[página N — OCR conf=93]` | página lida por OCR |
  | `[página N — OCR baixa confiança conf=41]` | confiança abaixo de 60 |
  | `... girada=180]` no fim | a 2ª passada a 180° rendeu mais |
  | `[página N — OCR falhou]` | falha naquela página; o documento segue |
  | `[página N — OCR não processado: tempo esgotado]` | o orçamento de tempo acabou |
  | `[imagem — OCR conf=93]` | imagem (mesmas variantes de baixa confiança e girada) |

- **Rodapé e aviso.** O rodapé ganha `paginas_ocr=`, `conf_ocr_media=` e `paginas_ocr_falhas=` (e `paginas_ocr_nao_processadas=` se maior que 0). Logo depois dele vem sempre `[aviso] texto obtido por OCR: números, datas, nomes e CPF/CNPJ podem conter erros de reconhecimento; confira os valores críticos no original`.
- **Limites.** `--ocr-max-pages` (30): acima disso o documento é recusado ("divida o documento"; páginas em branco não contam). `--ocr-max-megapixels` (40): página grande tem o dpi reduzido até caber; imagem acima de 2 vezes esse teto de pixels, ou de ~256 MiB estimados de decodificação, é recusada. `--ocr-max-mem` (1 GiB por processo). `--ocr-page-timeout` (1 min por página). O orçamento total vem do `--timeout` da chamada: ao acabar, as páginas restantes ganham o marcador de tempo esgotado. Se nada for reconhecido, a chamada é recusada.
- **Desempenho medido.** Com `best`, ~4,5 a 6,7 s por página na geração de teste (1 worker, A4 a 300 dpi). No documento real de 12 páginas, com 4 workers, ~1,6 s por página (18,8 s no total, confiança média 90). Fora o OCR, o 1º PDF do processo ainda paga a inicialização do PDFium (~7 a 8 s sem `--pdf-cache-dir`).
- **Limitações.**
  - Não há detecção de orientação: só existe uma 2ª passada a 180°; páginas giradas a 90° ou 270° não são corrigidas.
  - Linhas de tabela sobre fundo sombreado podem sumir (medido: 4 de 11 linhas numa tabela de preços, incluindo o total).
  - Números, valores, datas, nomes e CPF/CNPJ precisam de conferência no original (por isso o `[aviso]`).
  - Só Windows. Sem cache de OCR em disco.

## 4. Servidor Ollama (alvo)

| Item | Valor |
| --- | --- |
| Host | `ia.cslsolucoes.com.br` = `10.100.2.2` (CentOS Stream 10, 4× RTX 3070 8 GB) |
| Ollama | v0.40.8 · modelo **`qwen3-ia`** = MoE `qwen3-coder:30b-a3b` (100% GPU; ~97 tok/s de geração e ~5.000 tok/s de prefill, medidos em 2026-10-23); embeddings `nomic-embed-text` (768 dimensões); também `qwen3.8:27b-q4_K_M` e `qwen3.8:27b-64k` (nível `local`) |
| Contexto | **4 GPUs, `num_ctx` 65536 e 150 W por GPU** (2026-10-26). 4 GPUs com 131072 derrubou a máquina (5/5); 170 W caiu em ~5 min de carga máxima contínua; 150 W aguentou 10 min. O link PCIe da GPU `03:00.0` (porta `00:1b.4`) acumula erros corrigíveis e aguarda reparo. Mantém 2 modelos residentes (`OLLAMA_MAX_LOADED_MODELS=2`). Ver `.workspace/reports/MCPOllam2026-10-26-122500-teste-stress.report.md` |
| Na LAN | `https://ia.cslsolucoes.com.br:8443` (porta explícita; cert Cloudflare Origin CA → `--tls-ca` ou `-k`) |
| Pela internet | `https://ia.cslsolucoes.com.br` (Cloudflare Tunnel, sem porta) |
| Auth | `Authorization: Bearer <token>` → `200` válido · `401` sem token · `403` endpoint bloqueado |
| Guia de operação | `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md` |

## 5. Referências

### Web
1. **Ollama MCP Server (dxeo)**: <https://lobehub.com/mcp/dxeo-ollama-mcp>.
   Servidor MCP em Go que roteia tarefas para modelos Ollama locais, com modelo de
   raciocínio e de embeddings separados. Configura por `REASONING_MODEL`,
   `EMBEDDING_MODEL`, `CONTEXT_SIZE` e `OLLAMA_HOST`.
2. **MCP.so (diretório de servidores)**: <https://mcp.so/servers>.
   Catálogo de servidores MCP por categoria. Serve para descobrir implementações e padrões.
3. **NightTrek/Ollama-mcp (mcpservers.org)**: <https://mcpservers.org/pt-BR/servers/NightTrek/Ollama-mcp>.
   A página deu HTTP 403 na coleta de 2026-10-23; o README do repositório
   (<https://github.com/NightTrek/Ollama-mcp>) foi lido: servidor Node/TypeScript, sem leitura
   de arquivo (brief `Web/W3`).
4. **Ollama vs LocalAI (Contabo)**: <https://contabo.com/blog/ollama-vs-localai-best-self-hosted-openai-compatible-llm-server/?utm_source=google&utm_medium=cpc&utm_campaign=brand-pmax-global&utm_term=&utm_content=&gad_source=1&gad_campaignid=23403612477&gbraid=0AAAAAD_Qy-d_waeLRQTVzJwVB2ar-CtRP&gclid=CjwKCAjwi-zWBhAdEiwAqlRf-Bs04Kk4H8xq7S358PcF3R8vs_QSSFDiNbingREwWzzFAPT5VCVGWxoCbukQAvD_BwE>.
   Compara servidores LLM self-hosted com API compatível com a OpenAI (`/v1/` na
   porta 11434) e recomenda TLS + proxy de token na frente, como o nginx da CSL.
5. **MCP com Ollama (Apidog)**: <https://apidog.com/pt/blog/mcp-ollama/>.
   Tutorial do Dolphin MCP (Python): o Dolphin é o cliente MCP, e o Ollama aparece
   só como provedor do modelo (brief `Web/W5`). Útil como contraste.
6. **KTCrisis (GitHub)**: <https://github.com/KTCrisis>.
   Autor do `ollama-mcp-go` (cópia em `referencias/ollama-mcp-go`) e da suíte
   `flux7-*` (mesh/proxy de guardrails, supervisor com LLM plugável Ollama/Claude,
   memória multiagente).
7. **ollama-tlms-golang (GitHub)**: <https://github.com/ollama-tlms-golang>.
   Série de estudo sobre Ollama + modelos pequenos + Go: saída JSON, RAG, raciocínio,
   "make your MCP server", host MCP (cópia de `04-what-is-mcp` em `referencias/`).

### Locais (`referencias/`)

Síntese detalhada de todas as referências (locais, web e servidor) em
[`referencias/Brief_Referencias/`](referencias/Brief_Referencias/). Plano do brief em
`.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md`.

| Pasta | O que aproveitar |
| --- | --- |
| `ollama-mcp-go/` | Implementação Go mínima (stdio, `generate`/`chat`/`embed`, streaming interno com trace em arquivo) |
| `MCPVMWare/` | Projeto Go/MCP da CSL: layout `src/`, flags→env, `--tools-verbosity compact`, `--profile`, gate `--allow-destructive` |
| `MCPNas/truenas-mcp/` | Projeto Go/MCP da CSL: `--tls-ca`, `--insecure`, proxy stdio, tasks |
| `04-what-is-mcp/` | Exemplo didático de servidor MCP em Go (ollama-tlms-golang) |

> ⚠️ `referencias/` contém configs com credenciais reais (ex.: `MCPVMWare/.mcp.json`).
> Não commitar nem copiar trechos dessa pasta sem revisar.
