# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## O projeto

**mcpollama**: servidor **MCP em Go** (stdio, JSON-RPC 2.0) que expõe o Ollama self-hosted
da CSL (`qwen3-ia`) como tools para Claude Code (CLI/VS Code) e Cursor. O Claude **orquestra**;
o trabalho mecânico em volume (resumir, extrair, traduzir, rascunhos, embeddings, dados sensíveis)
vai para o modelo local, para **economizar tokens Anthropic**. Objetivo, configs de cliente e
links de referência estão no [README.md](README.md).

Princípio de design que guia as tools: a economia vem de o **servidor ler os arquivos do lado
dele** e devolver só o resultado compacto. Uma tool que exige o Claude já ter o conteúdo no
contexto economiza pouco.

Estado em 2026-10-23: **Fases 1 a 4 concluídas** (tools base `list_models`, `generate`,
`chat` e `embed`; tools de arquivo `summarize_file`, `extract` e `translate`, que só existem
com `--allow-root` e passam pelo guard de `src/files/`; catálogo em `TOOLS.md`, gerado por
`src/cmd/dumptools`). Qualquer mudança em `src/files/` exige rodar de novo os testes de
ataque da revisão adversarial
(`.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.*`). O protocolo
está em `src/` (`mcp/types.go`, `mcpollama-mcp/`); o cliente Ollama em `src/ollama/` (stream
interno, TLS `SystemCertPool`+CA, mapa de erros, redação do token via `src/redact/`). Testes ao
vivo ficam atrás da build tag `live` (`go test -tags live -run TestLiveN ./ollama` ou `./tools`,
uma chamada de cada vez).
**Fase 5:** o foco é o **Claude Code** (decisão do owner, 2026-10-24). O servidor se chama
**`mcpOllama`** no `.mcp.json` da raiz (escopo de projeto, fora do git; executável `D:\MCP\mcpollama-mcp.exe`; token como argumento
`--api-key`) e já
foi validado nele; Cursor (`.cursor/mcp.json`) e VS Code (`.vscode/mcp.json`) estão
configurados, mas a validação neles é opcional.
**Fase 6 (medição):** resumir um arquivo de ~72 mil caracteres com `summarize_file` custou
**−66% de custo Anthropic** e deixou **~22 mil tokens fora do contexto** do Claude, com ~15 s a
mais de espera (`.workspace/reports/MCPOllam2026-10-24-173500-economia-tokens.report.md`).
O Claude Code aceitou uma tool de ~87 s sem corte; o limite efetivo é o `--timeout` do servidor.
O layout abaixo é o alvo; requisitos por fase em
`referencias/Brief_Referencias/61-Matriz_Cobertura.md` §3.

---

## Início de sessão

### FASE 1 — Validação dos espelhos

```powershell
powershell -ExecutionPolicy Bypass -File ".cursor/scripts/bootstrap-mirror-symlinks.ps1" -ValidateOnly
```

Se falhar por falta de privilégios de Administrador: informar e **parar**.

### FASE 2 — Detecção do projeto Go

- Se existe `src/go.mod`: projeto ativo; seguir a seção **Comandos**.
- Se **não** existe: o scaffold ainda não foi feito. Montar conforme **Arquitetura alvo**
  (README §3), confirmando o âmbito com o owner.
- **Não** usar o wizard Delphi/FPC (`bootstrap-build-config.ps1`, `.dpr`/`.lpr`): o pack
  `.cursor/` veio de projetos Delphi, mas este projeto é Go.

---

## Comandos

Os fontes ficam em `src/`; o **executável fica em `D:\MCP\mcpollama-mcp.exe`** (junto dos outros
servidores MCP da CSL, decisão do owner de 2026-10-26), que é o caminho registrado nos clientes
MCP. Binários não são versionados. Se o servidor estiver aberto num cliente, o `go build` renomeia
o `.exe` em uso para `.exe~`; apague-o depois de reiniciar o cliente.

