---
title: "mcpollama — implementação do servidor MCP em Go para o Ollama da CSL"
name: "mcpollama-implementacao"
overview: "Servidor MCP stdio em Go (src/ → mcpollama-mcp.exe na raiz) que delega trabalho mecânico do Claude ao Ollama qwen3-ia para reduzir tokens Anthropic."
created: 2026-10-23
updated: 2026-10-23
status: draft
locale: pt-BR
---

# mcpollama — plano de implementação

## Resumo

Construir em Go um servidor MCP (stdio, JSON-RPC 2.0) que expõe o Ollama self-hosted da CSL
(`qwen3-ia`, `ia.cslsolucoes.com.br`) como tools para Claude Code (CLI e extensão VS Code) e Cursor.
O Claude fica como orquestrador. Resumo, extração, tradução, rascunho e embeddings vão para o
modelo local, que custa zero da cota Anthropic. O ganho principal vem de tools que **leem arquivos
do lado do servidor** e devolvem só o resultado compacto. Resultado esperado: `mcpollama-mcp.exe`
na raiz do projeto, registrado nos três clientes, com a economia medida por contadores de tokens.

## Objectivos

1. Binário `mcpollama-mcp.exe` na raiz, gerado de `src/`, com `go build`/`go vet`/`go test -count=1` verdes.
2. Protocolo MCP mínimo correto: `initialize` (com negociação de `protocolVersion`), `notifications/initialized`,
   `tools/list`, `tools/call` e `ping`.
3. Configuração no molde MCPVMWare/MCPNas: flags com fallback para variável de ambiente.
4. Tools base: `list_models`, `generate`, `chat`, `embed`.
5. Tools de economia: `summarize_file`, `extract`, `translate`, que leem o arquivo localmente e respeitam um **teto de prompt configurável** (`--max-prompt-chars`, valor provisório na P-6), sem literal de contexto no código. O `qwen3-ia` está hoje com `num_ctx` 65536 (provisório, 3 GPUs); volta a 131072 após o reparo da GPU 3.
6. Todo resultado traz `prompt_eval_count`, `eval_count` e a duração, para medir a economia.
7. Registro e validação real nos três clientes (Claude Code, Cursor, VS Code).
8. Um report com a medição de tokens poupados num caso real.

## Âmbito e exclusões

**Entra:**
- `src/` (código Go e testes);
- executável na raiz;
- `.gitignore`;
- configs de cliente fora do git;
- atualização de `README.md` e `CLAUDE.md`.

**Fica fora:**
- tools administrativas do Ollama (`pull`/`push`/`create`/`delete`/`copy`/`blobs` dão 403 no nginx);
- transporte HTTP/SSE (só stdio nesta versão);
- mudanças no servidor 10.100.2.2 (nginx, systemd, GPUs);
- mudanças no `CLAUDE.md` global e no `ia-local.ps1`, que são do owner (ficam como pendência);
- áreas protegidas do pack (`.cursor/skills|rules|agents|Templates`, `Documentation/`).

## Inventário de ficheiros / artefactos

