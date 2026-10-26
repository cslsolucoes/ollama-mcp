# mcpollama — servidor MCP em Go para o Ollama da CSL

> Servidor **MCP (Model Context Protocol)** escrito em **Go**, que expõe o Ollama
> self-hosted da CSL (`qwen3-ia`) como ferramentas para o **Claude**. O Claude é o
> **orquestrador**: decide, planeja e verifica. O trabalho mecânico em volume é
> **delegado ao modelo local**, que custa **zero** da cota Anthropic.
>
> Estado: **concepção** (2026-10-23). Os fontes ficam em `src/` (criada, ainda vazia);
> esta página define o objetivo e a arquitetura alvo.

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
             │ qwen3-ia · 3× RTX 3070 │  100% GPU, num_ctx 65536 (MoE 30B; provisório)
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
      "command": "E:\\MCPOllam\\mcpollama-mcp.exe",
      "args": ["--ollama-url", "https://ia.cslsolucoes.com.br:8443",
               "--tls-ca", "C:\\Users\\<user>\\.openclaude\\certs\\cloudflare-origin-ca.pem"],
      "env": { "OLLAMA_API_KEY": "<token — ver nota de segurança>" }
    }
  }
}
```

> **Segurança:** o Bearer token **nunca** vai para o git, logs ou chat. A fonte está
> no servidor (`/root/API-Key`, 2ª linha) e já existe uma cópia local para o OpenClaude
> em `%USERPROFILE%\.openclaude\csl-ollama.env`. Quando possível, prefira variável de
> ambiente a `args`. `.mcp.json` com segredo fica no `.gitignore`.

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
| `--api-key` | `OLLAMA_API_KEY` | — (Bearer do nginx; prefira a env: `args` aparecem na lista de processos) |
| `--tls-ca` | `OLLAMA_TLS_CA` | — (PEM da Cloudflare Origin CA) |
| `--insecure` | `OLLAMA_INSECURE` | `false` (gera WARNING a cada start) |
| `--model` | `OLLAMA_MODEL` | `qwen3-ia` |
| `--embed-model` | `OLLAMA_EMBED_MODEL` | `nomic-embed-text` |
| `--allowed-models` | `MCPOLLAMA_ALLOWED_MODELS` | `qwen3-ia;nomic-embed-text` (só os modelos otimizados para as GPUs do servidor; outro modelo exige incluí-lo aqui) |
| `--timeout` | `OLLAMA_TIMEOUT` | `120s` (decisão P-7; gerações longas com modelo frio podem estourar e recebem erro claro) |
| `--max-prompt-chars` | `MCPOLLAMA_MAX_PROMPT_CHARS` | `128000` (~48k tokens) |
| `--max-file-bytes` | `MCPOLLAMA_MAX_FILE_BYTES` | `4194304` |
| `--max-line-bytes` | `MCPOLLAMA_MAX_LINE_BYTES` | `8388608` (linha de stdin; sem o teto de 64 KB do `bufio.Scanner`) |
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

## 4. Servidor Ollama (alvo)

| Item | Valor |
| --- | --- |
| Host | `ia.cslsolucoes.com.br` = `10.100.2.2` (CentOS Stream 10, 4× RTX 3070 8 GB) |
| Ollama | v0.40.8 · modelo **`qwen3-ia`** = MoE `qwen3-coder:30b-a3b` (100% GPU; ~97 tok/s de geração e ~5.000 tok/s de prefill, medidos em 2026-10-23); embeddings `nomic-embed-text` (768 dimensões); também `qwen3.8:27b-q4_K_M` e `qwen3.8:27b-64k` (nível `local`) |
| Contexto | **provisório: 3 GPUs e `num_ctx` 65536.** A GPU 3 está fora por falha de PCIe, que reiniciava a máquina com prompt grande (incidente mitigado em 2026-10-23). Volta a 4 GPUs e 131072 após o reparo. Ver `.workspace/reports/MCPOllam2026-10-23-191600-resposta-incidente-ollama-prompt-grande.report.md` |
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