```powershell
# dentro de src/
go build ./...                                          # compila tudo
go build -ldflags "-X main.Version=<versao>" -o D:\MCP\mcpollama-mcp.exe ./mcpollama-mcp   # binário Windows → D:\MCP
$env:GOOS='linux'; go build -o D:\MCP\mcpollama-mcp ./mcpollama-mcp; Remove-Item Env:GOOS   # binário Linux → D:\MCP
go vet ./...
go test ./...                                           # suíte completa
go test ./tools -run TestNomeDoTeste -v                 # um teste
go test -count=1 ./...                                  # sem cache de teste (validação "limpa")
# detector de corrida: precisa de cgo; usa o gcc já presente em deps\ (só na sessão)
$env:PATH="E:\MCPOllam\deps\gcc-mingw-w64\bin;$env:PATH"; $env:CC="gcc"; $env:CGO_ENABLED='1'; go test -race -count=1 ./...; Remove-Item Env:CC,Env:CGO_ENABLED
```

Smoke test do protocolo (stdio), sem cliente MCP:

```powershell
D:\MCP\mcpollama-mcp.exe --version
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}',
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | D:\MCP\mcpollama-mcp.exe --ollama-url https://ia.cslsolucoes.com.br:8443 --tls-ca <pem>
```

Plano de implementação (fases, estado e changelog): `.workspace/plans/MCPOllam*-mcpollama-implementacao.plan.md`.
Atualizar o plano **a cada fase**.

Toolchain local: Go 1.26.5 (`C:\Program Files\Go\bin\go`).

---

## Arquitetura alvo (molde dos projetos Go/MCP da CSL)

Seguir o mesmo layout e as mesmas convenções de `referencias/MCPVMWare/src` e
`referencias/MCPNas/truenas-mcp`. Antes de criar um mecanismo novo, ver como eles já resolvem.

```
src/go.mod                  module github.com/cslsoftwares/mcpollama
src/mcpollama-mcp/main.go   entrypoint stdio: parse de flags → fallback env → cliente → registro de tools
src/mcp/types.go            tipos JSON-RPC 2.0 + MCP escritos à mão (sem SDK MCP externo)
src/ollama/                 cliente HTTP do Ollama (API nativa /api/*)
src/tools/                  uma tool (ou família) por arquivo, com *_test.go ao lado
```

- **Configuração:** flags com fallback para variável de ambiente, como em `TruenasMcp`/`mcpvmware-mcp`:
  `--ollama-url`/`OLLAMA_HOST`, `--api-key`/`OLLAMA_API_KEY` (nas configs dos clientes o token vai como argumento `--api-key`,
  decisão do owner de 2026-10-26; sem `--api-key-file`),
  `--tls-ca`/`OLLAMA_TLS_CA`, `--insecure`/`OLLAMA_INSECURE`, `--model`/`OLLAMA_MODEL`
  (default `qwen3-ia`), `--embed-model`/`OLLAMA_EMBED_MODEL` (default `nomic-embed-text`),
  `--allowed-models`/`MCPOLLAMA_ALLOWED_MODELS` (default fixo `qwen3-ia;nomic-embed-text`: **só
  modelos otimizados para o hardware**, decisão do owner de 2026-10-26; modelo fora da lista é
  recusado na partida e por chamada, sem chegar ao Ollama),
  `--timeout`/`OLLAMA_TIMEOUT` (default 120 s, decisão P-7; o Claude Code aceita tools de ~87 s ou mais),
  `--max-prompt-chars`/`MCPOLLAMA_MAX_PROMPT_CHARS` (default 128000),
  `--max-file-bytes`/`MCPOLLAMA_MAX_FILE_BYTES`, `--max-line-bytes`/`MCPOLLAMA_MAX_LINE_BYTES`,
  `--tools-verbosity`/`MCPOLLAMA_TOOLS_VERBOSITY` (`full|compact`; o tamanho do `tools/list`
  também custa tokens), `--debug`/`MCPOLLAMA_DEBUG`, `--version`. A fonte da verdade é
  `src/mcpollama-mcp/config.go`.
- **stdout é exclusivo do JSON-RPC.** Log e debug vão para stderr. O servidor não grava em
  disco a não ser que uma flag peça.
- **Sem tools administrativas.** O nginx devolve 403 em `/api/pull|push|create|delete|copy|blobs`.
- Respostas das tools devolvem os contadores do Ollama (`prompt_eval_count`, `eval_count`),
  para medir a economia.
- Testes de tool usam `httptest` para simular o Ollama (ver `referencias/ollama-mcp-go/main_test.go`).

---

## Servidor Ollama (alvo)

- `ia.cslsolucoes.com.br` = 10.100.2.2 · Ollama v0.40.8 · modelo **`qwen3-ia`** = MoE
  `qwen3-coder:30b-a3b` (30,5B, Q4_K_M, 100% GPU). Medido em 2026-10-23: ~97 tok/s de geração
  e ~5.000 tok/s de prefill (15.959 tokens em 3,2 s). Embeddings: `nomic-embed-text` (768
  dimensões); o `qwen3-ia` não gera embeddings.