| Caminho | Operação | Nota |
| --- | --- | --- |
| `.gitignore` | criar | Fase 0 (feito em 2026-10-23) |
| `src/go.mod` | criar | `module github.com/cslsoftwares/mcpollama`, Go 1.26 |
| `src/mcp/types.go` | criar | tipos JSON-RPC 2.0 + MCP escritos à mão (molde `MCPVMWare/src/mcp/types.go`) |
| `src/mcpollama-mcp/main.go` | criar | entrypoint stdio: flags → env, loop de leitura, dispatch |
| `src/mcpollama-mcp/server.go` (+ `_test`) | criar | dispatch de métodos, negociação de versão, erros JSON-RPC |
| `src/ollama/client.go` (+ `_test`) | criar | HTTP: Bearer, `--tls-ca`, `--insecure`, timeouts, `/api/tags`, `/api/chat`, `/api/generate`, `/api/embed`, `/api/ps` |
| `src/tools/registry.go` | criar | registro de tools, `--tools-verbosity full|compact` |
| `src/tools/models.go`, `generate.go`, `chat.go`, `embed.go` (+ `_test`) | criar | tools base |
| `src/tools/files.go` (+ `_test`) | criar | `summarize_file`/`extract`/`translate` com allowlist de diretórios e chunking |
| `src/cmd/dumptools/` + `TOOLS.md` (raiz) | criar | catálogo das tools **gerado do registro** (P-B2 do brief, decidida em 2026-10-23). Mecanismo confirmado no B2: dump offline do registro com cliente nulo, como em `MCPVMWare/src/cmd/dumptools/main.go:23-47`. **Não** é o `src/gen` do MCPVMWare, que é um classificador de uso único (`src/gen/main.go:9-11`) |
| `mcpollama-mcp.exe` (raiz) | gerar | build, não versionado |
| `.mcp.json` (raiz) | criar | registro no Claude Code, **fora do git** |
| `.vscode/mcp.json` | criar | arquivo próprio do espelho (não é symlink), chave `servers` |
| `~/.cursor/mcp.json` | editar | global do Cursor (decidir na Fase 5) |
| `README.md`, `CLAUDE.md` | editar | refletir o estado real ao fim de cada fase |
| `.workspace/reports/MCPOllam<ts>-economia-tokens.report.md` | criar | Fase 6 |

## Passos / fases

**Requisitos (SSOT no brief).** O gate de cada fase inclui os requisitos **MUST** dela, listados em `referencias/Brief_Referencias/61-Matriz_Cobertura.md` §3 ("MUST por fase"). As contagens são: Fase 1: 44, Fase 2: 29, Fase 3: 38, Fase 4: 23, Fase 5: 14, Fase 6: 6. O enunciado e a verificação de cada `MCPO-REQ-*` estão em `60-Requisitos_mcpollama.md`. Os itens "Do brief" de cada fase abaixo vêm de `61` §5 ("Ajustes sugeridos às fases"). **Não duplicar requisitos aqui.**

### Fase 0 — Ambiente e repositório
1. Criar `.gitignore` (`*.exe`, binário Linux na raiz, `.mcp.json`, `referencias/`, `.remember/`, `.claude/settings.local.json`).
2. Rodar `git init`. **Sem commit** até o owner pedir.
3. Gate: `git status` não pode listar nada de `referencias/` nem `.mcp.json`.

### Fase 1 — Esqueleto do protocolo
1. Criar `src/go.mod`, `src/mcp/types.go` e `src/mcpollama-mcp/main.go`.
2. Loop stdio, um JSON por linha. **stdout só para JSON-RPC**; log vai para stderr.
3. Métodos: `initialize`, `notifications/initialized` (sem resposta), `ping`, `tools/list` (vazio), e `-32601` para método desconhecido.
4. Flags e env:
   - `--ollama-url`/`OLLAMA_HOST`
   - `--api-key`/`OLLAMA_API_KEY`
   - `--tls-ca`/`OLLAMA_TLS_CA`
   - `--insecure`/`OLLAMA_INSECURE`
   - `--model`/`OLLAMA_MODEL` (default `qwen3-ia`)
   - `--timeout`
   - `--debug`/`MCPOLLAMA_DEBUG`
   - `--version`
   - `--tools-verbosity`
5. Do brief:
   - leitura do stdin **sem** o limite de 64 KB do `bufio.Scanner` (`MCPO-REQ-PROTO`; o anti-padrão medido nos dois moldes CSL derruba o servidor);
   - `ping` → `{}`; `-32700` com `id:null`;
   - `Run()` testável, com smoke sem backend: **nenhuma chamada ao Ollama no `initialize`**;
   - stdout só JSON;
   - flags propostas: `--max-line-bytes`, `--embed-model`, `--max-prompt-chars` (com env; valores na P-6).
6. Gate:
   - `go build`, `go vet` e `go test -count=1 ./...` verdes;
   - smoke via pipe (`initialize` + `tools/list`), **lendo** as respostas;
   - os MUST da Fase 1 (`61` §3).

