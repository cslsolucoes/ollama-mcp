# mcpollama

> Servidor **MCP (Model Context Protocol)** em **Go** que expõe o Ollama self-hosted da CSL
> (`qwen3.8:27b`) como ferramentas para o **Claude Code** e o **Cursor**, mais o **gateway**
> (fila + `/monitor`) que protege e observa esse Ollama. O Claude é o **orquestrador**; o trabalho
> mecânico em volume vai para o modelo local, que custa **zero** da cota Anthropic.
>
> Repositório privado: <https://github.com/cslsolucoes/ollama-mcp> · Uso interno da CSL.

## Índice

1. [O que é](#1-o-que-é)
2. [Estado atual](#2-estado-atual)
3. [As tools MCP](#3-as-tools-mcp)
4. [Uso no Claude Code](#4-uso-no-claude-code)
5. [Configuração do mcpollama](#5-configuração-do-mcpollama)
6. [O gateway: fila e `/monitor`](#6-o-gateway-fila-e-monitor)
7. [Servidor de IA](#7-servidor-de-ia)
8. [Instalação e atualização](#8-instalação-e-atualização)
9. [Desenvolvimento](#9-desenvolvimento)
10. [Segurança e sigilo](#10-segurança-e-sigilo)
11. [Documentação e planos](#11-documentação-e-planos)
12. [Referências e licença](#12-referências-e-licença)

---

## 1. O que é

O **mcpollama** é um servidor MCP (stdio, JSON-RPC 2.0) que roda na estação do desenvolvedor. O
Claude Code ou o Cursor o chama como uma tool nativa, e ele fala com o Ollama da CSL pela rede.
Tarefas simples e verificáveis (resumir, extrair, traduzir, rascunhar, gerar embeddings, ler dados
sensíveis que não podem sair da LAN) deixam de gastar o contexto do Claude.

```
Claude Code (CLI / VS Code)      Cursor
          │  stdio (MCP)            │
          └───────────┬─────────────┘
              mcpollama-mcp.exe      lê os arquivos localmente (summarize_file, extract, translate)
                      │  HTTPS + Bearer   LAN :8443  ·  internet: Cloudflare Tunnel
                      ▼
      nginx :8443   TLS, Bearer, limites por IP, bloqueio de /api/pull|push|create|delete|copy|blobs
                      ▼
      mcpollama-gateway 127.0.0.1:11500   fila com prioridade e justiça, políticas, SQLite, /monitor
                      ▼
      Ollama 127.0.0.1:11434   qwen3.8:27b · nomic-embed-text   (4× RTX 3070, 100% GPU)
```

**De onde vem a economia.** Uma tool que só recebe um prompt pronto economiza pouco, porque o Claude
já teria lido o arquivo. Aqui o **servidor lê o arquivo do lado dele** e devolve só o resultado
compacto: o texto bruto nunca entra no contexto do Claude. Medido na Fase 6, resumir um arquivo de
~72 mil caracteres com `summarize_file` custou **−66% de custo Anthropic** e deixou **~22 mil
tokens fora do contexto**, com ~15 s a mais de espera
([report](.workspace/reports/MCPOllam2026-10-24-173500-economia-tokens.report.md)).

| Fica no Claude (orquestrador) | Vai para o Ollama via mcpollama |
| --- | --- |
| decisões, arquitetura, código final | resumir e extrair de arquivos e logs |
| verificação crítica, revisão adversarial | coleta e leitura em volume |
| síntese final e conversa com o owner | tradução, conversão de formato, rascunhos |
| — | leitura de dados sensíveis (não saem da LAN) |
| — | embeddings (busca semântica) |

O mcpollama é a evolução, via MCP, do nível `local` antigo (`%USERPROFILE%\.claude\scripts\ia-local.ps1`,
OpenClaude headless).

## 2. Estado atual

Em produção (2026-10-30):

| Componente | Versão (tag) | Onde |
| --- | --- | --- |
| `mcpollama-mcp` | **0.9.2** (`mcpollama-v0.9.2`, `0.9.2-f367b35`; sessões reiniciadas em 2026-11-01 00:11). Rótulo padrão `<máquina>/<usuário>` + cabeçalho `X-Mcpollama-Station` (máquina, usuário, domínio, MAC via WMI) | `D:\MCP\mcpollama-mcp.exe` (estações) |
| `mcpollama-gateway` | **0.13.2** (`0.13.2-bb51ed6`, implantado em 2026-11-05; antes, `0.12.6-a155513` desde 2026-11-01), com o pool de backends; `--max-wait 30m`; fila 64 por cliente/IP e 128 no total; o painel não trava mais quando uma leitura do banco empaca | `10.100.2.2`, `/usr/local/bin/mcpollama-gateway` |

Tags no repositório: `mcpollama-v0.8.1`, `mcpollama-v0.9.0`, `mcpollama-v0.9.1`, `mcpollama-v0.9.2`, `gateway-v0.11.1`, `gateway-v0.12.0` … `gateway-v0.12.6` (a release 0.13.2 e o mcpollama 0.10.1 foram implantados em 2026-11-05). O histórico de versões está no
[CHANGELOG.md](CHANGELOG.md). O **fallback do mcpollama** (seção 5, desde o `0.9.0`) segue no binário atual, **desligado por padrão**.

| Fase | Assunto | Estado |
| --- | --- | --- |
| 1 | Esqueleto do protocolo MCP (sem SDK externo) | concluída |
| 2 | Cliente Ollama (stream interno, TLS, mapa de erros, redação do token) | concluída |
| 3 | Tools base: `list_models`, `generate`, `chat`, `embed` | concluída |
| 4 | Tools de arquivo: `summarize_file`, `extract`, `translate` (guard em `src/files/`) | concluída |
| 5 | Integração com os clientes (foco no Claude Code, decisão de 2026-10-24) | concluída |
| 6 | Medição da economia (−66% de custo Anthropic) | concluída |
| 7 | Documentos `.docx`, `.pptx`, `.pdf` e OCR (PDF digitalizado, `.png`, `.jpg`) | concluída (OCR opt-in) |
| 8 | Bake-off de modelos: `qwen3-ia` = `qwen3.5:9b`; argumento `think` por chamada | concluída |
| 9 | Gateway: fila, políticas, `/monitor`, SQLite, instalador | concluída (2026-10-30) |
| 10 | Log de jobs, pool de backends e fallback (faixa CPU descartada) | **10.1 a 10.8 concluídas; plano congelado em 2026-10-30**; 10.S (secundário) pendente, sem data |
| 11 | Túnel WebSocket e jobs assíncronos ([plano](.workspace/plans/congelados/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md)) | **congelada em 2026-10-30 23:50** (decisão do owner); só o spike 11.1 foi feito ([relatório](.workspace/reports/MCPOllam2026-10-30-234600-fase11-1-spike-espera-longa.report.md)); nenhum código |
| 12 | Erros do modelo local, base de treino, melhora sem treino ([plano](.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md), [contrato](docs/CONTRATO-DADOS-TREINO.md)) | **implantada** (gateway 0.13.0 e mcpollama 0.10.0 em 2026-11-04; release de ajuste implantada em 2026-11-05: gateway 0.13.2, mcpollama 0.10.1; ver [MANUAL, seção 17](docs/MANUAL-GATEWAY.md#17-erros-do-modelo-e-base-de-treino-fase-12)) |

**Fase 10** ([plano](.workspace/plans/congelados/MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md), congelado):

| Etapa | Estado |
| --- | --- |
| 10.1 e 10.2: medição da CPU, interferência na GPU e energia | concluídas (qwen3-ia na CPU: ~4,6 tok/s de geração, ~25 tok/s de prefill; saldo de CPU ~0; interferência de -4% a -10% na GPU; pico de energia 392 W) |
| 10.3: decisão sobre a faixa CPU | **faixa CPU descartada** (decisão do owner); só o pool pronto para secundário |
| **10.L**: log de jobs com conteúdo completo | **em produção** (gateway 0.11.1), revisada adversarialmente |
| 10.4: pool de backends (registro, saúde, disjuntor, repasse por sobrecarga, drenar) e fallback do mcpollama | **concluída** (merge `736c205`; o ramo `wip/fase10-4-pool` foi removido) |
| 10.5: revisão adversarial do pool | **concluída** (correções `b1d8418`) |
| 10.6: implantação do pool | **concluída**: gateway `0.12.0` em produção desde 2026-10-30 19:43, com um único backend (`gpu-local`) |
| 10.7: aceite em instância de teste | **concluída** (todos os cenários PASS); correções pós-aceite em `4a54c8e` e `c4031f7` |
| 10.8: implantação 0.12.1/0.9.0, documentação final e congelamento | **concluída** (gateway `0.12.1-c4031f7` em produção às 20:46; depois atualizado até o `0.12.3` às 22:35; [relatório final](.workspace/reports/MCPOllam2026-10-30-205000-fase10-final.report.md)) |
| 10.S: secundário 4× RTX 2060 | planejado, **sem data** |

Pendências sem data: segunda fonte para o servidor, backup do `content.db` (excluir ou cifrar) e
divisão da RAM do secundário.

## 3. As tools MCP

Sete tools, todas `readOnlyHint:true` e `openWorldHint:false`. O catálogo completo (esquemas JSON) está
em [TOOLS.md](TOOLS.md), gerado do registro por `go run ./cmd/dumptools` (um teste falha se divergir).
Os nomes das tools são contrato: renomear é quebra de compatibilidade.

| Tool | O que faz |
| --- | --- |
| `list_models` | lista os modelos permitidos instalados e os residentes na VRAM; não carrega modelo nem entra na fila |
| `generate` | roda um prompt avulso no modelo local (rascunhos, reescritas, conversão) |
| `chat` | próxima mensagem de uma conversa; o servidor não guarda histórico, mande a conversa inteira |
| `embed` | embeddings com `nomic-embed-text` (768 dim); devolve um resumo, e os vetores só com `include_vectors` |
| `summarize_file` | resume um arquivo lido **no servidor**; arquivo grande vira map-reduce automático |
| `extract` | extrai do arquivo o que a instrução pede, no formato pedido (JSON, tabela) |
| `translate` | traduz o arquivo preservando estrutura e identificadores; documento volta como texto, nada é gravado |

- As três tools de arquivo só existem com `--allow-root`. A leitura recusa caminho fora da raiz (inclusive por
  symlink, junction ou hardlink), nomes da denylist (`.env*`, chaves, certificados, `.mcp.json`, `.git/`...),
  caminhos especiais do Windows (UNC, `\\?\`, ADS, dispositivos), binário e arquivo grande demais.
- **Formatos:** texto UTF-8, `.docx`, `.pptx`, `.pdf` com camada de texto e, com o OCR ligado, PDF
  digitalizado, `.png` e `.jpg`/`.jpeg`. `.doc`, `.ppt`, `.xls` e documentos protegidos por senha são recusados.
  O texto sai com marcadores de origem (`[página N]`, `[slide N]`, `[cabeçalho]`, `[OCR conf=93]`...).
- **OCR** (opt-in, só Windows): Tesseract 5.3.3 em processos filhos isolados (Job Object com limite de memória),
  ligado por `--ocr-lib` + `--ocr-lib-sha256`. Sem as flags, o PDF digitalizado segue recusado. O resultado
  traz sempre o `[aviso]` de que números, datas, nomes e CPF/CNPJ devem ser conferidos no original.
- **`think`:** `generate`, `chat`, `extract`, `summarize_file` e `translate` aceitam `think` por chamada. O padrão
  é `false` (rápido); `true` faz o modelo raciocinar antes (mais lento, para análise difícil).
- **Linha `[uso]`:** toda resposta termina com
  `[uso] modelo=… prompt_eval_count=… eval_count=… total_ms=… load_ms=… queue_ms=…`, também em `_meta`. As tools
  de arquivo acrescentam ao rodapé campos como `formato=`, `paginas=` e `paginas_ocr=`. O `queue_ms` mostra
  quanto o pedido esperou na fila do gateway.
- **Limites do modelo:** contas e contagens diretas são pouco confiáveis; peça passo a passo ou uma lista em
  JSON e confira por fora. Valores literais e resumos são fiéis ao arquivo.
- **Sem tools administrativas:** o nginx devolve 403 em `/api/pull|push|create|delete|copy|blobs`.

## 4. Uso no Claude Code

O executável fica em **`D:\MCP\mcpollama-mcp.exe`**, junto dos outros servidores MCP da CSL. **Depois de
trocar o binário, reinicie a sessão do Claude Code** (o `go build` renomeia o `.exe` em uso para `.exe~`;
apague-o depois de reiniciar).

Registro por cliente:

| Cliente | Arquivo | Chave raiz |
| --- | --- | --- |
| Claude Code (projeto) | `.mcp.json` na raiz (**fora do git**) | `mcpServers` |
| Claude Code (usuário) | `claude mcp add` / `~/.claude.json` | `mcpServers` |
| Cursor | `.cursor/mcp.json` (projeto) ou `~/.cursor/mcp.json` | `mcpServers` |
| VS Code (MCP nativo) | `.vscode/mcp.json` | `servers` |

Exemplo de `.mcp.json` (troque os placeholders; **nunca** commite o arquivo preenchido):

```json
{
  "mcpServers": {
    "mcpOllama": {
      "command": "D:\\MCP\\mcpollama-mcp.exe",
      "args": [
        "--ollama-url", "https://ia.cslsolucoes.com.br:8443",
        "--tls-ca", "<CAMINHO>\\.openclaude\\certs\\cloudflare-origin-ca.pem",
        "--allow-root", "<PASTA-DO-PROJETO>",
        "--api-key", "<TOKEN>"
      ]
    }
  }
}
```

- O token vai como argumento `--api-key` (decisão do owner de 2026-10-26). Os `args` aparecem na lista de
  processos da máquina; por isso o `.mcp.json` fica no `.gitignore`.
- A CA da Cloudflare Origin não é confiada pelo Windows: `--tls-ca` soma o PEM ao pool do sistema.
- Para ver o servidor funcionando sem cliente (smoke test do protocolo):

```powershell
D:\MCP\mcpollama-mcp.exe --version
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}',
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | D:\MCP\mcpollama-mcp.exe --ollama-url https://ia.cslsolucoes.com.br:8443 --tls-ca <PEM>
```

## 5. Configuração do mcpollama

Cada flag tem fallback para variável de ambiente (ordem: flag, env, padrão). Fonte da verdade:
[`src/mcpollama-mcp/config.go`](src/mcpollama-mcp/config.go). A lista completa, com as flags de OCR e as de
diagnóstico, está no [CLAUDE.md](CLAUDE.md).

| Flag | Env | Padrão |
| --- | --- | --- |
| `--ollama-url` | `OLLAMA_HOST` | sem padrão (o erro vem por chamada) |
| `--api-key` | `OLLAMA_API_KEY` | sem padrão (Bearer do nginx) |
| `--tls-ca` | `OLLAMA_TLS_CA` | sem padrão (PEM da Cloudflare Origin CA) |
| `--insecure` | `OLLAMA_INSECURE` | `false` (WARNING a cada partida) |
| `--model` | `OLLAMA_MODEL` | `qwen3.8:27b` |
| `--embed-model` | `OLLAMA_EMBED_MODEL` | `nomic-embed-text` |
| `--allowed-models` | `MCPOLLAMA_ALLOWED_MODELS` | `qwen3.8:27b;nomic-embed-text` (só modelos otimizados para o hardware) |
| `--timeout` | `OLLAMA_TIMEOUT` | `30m` desde a 0.10.1 (antes `120s`; limite efetivo de cada tool, fila + execução; o gateway espera na fila até `--max-wait 30m`) |
| `--allow-root` | `MCPOLLAMA_ALLOW_ROOT` | sem padrão (sem ele, as tools de arquivo não existem) |
| `--max-prompt-chars` | `MCPOLLAMA_MAX_PROMPT_CHARS` | `128000` |
| `--max-file-bytes` | `MCPOLLAMA_MAX_FILE_BYTES` | `4194304` |
| `--max-doc-bytes` | `MCPOLLAMA_MAX_DOC_BYTES` | `33554432` (32 MiB; `.docx/.pptx/.pdf`) |
| `--max-chunks` | `MCPOLLAMA_MAX_CHUNKS` | `40` (teto de chamadas do map-reduce por arquivo) |
| `--pdf-cache-dir` | `MCPOLLAMA_PDF_CACHE_DIR` | sem padrão (absoluto; sem ele nada é gravado e o 1º PDF leva ~8 s a mais) |
| `--ocr-lib` e `--ocr-lib-sha256` | `MCPOLLAMA_OCR_LIB`, `MCPOLLAMA_OCR_LIB_SHA256` | sem padrão (juntas, ligam o OCR) |
| `--client-label` | `MCPOLLAMA_CLIENT_LABEL` | `<máquina>/<usuário>` (rótulo que aparece no `/monitor`; sem usuário, `<máquina>/<pid>`) |
| `--tools-verbosity` | `MCPOLLAMA_TOOLS_VERBOSITY` | `full` (`compact` encurta o `tools/list`) |
| `--ollama-url-fallback` | `OLLAMA_HOST_FALLBACK` | sem padrão: **desligado**. Segundo servidor, tentado **uma vez** quando o primário dá erro de conexão ou 503 de fila cheia; a linha `[uso]` ganha `backend=fallback` |
| `--fallback-api-key` | `OLLAMA_FALLBACK_API_KEY` | sem padrão (Bearer **do fallback**; o token do primário nunca vai para ele; só com `https://` ou loopback) |
| `--fallback-tls-ca` | `OLLAMA_FALLBACK_TLS_CA` | sem padrão (PEM do fallback, somado ao pool do sistema; o `--insecure` não vale para ele) |
| `--debug` | `MCPOLLAMA_DEBUG` | `false` (só metadados, nunca argumentos nem token) |

stdout é exclusivo do JSON-RPC; log e debug vão para stderr. O servidor não grava em disco a não ser que uma
flag peça.

## 6. O gateway: fila e `/monitor`

O Ollama processa **um pedido por vez** com o `qwen3.8:27b`, e a fila interna dele é invisível. O
`mcpollama-gateway` (código em `src/gateway/` e `src/cmd/mcpollama-gateway/`) é uma **fila visível, justa e
auditável** na frente do Ollama, com uma página para ver e mexer nela. Para os clientes nada muda: mesma URL,
mesmo Bearer, mesmas rotas. O Ollama escuta só em `127.0.0.1`, então ninguém contorna a fila.

**O que faz**

| Tema | Comportamento |
| --- | --- |
| Faixas | `geracao` (`/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions`; 1 slot) e `embed` (2 slots) |
| Prioridade | `alta`, `normal` (padrão) e `baixa`, pelo header `X-Mcpollama-Priority`. `alta` só vale para IPs de `--priority-ips` (hoje a LAN `10.100.2.0/24`); de outro IP vira `normal` sem erro. `baixa` que espera 5 min passa a `normal` |
| Justiça | dentro da faixa: prioridade, fixados do admin, rodízio por **IP** (o IP vem do nginx e não se forja), depois por rótulo (`X-Mcpollama-Client`) |
| Limites | fila total 128 e 64 por IP/rótulo (acima: **503** com `Retry-After`); espera máxima 10 min (**504**); execução máxima 10 min; no nginx, 600 pedidos/min (burst 100) e 80 simultâneos por IP (**429**) |
| Raciocínio | desligado por padrão. Liga com `think:true`, header `X-Mcpollama-Think: on` ou o nome `qwen3.8:27b-think` (nome virtual, só do gateway). No `/v1`, injeta `reasoning_effort:"none"` |
| Opções | `keep_alive`, `num_ctx` e `num_gpu`/`main_gpu`/`low_vram`/`use_mmap`/`use_mlock`/`numa` do cliente são removidos em silêncio, para não recarregar o modelo (~20 s) |
| Modelos | só os de `--allowed-models`; outro dá **403** |
| Cliente mcpollama | manda `X-Mcpollama-Client` e `X-Mcpollama-Tool`, lê `X-Queue-Ms` e repete **uma vez** em 503 com `Retry-After` (não repete 409) |

**O `/monitor`** — `https://ia.cslsolucoes.com.br:8443/monitor`, **só da LAN** (pelo túnel da Cloudflare dá 403).
Ver não exige login; as ações exigem o usuário `admin`. Atualiza ao vivo por SSE a cada 1 s.

| Quadro | Conteúdo |
| --- | --- |
| Cartões | em execução, na fila, concluídos e erros do dia, espera p50/p95, tokens/s, pedidos/min |
| Em execução | quem, tool, modelo, raciocínio, tempos |
| Fila de espera | logo abaixo de Em execução: prioridade, ordem simulada de início e botões ▲ ▼ ✕ numa linha |
| Concluídos nos últimos 5 min | seção própria: cada pedido que terminou, com status, pico de W e de clock, e botão **Detalhes** |
| GPUs | potência em uso e limite aplicado, memória, uso, temperatura, contador AER |
| CPU / RAM | uso por thread e por núcleo, frequência, temperatura, carga, RAM e swap |
| Modelos | carregados na VRAM e instalados, com o que é permitido |
| Abas | Concluídos recentes, Eventos e **Log em tempo real** |

Ordem do painel (0.12.3): Em execução, Fila de espera, Concluídos nos últimos 5 min, Backends, GPUs, CPU/RAM, Modelos, Última
hora e abas. O painel é responsivo (só as tabelas rolam, dentro do `.wrap`), as tabelas se atualizam no lugar (rolagem e cliques
preservados) e erros de desenho aparecem em `#msg`.

- **Ações do admin** (com login, CSRF e auditoria que nunca expira): promover, despromover, remover (o cliente recebe
  409) e cancelar um pedido em execução.
- **Log em tempo real (10.L):** cada job aparece ao entrar na fila e segue até o estado final. **Detalhes** mostra os
  metadados, o **extrato das GPUs segundo a segundo** (W e clock, com gráficos) e, **só para o admin logado**, o
  prompt e a resposta completos. **Exportar log** baixa JSONL ou CSV (até 7 dias; o conteúdo só com sessão de admin).
- **Banco SQLite:** `gateway.db` (metadados: pedidos 90 dias, amostras de GPU/CPU 7 dias, auditoria, eventos) e
  `content.db` (texto dos jobs, 7 dias). O que sobrevive a reinício ou reboot fica marcado como
  `interrompido_reboot` ou `interrompido_gateway`.

**Pool de backends (Fase 10.4 a 10.6, em produção desde a `0.12.0`; `0.12.6` desde 2026-11-01).** O gateway tem um **registro de backends**
(`backends.json`, modo 0600, lido só na partida), saúde ativa e passiva, disjuntor, histerese e "drenar" no monitor, e repassa a
geração que **ainda não começou** a um Ollama secundário da LAN quando o primário está sobrecarregado ou doente. **Hoje o pool
tem um único backend, o `gpu-local`** (o Ollama do próprio servidor, sem `--backends`): o comportamento é o mesmo de antes do
pool. O mcpollama ganhou um **fallback opcional** (seção 5), **desligado por padrão**, para o caso de o servidor inteiro cair. A
faixa CPU foi **descartada** (medição 10.1/10.2: saldo ~0). O secundário planejado é um PC com 4× RTX 2060, **sem data**.
A revisão adversarial do pool (10.5) e o aceite em instância de teste (10.7) estão feitos; o plano da Fase 10 está congelado.

Detalhes (instalação, as flags `MCPGW_*`, API do monitor, SQLite, diagnóstico, rollback e segurança):
[docs/MANUAL-GATEWAY.md](docs/MANUAL-GATEWAY.md).

## 7. Servidor de IA

| Item | Valor |
| --- | --- |
| Host | `ia.cslsolucoes.com.br` (`lx-ai01`, CentOS Stream 10), Ollama v0.40.13 (desde 2026-10-31; antes 0.40.8) |
| Hardware | 4× RTX 3070 (8 GB cada, 32 GB de VRAM), **100 W por GPU**, clock travado em 210–1400 MHz desde 2026-10-30 |
| Limite conhecido | a **fonte de 850 W** não aguenta as 4 placas (causa dos reboots); os 100 W e o clock travado valem até a troca. A segunda fonte está planejada |
| Modelo | **`qwen3.8:27b`** (nome real, sem alias; arquitetura `qwen35`, 27,3B denso, Q4_K_M, 17 GB em disco, `num_ctx` 65536 com 66/66 camadas na GPU: ~19,6 GB + nomic ≈ 24 GB dos 32 GB), desde a release mcpollama 0.10.1 / gateway 0.13.2 da Fase 12 (2026-11-05; **implantado em 2026-11-05 (gateway 0.13.2, mcpollama 0.10.1)**). Antes: `qwen3-ia` = alias do `qwen3.5:9b` (desde 2026-10-29). Raciocina por padrão; o mcpollama manda `think:false` e o gateway também. Contas e contagens: pedir passo a passo ou ligar `think` (nas medições do bake-off, conta direta 0/15 sem `think` e 15/15 com `think`) |
| Embeddings | `nomic-embed-text` (768 dim), residente junto |
| Concorrência | o Ollama roda essa arquitetura com **1 slot**; `NUM_PARALLEL=1` e gateway `--gen-slots 1` (`--run-timeout 10m`) desde 2026-11-04 (a arquitetura `qwen35` não paraleliza: o Ollama avisa "does not currently support parallel requests"), `MAX_QUEUE=32`; a fila de verdade é a do gateway |
| Carga medida (Fase 8.9, com o `qwen3.5:9b`) | 10 agentes × 32k de contexto, 0 erros, **~700–790 tok/s somados** (~2.000 com documentos grandes) |
| Carga a frio | ~33 s com o `qwen3.8:27b` (era ~16–22 s com o `qwen3.5:9b`) |

**Acesso**

- **LAN:** `https://ia.cslsolucoes.com.br:8443` (porta explícita). Use `--tls-ca` com a Cloudflare Origin CA.
- **Internet:** o mesmo host sem porta (Cloudflare Tunnel); de dentro da LAN esse caminho dá timeout (DNS split-horizon).
- **Autenticação:** `Authorization: Bearer <TOKEN>`; `200` com token válido, `401` sem token, `403` em endpoint bloqueado.
  O token vem de `/root/API-Key` no servidor e **nunca** entra no repositório.
- **`/monitor`:** só a LAN `10.100.2.0/24` o enxerga.
- Guia de operação (SSH, GPUs, nginx): `CONEXAO-IA.md` no repositório de operação do owner (fora deste repositório).
  Muitas conexões SSH seguidas ao servidor acionam o fail2ban.

## 8. Instalação e atualização

> **Guia completo do zero:** para montar o servidor de IA, o gateway, as estações e os clientes do começo, ou depois de um desastre, siga o
> **[docs/INSTALACAO.md](docs/INSTALACAO.md)** (passo a passo com comandos, saída esperada e conferência). Os arquivos de referência do servidor estão em
> [`deploy/servidor-ia/`](deploy/servidor-ia/README.md). O resumo abaixo cobre só a instalação e a atualização do dia a dia.

**mcpollama (estação Windows).** Os fontes ficam em `src/`; o executável vai para `D:\MCP`. Binários não são versionados.

```powershell
cd E:\MCPOllam\src
go build -ldflags "-X main.Version=<versao>" -o D:\MCP\mcpollama-mcp.exe ./mcpollama-mcp    # Windows
$env:GOOS='linux'; go build -o D:\MCP\mcpollama-mcp ./mcpollama-mcp; Remove-Item Env:GOOS    # Linux
```

Depois, **reinicie a sessão do Claude Code** e confira com `D:\MCP\mcpollama-mcp.exe --version`. Para o OCR, a DLL
(`libtesseract64.dll`) e os modelos (`tessdata\{best,fast}`) ficam em `D:\MCP` e entram pelas flags `--ocr-*`.

**mcpollama-gateway (servidor).** Pelo instalador, que roda no PC (precisa de `go`, `git`, `ssh.exe`, `scp.exe` e do alias
SSH `lx-ai01-root`):

```powershell
cd E:\MCPOllam\deploy\gateway
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1                  # instalação completa
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Atualizar   # só troca o binário e reinicia o gateway
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Senha       # (re)define a senha do admin
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Preparar     # só a etapa 1 (usuário, pastas, binário)
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Reverter -Backup /root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS
```

| Modo | Muda o tráfego? |
| --- | --- |
| `Completo` | **sim** (etapa 2: Ollama só no loopback, nginx para o gateway; reinicia o Ollama) |
| `Preparar` | não |
| `Atualizar` | não (reinicia só o gateway; corta quem está em execução, então olhe o `/monitor` antes) |
| `Senha` | não; reinicie o gateway depois, porque o arquivo só é lido na partida |
| `Reverter` | **sim** (volta ao estado anterior) |

O instalador gera o binário Linux, monta o pacote com LF e o manifesto `SHA256SUMS`, envia para `/root/deploy96`
(root, 0700), confere os hashes e roda as etapas. O token e a senha nunca passam pela linha de comando.

**Rollback.** Se a etapa 2 falhar depois de mexer no nginx ou no Ollama, a volta é automática. Para voltar à mão, use
`-Modo Reverter` com a pasta de backup `fase96-ativacao-*` (para o estado **anterior ao gateway**, a mais antiga) ou, no
servidor, `bash <B>/rollback.sh <B>`. Ele devolve o nginx direto ao Ollama, o Ollama à LAN e desliga o gateway; não
apaga usuário, binário, unit, senha nem banco. Para voltar só o binário, veja a seção 2.4 do MANUAL.

## 9. Desenvolvimento

**Layout do repositório**

```
README.md  CHANGELOG.md  NOTICE.md  CLAUDE.md  TOOLS.md   visão geral, versões, aviso de uso, guia do Claude Code, catálogo das tools
docs/                      INSTALACAO.md, MANUAL-GATEWAY.md e o índice (README.md)
deploy/gateway/            instalador, unit do systemd, ollama.conf do nginx e scripts de etapa do gateway
deploy/servidor-ia/        cópias das configurações do servidor de IA (sem segredos)
src/                       código Go (módulo único)
.workspace/                planos e reports históricos
```

`TOOLS.md` fica na raiz de propósito: é gerado por `src/cmd/dumptools` e conferido por `TestCatalogInSync`.

**Layout de `src/`** (módulo Go `github.com/cslsoftwares/mcpollama`, `go 1.26.0`; toolchain local go1.27.0)

| Pasta | Conteúdo |
| --- | --- |
| `mcpollama-mcp/` | entrypoint stdio: flags, env, cliente, registro das tools (`config.go`, `main.go`) |
| `mcp/` | tipos JSON-RPC 2.0 e MCP escritos à mão (sem SDK externo) |
| `ollama/` | cliente HTTP do Ollama (`/api/*`), TLS, mapa de erros |
| `tools/` | uma tool (ou família) por arquivo, com `*_test.go` ao lado |
| `files/` | guard de leitura de arquivos (raiz, denylist, symlink/hardlink) |
| `doctext/` | extração de `.docx`, `.pptx` e `.pdf` (go-pdfium em WASM/wazero, sem cgo) |
| `ocr/` | OCR de PDF digitalizado e imagens (Tesseract em processos filhos) |
| `redact/` | redação do token em mensagens e logs |
| `gateway/` | `queue`, `proxy`, `pool`, `store`, `monitor`, `gpu`, `cpu`, `models`, `content`, `joblog` |
| `cmd/` | `mcpollama-gateway`, `dumptools` (gera o `TOOLS.md`) e `bakeoff` |

**Comandos** (dentro de `src/`)

```powershell
go build ./...                      # compila tudo
go vet ./...
go test ./...                       # suíte completa
go test -count=1 ./...              # sem cache de teste (validação "limpa")
go test ./tools -run TestNome -v    # um teste
# detector de corrida: precisa de cgo; usa o gcc que já está em deps\ (só na sessão)
$env:PATH="E:\MCPOllam\deps\gcc-mingw-w64\bin;$env:PATH"; $env:CC="gcc"; $env:CGO_ENABLED='1'; go test -race -count=1 ./...; Remove-Item Env:CC,Env:CGO_ENABLED
# testes ao vivo (build tag live): uma chamada de cada vez
go test -tags live -run TestLiveN ./ollama        # ou ./tools
# testes do OCR com a DLL real
$env:MCPOLLAMA_TEST_OCR_LIB='<caminho da libtesseract64.dll>'; $env:MCPOLLAMA_TEST_OCR_TESSDATA='<pasta tessdata>'; go test -tags ocr ./...
# catálogo das tools
go run ./cmd/dumptools
```

**Regra dos testes de ataque.** Mexeu em `src/files`, `src/doctext` ou `src/gateway`? Rode de novo os testes de
ataque da revisão adversarial correspondente antes de dar por pronto:

| Área | Testes de ataque |
| --- | --- |
| `src/files/` | [`...fase4-arquivos.testes`](.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.testes) |
| `src/doctext/` | [`...revisao-adversarial-fase7.testes`](.workspace/reports/MCPOllam2026-10-28-161147-revisao-adversarial-fase7.testes) |
| `src/gateway/` | [`...revisao-adversarial-fase9.testes`](.workspace/reports/MCPOllam2026-10-28-235633-revisao-adversarial-fase9.testes), [`...revisao-adversarial-10L.testes`](.workspace/reports/MCPOllam2026-10-30-173000-revisao-adversarial-10L.testes) e [`...revisao-adversarial-10-5-pool.testes`](.workspace/reports/MCPOllam2026-10-30-190200-revisao-adversarial-10-5-pool.testes) |
| `src/ocr/` | `go test -tags ocr ./...` com a DLL real |

**Convenções**

- Um commit por fase, quando verde (build, vet, testes); stage **por caminho**, sem `git add -A`; nunca commitar com FAIL.
- Mensagens no estilo `feat:`, `fix:`, `docs:`, `deploy:`, `merge:`, com o número da fase (`Fase 10.L - ...`).
- Cada release leva uma tag anotada: `mcpollama-v<versão>` e `gateway-v<versão>`.
- Planos e reports vão em `.workspace/plans/` e `.workspace/reports/` (nome `MCPOllam<AAAA-MM-DD-hhmmss>-<assunto>`),
  e o plano é atualizado **a cada fase**: o que deu certo, o que não deu, a solução e as pendências numeradas.
- O pack `.cursor/` é a fonte única dos espelhos `.claude/`, `.vscode/` e similares; não edite os espelhos.

## 10. Segurança e sigilo

- **Token:** só por argumento (`--api-key`) ou variável de ambiente, nunca em código, teste, log, chat ou commit. O
  `.mcp.json` (e `.cursor/mcp.json`, `.vscode/mcp.json`) com o token fica fora do git. O token do cliente não é
  repassado ao Ollama nem gravado pelo gateway.
- **`referencias/`:** material de terceiros e de outros projetos, com credenciais reais. É **somente leitura** e
  **não versionada**; só o `referencias/Brief_Referencias/` (síntese própria, 0 segredos) entra no git.
- **Binários** (`*.exe`, `/deps/`, `/sdk/`) e o corpus de PDFs de clientes não são versionados.
- **O log de jobs grava prompt e resposta completos (decisão do owner, 2026-10-30).** Fica só no `content.db`
  (`/var/lib/mcpollama-gateway/content.db`), **7 dias**, modo **0600**, dono `mcpgw`, **visível só para o admin logado**
  no `/monitor`. Isso inclui documentos e dados de clientes enviados por `extract`, `summarize_file` e `translate`:
  trate o servidor e os backups dessa pasta como dado sigiloso. Desligar: `--log-content=false`. Embeddings guardam só
  contagem e dimensão.
- **Rede:** o `/monitor` é bloqueado para a internet e para o túnel da Cloudflare (403 com `CF-Connecting-IP`); o
  Ollama escuta só no loopback; endpoints administrativos do Ollama dão 403.
- **Revisões adversariais feitas:**

| Revisão | Resultado | Report |
| --- | --- | --- |
| Fase 4 (arquivos) | escape crítico por hardlink achado e corrigido | [report](.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md) |
| Fase 7 (documentos) | 0 ESCAPOU; 2 achados Altos de disponibilidade (memória), corrigidos | [report](.workspace/reports/MCPOllam2026-10-28-161147-revisao-adversarial-fase7.report.md) |
| Fase 9.5 (gateway) | 30 vetores, 0 Altos, todos os achados corrigidos | [report](.workspace/reports/MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md) |
| 10.L (log de jobs) | 0 Altos, 5 Médios e 5 Baixos, todos corrigidos | [report](.workspace/reports/MCPOllam2026-10-30-173000-revisao-adversarial-10L.report.md) |

Riscos residuais conhecidos estão na seção 12 do [MANUAL](docs/MANUAL-GATEWAY.md) (por exemplo: o rótulo do
cliente é livre, há um único administrador compartilhado e a LAN inteira pode pedir prioridade `alta`).

## 11. Documentação e planos

| Documento | Para quê |
| --- | --- |
| [CLAUDE.md](CLAUDE.md) | estado das fases, servidor, configuração completa, comandos e segredos |
| [TOOLS.md](TOOLS.md) | catálogo das 7 tools (esquemas), gerado do código |
| [docs/MANUAL-GATEWAY.md](docs/MANUAL-GATEWAY.md) | operação do gateway: instalação, flags, fila, políticas, monitor, API, SQLite, diagnóstico, rollback, segurança |
| [deploy/gateway/README.md](deploy/gateway/README.md) | instalador e arquivos de `deploy/` |
| [docs/INSTALACAO.md](docs/INSTALACAO.md) | guia completo de instalação do zero: servidor, gateway, estação e clientes |
| [docs/README.md](docs/README.md) e [deploy/README.md](deploy/README.md) | índices das pastas `docs/` e `deploy/` |
| [CHANGELOG.md](CHANGELOG.md) | histórico de versões do mcpollama e do mcpollama-gateway |
| [NOTICE.md](NOTICE.md) | aviso de uso proprietário e componentes de terceiros |
| [deploy/servidor-ia/](deploy/servidor-ia/README.md) | cópias dos arquivos do servidor de IA (Ollama, powercap, nginx, cloudflared), coletadas em 2026-10-30 |
| [Plano de implementação (Fases 0 a 6)](.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md) | decisões e changelog das fases iniciais |
| [Plano da Fase 7 (extração de documentos)](.workspace/plans/MCPOllam2026-10-28-132000-extracao-docx-pdf-pptx.brief.md) | brief de `.docx`, `.pdf` e `.pptx` |
| [Plano da Fase 8 (bake-off)](.workspace/plans/MCPOllam2026-10-28-193200-fase8-bakeoff-modelos.plan.md) | escolha do modelo |
| [Plano da Fase 9 (fila e monitor)](.workspace/plans/MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md) | decisões D9-* e changelog |
| [Plano da Fase 10 (pool)](.workspace/plans/congelados/MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md) | congelado em 2026-10-30 |
| [Plano do brief de referências](.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md) | ondas B0 a B7 |

Reports principais (em [`.workspace/reports/`](.workspace/reports/)):

| Assunto | Report |
| --- | --- |
| Economia de tokens (Fase 6) | [economia-tokens](.workspace/reports/MCPOllam2026-10-24-173500-economia-tokens.report.md) |
| Teste de stress e integração | [teste-stress](.workspace/reports/MCPOllam2026-10-26-122500-teste-stress.report.md), [teste-integracao-claude-code](.workspace/reports/MCPOllam2026-10-26-141000-teste-integracao-claude-code.report.md) |
| Documentos e OCR (Fase 7) | [extração](.workspace/reports/MCPOllam2026-10-28-191530-fase7-extracao-documentos.report.md), [aceite ao vivo](.workspace/reports/MCPOllam2026-10-28-155000-fase7-ao-vivo-aceite.report.md), [OCR](.workspace/reports/MCPOllam2026-10-28-183855-fase7-9-ocr.report.md) |
| Modelo (Fase 8) | [bake-off](.workspace/reports/MCPOllam2026-10-28-220428-fase8-bakeoff-modelos.report.md), [carga multiagente](.workspace/reports/MCPOllam2026-10-28-235002-fase8-9-carga-multiagente.report.md) |
| Gateway (Fase 9) | [report final](.workspace/reports/MCPOllam2026-10-30-142900-fase9-fila-monitor-final.report.md) |
| Fase 10 | [relatório final](.workspace/reports/MCPOllam2026-10-30-205000-fase10-final.report.md), [medição da CPU](.workspace/reports/MCPOllam2026-10-30-161904-fase10-1-medicao-cpu.report.md) |

O brief das referências está em [`referencias/Brief_Referencias/`](referencias/Brief_Referencias/).

## 12. Referências e licença

**Web** (coletadas em 2026-10-23)

1. Ollama MCP Server (dxeo), servidor MCP em Go com modelos de raciocínio e de embeddings separados: <https://lobehub.com/mcp/dxeo-ollama-mcp>
2. MCP.so, diretório de servidores MCP: <https://mcp.so/servers>
3. NightTrek/Ollama-mcp (a página deu 403 na coleta; o README do repositório foi lido): <https://mcpservers.org/pt-BR/servers/NightTrek/Ollama-mcp> e <https://github.com/NightTrek/Ollama-mcp>
4. Ollama vs LocalAI, API compatível com a OpenAI e proxy de token na frente: <https://contabo.com/blog/ollama-vs-localai-best-self-hosted-openai-compatible-llm-server/>
5. MCP com Ollama via Dolphin MCP (o Dolphin é o cliente; o Ollama é só o provedor do modelo): <https://apidog.com/pt/blog/mcp-ollama/>
6. KTCrisis, autor do `ollama-mcp-go` e da suíte `flux7-*`: <https://github.com/KTCrisis>
7. ollama-tlms-golang, série de estudo sobre Ollama + Go + MCP: <https://github.com/ollama-tlms-golang>

**Locais** (`referencias/`, somente leitura, não versionadas): `ollama-mcp-go` (implementação mínima de MCP para Ollama),
`MCPVMWare` e `MCPNas/truenas-mcp` (moldes de projeto Go/MCP da CSL) e `04-what-is-mcp` (exemplo didático). Contêm
credenciais reais: não copie configs dali.

**Licença.** O repositório não tem arquivo de licença: é **software proprietário de uso interno da CSL**. Não redistribua.
Os avisos de uso e os componentes de terceiros estão no [NOTICE.md](NOTICE.md).