- **Config atual (provisória): 3 GPUs e `num_ctx` 65536.** A GPU 3 está excluída por falha de
  PCIe, que **reiniciava a máquina** no prefill de prompts grandes (incidente de 2026-10-23,
  mitigado). A config volta a 4 GPUs e 131072 quando o hardware for reparado. **Não fixar o
  contexto no código:** usar o teto configurável. Relatórios em `.workspace/reports/`
  (`...190500-incidente-...` e `...191600-resposta-...`).
- Carga a frio do `qwen3-ia`: ~32–36 s. Detalhes medidos:
  `referencias/Brief_Referencias/Servidor-Ollama-CSL/dados/validacao-empirica.md`.
- Na LAN: `https://ia.cslsolucoes.com.br:8443` (porta explícita). Pela internet: o mesmo host
  sem porta (Cloudflare Tunnel); de dentro da LAN esse caminho dá timeout (DNS split-horizon).
  O certificado é Cloudflare Origin CA e o SO não confia nele: usar `--tls-ca` com
  `%USERPROFILE%\.openclaude\certs\cloudflare-origin-ca.pem`, somado ao `x509.SystemCertPool()`
  (sem substituir o pool do SO).
- Auth: `Authorization: Bearer`. Respostas: `200` com token válido, `401` sem token, `403` em endpoint bloqueado.
- Guia completo de operação (SSH, GPUs, nginx, checklist):
  `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md`.
- O nível `local` atual (`%USERPROFILE%\.claude\scripts\ia-local.ps1`, OpenClaude headless,
  config em `%USERPROFILE%\.openclaude\csl-ollama.env`) é o que este servidor substitui por uma
  tool MCP. Desde 2026-10-26 ele usa o mesmo `qwen3-ia` (decisão do owner: **só modelos
  otimizados para o hardware**: `qwen3-ia` para geração e `nomic-embed-text` para embeddings;
  os densos `qwen3.8:27b-*` não cabem inteiros na GPU). Com isso acabou a disputa de VRAM entre
  os dois níveis. Em teste ao vivo, **uma chamada de cada vez**: as GPUs servem um único modelo
  residente, e chamadas paralelas entram em fila.

---

## Segredos e `referencias/`

- O token do Ollama é usado **pelo caminho do arquivo**, nunca pelo conteúdo. Não colocar em
  código, testes, logs, chat ou commits. `.mcp.json` com segredo fica fora do git.
- `referencias/` é material de terceiros e de outros projetos. Contém credenciais reais
  (ex.: `referencias/MCPVMWare/.mcp.json`). **Somente leitura**, nunca commitar, e não copiar
  configs dali. O `.gitignore` usa `referencias/*` + `!referencias/Brief_Referencias/`:
  **só o brief** (síntese própria) é versionado e editável. Antes de gravar no brief, rodar a
  varredura de segredos do plano do brief (meta: 0 ocorrências).
- Muitas conexões SSH seguidas ao 10.100.2.2 podem acionar o fail2ban e banir a estação admin.

---

## Referências (consultar antes de decidir)

Ordem de consulta: **brief → referências locais → web**. O brief é a síntese rastreável
(`caminho:linha`) e evita reler código de terceiros.

### Brief e planos (deste projeto)