### Fase 2 — Cliente Ollama
1. Criar `src/ollama/client.go` com Bearer, CA customizada, `--insecure`, timeout por chamada e uso de `context`.
2. Tratar `401`, `403` e timeout com mensagem clara. **Nunca** incluir o token no erro ou no log.
3. Testes com `httptest`, cobrindo 200, 401, 403, timeout e JSON inválido.
4. Do brief:
   - TLS com `x509.SystemCertPool()` + `--tls-ca`, **não** `NewCertPool()` (medido na B4: funciona no Windows);
   - `context` por chamada;
   - mapa de status 401/403/404/5xx;
   - **conexão derrubada pelo servidor** vira erro de tool claro, que cita a carga a frio de ~36 s. Visto no incidente de 2026-10-23: o host reiniciava, e o cliente via só um RST;
   - decisão stream × não-stream por teste (PB5-8);
   - corpo de erro limitado;
   - redação do token por valor exato.
5. Gate de 2ª via (independente, contra o servidor real, uma chamada de cada vez): `curl` em `/api/tags` com o token,
   comparado com o resultado do cliente Go.

### Fase 3 — Tools base
1. `list_models` (`/api/tags` + `/api/ps`), `generate` (com `system`), `chat` (histórico enviado pelo cliente; sem estado no servidor) e `embed`.
2. Streaming interno do Ollama, com o resultado devolvido inteiro ao MCP (molde `ollama-mcp-go`).
3. Retorno com contadores de uso e duração.
4. Do brief:
   - semáforo de GPU N=1 e loop concorrente com `context` e `notifications/cancelled` (PB5-4, PB5-9);
   - linha de uso com os contadores em todo resultado (PB5-5);
   - `embed` usa um modelo de embeddings próprio: o `qwen3-ia` **não** tem essa capacidade. O medido é `nomic-embed-text`, com 768 dimensões, via `--embed-model` (PB5-6);
   - teste do conjunto exato de tools.
5. Gerador `src/cmd/dumptools` → `TOOLS.md`: monta o registro com cliente Ollama nulo e escreve o catálogo, que nunca é escrito à mão. Ele é atualizado de novo na Fase 4, com as tools de arquivo. Diferente do MCPVMWare, onde o disparo é manual e nada garante a sincronia, aqui um **teste** compara o `TOOLS.md` com o registro.
6. Gate:
   - testes `httptest` por tool;
   - smoke real (`tools/call generate`) lendo a resposta;
   - `TOOLS.md` regenerado sem diff contra o `tools/list` real.