| Artefato | Conteúdo |
| --- | --- |
| `referencias/Brief_Referencias/` | brief detalhado de todas as referências: uma pasta por referência (10 Arquitetura · 20 Tools · 30 Config/Segurança · 40 Código · 50 Testes/Build · 80 Lições), `60-Requisitos` (`MCPO-REQ-<AREA>-nnn`), `61-Matriz_Cobertura`, `Comparativo`, `70-Revisao` e a validação empírica do servidor |
| `.workspace/plans/MCPOllam*-brief-referencias.plan.md` | plano do brief (ondas B0–B7, estado, changelog) |
| `.workspace/plans/MCPOllam*-mcpollama-implementacao.plan.md` | plano de implementação (Fases 0–6, pendências P-n) |
| `E:\GestorERP\.workspace\referencias\` | **molde** de método do brief (ex.: `Financeiro-ESoft/00-CONVENCOES.md`, `70-Revisao.md`) |

### Referências locais (`referencias/`, somente leitura)

| Pasta | Tamanho | O que aproveitar |
| --- | --- | --- |
| `ollama-mcp-go/` | 4 arquivos Go, 1,4 mil linhas | implementação mínima de MCP para Ollama: `generate`/`chat`/`embed`/`list_models`, streaming interno com trace em arquivo, negociação de `protocolVersion`, testes `httptest` |
| `MCPVMWare/src/` | 23 arquivos de núcleo + 134 `generated_*` | **molde CSL**: layout `src/`, `mcp/types.go` escrito à mão, flags→env, `--tools-verbosity compact`, `--profile`, gate `--allow-destructive` + `confirm`, audit log, gerador `src/gen` → `TOOLS.md`, `tools/Build-Release.ps1` |
| `MCPNas/truenas-mcp/` | 116 arquivos Go, 109 mil linhas | **molde CSL**: `--tls-ca`/`--insecure`, `proxy/` stdio↔HTTP, `tasks/` (jobs assíncronos + poller) |
| `MCPVMWare/.wolf/`, `MCPNas/truenas-mcp/.wolf/` | `cerebrum.md`, `buglog.json` | **lições aprendidas e bugs já resolvidos** nos dois projetos (+ `.workspace/plans|reports`) |
| `04-what-is-mcp/` | 1 arquivo Go, 47 linhas | exemplo didático de servidor MCP em Go (série ollama-tlms-golang) |

### Web (detalhe e data da coleta no README §5)

1. dxeo Ollama MCP (Go; modelo de raciocínio e de embeddings separados): <https://lobehub.com/mcp/dxeo-ollama-mcp>
2. Diretório MCP.so: <https://mcp.so/servers>
3. NightTrek/Ollama-mcp (deu 403 em 2026-10-23): <https://mcpservers.org/pt-BR/servers/NightTrek/Ollama-mcp>
4. Ollama vs LocalAI (API compatível com OpenAI, TLS + proxy de token): <https://contabo.com/blog/ollama-vs-localai-best-self-hosted-openai-compatible-llm-server/>
5. MCP com Ollama via Dolphin MCP (o Dolphin é o cliente MCP; o Ollama é só o provedor do modelo, conforme o brief `Web/W5`): <https://apidog.com/pt/blog/mcp-ollama/>
6. KTCrisis: `ollama-mcp-go` e `flux7-*` (mesh, supervisor, memória): <https://github.com/KTCrisis>
7. ollama-tlms-golang (Ollama + modelos pequenos + Go; "make your MCP server"): <https://github.com/ollama-tlms-golang>

### Infraestrutura

- Servidor Ollama: `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md`.
- Nível `local` atual: `%USERPROFILE%\.claude\scripts\ia-local.ps1`, com config em
  `%USERPROFILE%\.openclaude\csl-ollama.env` e a CA em `certs\cloudflare-origin-ca.pem`.
  Ler os segredos só pelo caminho, nunca imprimir.

---

## Pack `.cursor/` (SSOT dos espelhos)

**`.cursor/` é a única fonte canônica.** Nunca editar diretamente `.claude/`, `.vscode/`,
`.continue/` ou `.opencode/`: são espelhos via symlinks de `bootstrap-mirror-symlinks.ps1`.
Exceção: arquivos próprios de cada espelho, como `.claude/settings.json`, `.vscode/tasks.json`
e um futuro `.vscode/mcp.json`. Planos e reports do projeto vão em `.workspace/`
(rule `workspace-plans-persist`). A maioria das skills e agents do pack é Delphi/FPC; para Go/MCP
valem as genéricas de quality/governance e `mcp-server-dev:build-mcp-server`.

### Áreas protegidas (plan mode obrigatório)

Antes de criar, mover, renomear, fundir ou eliminar arquivos em `Documentation/`,
`.cursor/skills/`, `.cursor/Templates/`, `.cursor/agents/` ou `.cursor/rules/` (todas
recursivas), **apresentar um plano completo e aguardar aprovação explícita**, mesmo que o
usuário diga "execute". O plano deve ter: resumo, inventário, antes/depois, dependências e
backup. Template em `.cursor/plans/documentation-migration-plan_V1.0.plan.md`.

Exceções:
- typo isolado quando o usuário fornece o texto exato;
- conteúdo exato fornecido pelo usuário;
- scaffold via skill com fluxo próprio de confirmação.