### Fase 4 — Tools de economia (leitura local)
1. Flag `--allow-root <dir>[,<dir>]`: só lê arquivos dentro desses diretórios. O path é normalizado, com proteção contra `..` e symlinks que saiam da raiz.
2. `summarize_file`, `extract` (instrução + formato de saída) e `translate`, que leem o arquivo no servidor.
3. Limite de tamanho, com chunking ou map-reduce pelo teto `--max-prompt-chars` (P-6). Não assumir o contexto do modelo, que hoje é 65536 provisório e vai a 131072 após o reparo. **Medido:** ~2,67 caracteres por token no texto de teste (42.540 caracteres = 15.959 tokens); prefill de ~5.000 tok/s.
4. Do brief:
   - casos de caminho do Windows (junction, UNC, `\\?\`, ADS, dispositivos) e denylist (`.env`, `*.pem`, `*.key`; P-10, PB5-2);
   - sem escrita em disco (PB5-13);
   - teste ao vivo com arquivo grande **dentro do teto da P-6**, uma chamada de cada vez e com `/api/ps` antes e depois. Se o host voltar a reiniciar, parar e avisar o owner.
5. Gate:
   - testes de path traversal e de limite;
   - medição real: tokens que o Claude teria lido contra os tokens do resumo devolvido.

### Fase 5 — Integração com os clientes
1. Claude Code: `.mcp.json` na raiz apontando para `E:\MCPOllam\mcpollama-mcp.exe`; validar com `claude mcp list` + uma chamada real.
2. VS Code: criar `.vscode/mcp.json` (chave `servers`).
3. Cursor: escolher entre `.cursor/mcp.json` e `~/.cursor/mcp.json` (pendência P-3).
4. Verificar o timeout de tool de cada cliente diante de gerações longas e da carga a frio de ~36 s (PB5-3). O resultado define o default de `--timeout` e se o modo assíncrono entra (P-7).
5. Do brief:
   - limite de 60 caracteres `servidor + tool` no Cursor;
   - descobrir que campos de uso chegam ao modelo em cada cliente (PB5-5);
   - `.mcp.json` sem segredo literal (token pela env `OLLAMA_API_KEY`).
6. Gate: uma chamada real bem-sucedida em cada cliente, com evidência registrada.

### Fase 6 — Medição e documentação
1. Report em `.workspace/reports/` com um caso real antes e depois (tokens Anthropic gastos, tempo, qualidade).
2. Atualizar `README.md` e `CLAUDE.md` com o estado real, removendo "concepção" e "proposta".

## Dependências e riscos

- **Fila única de GPU:** um só modelo residente (`OLLAMA_KEEP_ALIVE=-1`), e chamadas paralelas entram em fila. Testes reais rodam uma chamada de cada vez.
- **Timeout do cliente MCP:** gerações longas podem passar do timeout de tool do cliente. Medir na Fase 5 antes de afirmar qualquer limite.
- **Hardware do servidor (GPU 3):** um prompt grande **reiniciava a máquina** por falha de PCIe na GPU 3. **Mitigado** em 2026-10-23: 3 GPUs e `num_ctx` 65536. A 2ª via pelo cliente às 19:18 mostrou 15.959 tokens em 3,2 s, sem queda. Relatórios: `.workspace/reports/MCPOllam2026-10-23-190500-incidente-...` e `...191600-resposta-...`. **Riscos que restam:**
  - o contexto muda quando o hardware for reparado (65536 → 131072), daí o teto configurável;
  - se a GPU 3 voltar sem o reparo, o defeito volta, e o cliente verá apenas um RST.
- **Carga a frio de ~32–36 s:** o `qwen3-ia` e o `nomic-embed-text` se desalojam na VRAM, então alternar `generate` e `embed` custa recarga.
- **Segredo em `args`:** fica visível na lista de processos. Sem `--api-key-file` (P-1 = não), a mitigação é registrar o token nos clientes pela **env** `OLLAMA_API_KEY`, não por `--api-key` em `args`.
- **Certificado:** a Cloudflare Origin CA não é confiada pelo SO. Usar `--tls-ca`; `--insecure` só para diagnóstico.
- **fail2ban:** testes não usam SSH em loop contra 10.100.2.2.
- **Leitura local** (Fase 4) é superfície de ataque. Sem `--allow-root`, as tools de arquivo ficam desabilitadas.
- **`referencias/` tem credenciais reais.** Fica no `.gitignore` e nunca é copiada.

## Critérios de conclusão

1. `go build`, `go vet` e `go test -count=1 ./...` verdes, num build limpo.
2. `mcpollama-mcp.exe` na raiz respondendo a `initialize`, `tools/list` e `tools/call` reais contra o `qwen3-ia`, com as respostas lidas.
3. Uma chamada real com sucesso em Claude Code, VS Code e Cursor, com evidência.
4. Report de economia com números medidos, não estimados.
5. `git status` limpo de segredos, binários e `referencias/`.

## Rollback

O projeto ainda não tem consumidores. Rollback por fase é `git` sobre o commit da fase anterior.
Para desligar nos clientes, basta remover a entrada do `.mcp.json`/`mcp.json`. O nível `local` via
`ia-local.ps1` continua funcionando de forma independente.

## Referências

- `README.md` (objetivo, links web, configs de cliente) e `CLAUDE.md` (comandos, regras).
- `referencias/MCPVMWare/src` (layout, flags→env, `--tools-verbosity`), `referencias/MCPNas/truenas-mcp` (`--tls-ca`),
  `referencias/ollama-mcp-go` (tools base, streaming, testes `httptest`).
- `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md` (servidor Ollama).

## Pendências (decisões do owner)

1. ~~**P-1:** aceitar `--api-key-file` (lê o token de um arquivo) além de `--api-key`/env?~~ **Decidida (2026-10-23 15:50): não.** Só `--api-key` e `OLLAMA_API_KEY`, como nos moldes MCPVMWare/TrueNAS.
2. ~~**P-2:** modelo default `qwen3-ia` (32k) ou `qwen3.8:27b-64k` para as tools de arquivo?~~ **Superada (B4 do brief, 2026-10-23):** o `qwen3-ia` passou a ser o MoE `qwen3-coder:30b-a3b`, com `num_ctx` 131072 e ~97 tok/s. O `qwen3.8:27b-64k` é o modelo do nível `local`. O default continua `qwen3-ia`; o limite real passa à P-6.
3. **P-3:** Cursor com config por projeto (`.cursor/mcp.json`, que fica no pack SSOT) ou global (`~/.cursor/mcp.json`)?
4. **P-4:** depois da Fase 6, atualizar o nível `local` do `CLAUDE.md` global para apontar para a tool MCP? (Atenção: isso troca também o modelo, de `qwen3.8:27b-64k` para `qwen3-ia`, e não só o transporte.)
5. ~~**P-5:** no 1º commit, versionar o pack `.cursor/` (1101 arquivos) e os symlinks dos espelhos, ou só `src/` + docs + `.workspace/` + o brief?~~ **Decidida (2026-10-23 19:21): só o projeto.** Entram `CLAUDE.md`, `README.md`, `.gitignore`, `.claudeignore`, `.cursorignore`, `.workspace/` e `referencias/Brief_Referencias/`. O pack `.cursor/` e os espelhos ficam fora; essa é uma decisão separada, ainda aberta.
6. **P-6:** valor default de `--max-prompt-chars` e política de chunking. **Decidida (2026-10-23 19:21): default de ~48k tokens ≈ 128.000 caracteres**, configurável por flag e env (`MCPO-REQ-CFG-010`). A política de chunking fica para a Fase 4.
   - Insumos: `num_ctx` 65536 hoje (131072 após o reparo); ~2,67 caracteres por token medidos; o owner recomenda um teto de entrada de ~48k tokens, deixando o resto para a geração (relatório `...191600-resposta-...` §4).
   - O defeito que bloqueava (PB4-1) está mitigado.
7. **P-7:** default de `--timeout` e adoção (ou não) do modo assíncrono (`task_id`). Depende da medição da Fase 5.
8. **P-8:** conjunto final de tools: incluir `rank_files` (COULD) e tools assíncronas?
9. ~~**P-9:** valor de `openWorldHint` nas tools. A spec usa `true` como default, então é preciso declarar (`MCPO-REQ-TOOL-005`).~~ **Decidida (2026-10-23 19:21): `openWorldHint:false`, declarado explicitamente** em todas as tools.
10. **P-10:** denylist de arquivos (`.env`, `*.pem`, `*.key`...), política de override e regras de caminho do Windows.

## Changelog

### 2026-10-23 15:08 — criação do plano (status: draft)
- **Feito:**
  - `README.md` (objetivo, integração, 7 links web anotados, referências locais);
  - `CLAUDE.md` reescrito para Go: wizard Delphi da FASE 2/3 substituído, executável na raiz, `src/` para os fontes.
- **Decisões do owner:**
  - nome `mcpollama` (módulo `github.com/cslsoftwares/mcpollama`);
  - configuração no molde MCPVMWare/MCPNas (flags → env);
  - fontes em `src/`;
  - executável na raiz;
  - nesta sessão só o plano, sem scaffold;
  - `.gitignore` + `git init` sem commit.
- **Não deu certo:** o link `mcpservers.org/.../NightTrek/Ollama-mcp` devolveu HTTP 403 na coleta. Ficou registrado sem descrição.
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 | em execução |
  | 1 a 6 | pendente |

### 2026-10-23 15:10 — Fase 0 concluída (sem commit)
- **Feito:**
  - `.gitignore` criado (`*.exe`, `/mcpollama-mcp`, `.mcp.json`, `referencias/`, `.remember/`, `.claude/settings.local.json`);
  - `git init` em `E:/MCPOllam/.git/`.
- **Gate (lido):** `git status --porcelain --untracked-files=all` mostra **0** entradas de `referencias/`, `.mcp.json` ou `.exe`. `git check-ignore` confirma `referencias/MCPVMWare/.mcp.json` (regra da linha 10) e `.remember/` (linha 13).
- **Medido:** arquivos não rastreados por pasta:

  | Pasta | Arquivos |
  | --- | --- |
  | `.cursor/` | 1101 |
  | `.claude/` | 9 |
  | `.vscode/` | 12 |
  | `.workspace/` | 1 |

  Os espelhos `.claude/` e `.vscode/` aparecem como symlinks (entradas próprias, sem duplicar conteúdo).
- **Atenção:** `core.symlinks=false` neste repositório. Ao clonar em outra máquina, os symlinks dos espelhos podem virar arquivos de texto com o caminho; o `bootstrap-mirror-symlinks.ps1` os recria.
- **Pendências:**
  - **P-5:** no 1º commit, versionar o pack `.cursor/` (1101 arquivos) e os symlinks dos espelhos, ou só `src/` + docs + `.workspace/`?
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 | concluída (sem commit) |
  | 1 a 6 | pendente (aguardam aprovação do owner) |

### 2026-10-23 15:50 — decisões do owner
- **Decisões (via AskUserQuestion):**
  1. **P-1 = não:** sem `--api-key-file`. Consequência registrada em "Dependências e riscos": o token vai nos clientes pela env `OLLAMA_API_KEY`, nunca em `args`.
  2. **P-B2 (do plano do brief) = sim, gerado do registro:** entra `src/gen` → `TOOLS.md` no inventário e o passo 4 da Fase 3, com gate de "sem diff contra o `tools/list` real".
  3. Brief aprovado (ondas B0 a B7, com gate por onda). Ele roda antes da Fase 1.
- **Pendências em aberto:** P-2 (espera o B4 do brief), P-3 (Fase 5), P-4 (após a Fase 6), P-5 (1º commit).
- **Fases:** sem mudança de estado (0 concluída; 1 a 6 pendentes).

### 2026-10-23 19:21 — integração do brief (B7) e do incidente do servidor
- **Feito:**
  - nota de SSOT para os requisitos: gate de cada fase = MUST do `61` §3;
  - itens "Do brief" nas Fases 1–5;
  - Objetivo 5, Fase 4 e Riscos reescritos com os dados medidos: `num_ctx` 65536 provisório (3 GPUs), ~2,67 caracteres por token, prefill de ~5.000 tok/s, hardware mitigado;
  - P-2 **superada**; P-5 a P-10 registradas como pendências do owner;
  - Fase 3: o `embed` usa um modelo próprio (`nomic-embed-text`).
- **Fonte:** plano do brief (changelog 18:52–19:21), relatório do incidente e relatório de resposta do owner.
- **Fases:** sem mudança de estado (0 concluída; 1 a 6 pendentes, aguardando aprovação).

### 2026-10-23 19:25 — decisões do owner (via AskUserQuestion)
1. **P-5 = só o projeto** no 1º commit (sem o pack `.cursor/` e os espelhos).
2. **P-6 = default de `--max-prompt-chars` ≈ 128.000 caracteres (~48k tokens).**
3. **PB6-1 (brief) = `MCPO-REQ-PROTO-009` vira MUST:** tool desconhecida → `-32602`. O brief foi recontado para **105 MUST, 52 SHOULD, 14 COULD**; a Fase 1 fica com 44 MUST e a Fase 3 com 38. Conferido nos dois documentos (`60` e `61`).
4. **P-9 = `openWorldHint:false`** declarado.
- **Ainda abertas:** P-3 (Fase 5), P-4 (após a Fase 6), P-7 (após a medição da Fase 5), P-8, P-10 (Fase 4) e o versionamento do pack `.cursor/`.
