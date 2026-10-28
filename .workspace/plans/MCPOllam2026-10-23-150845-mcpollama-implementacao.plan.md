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
10. ~~**P-10:** denylist de arquivos (`.env`, `*.pem`, `*.key`...), política de override e regras de caminho do Windows.~~ **Decidida (2026-10-23 21:08):**
    - **denylist fixa, sem override:** `.env*`, `*.pem`, `*.key`, `*.pfx`/`*.p12`, `id_rsa*`/`id_ed25519*`, `.mcp.json`, `*credential*`/`*secret*`, `settings.local.json` e tudo dentro de `.git/`;
    - **recusar todos os caminhos especiais do Windows:** UNC, `\\?\`, `\\.\`, ADS e nomes de dispositivo. Junction ou symlink só se o destino resolvido (`EvalSymlinks`) ficar dentro do `--allow-root`;
    - **arquivo acima do teto: map-reduce automático**, com partes ≤ teto em fronteira de linha e a linha `[uso]` somando tudo e informando o nº de partes.

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
- **1º commit:** `0fba414` (52 arquivos: `CLAUDE.md`, `README.md`, `.gitignore`, `.claudeignore`, `.cursorignore`, `.workspace/` e o brief), com stage por caminho, sem push.
- **Para a Fase 1:** criar o `.gitattributes` (`* text=auto eol=lf`). O `core.autocrlf` global converte para CRLF no checkout (lição de CRLF do truenas-mcp).

### 2026-10-23 19:30 — Fase 1 iniciada (ordem do owner: "prossiga por ordem de dependência")
- **Ponto de partida (lido):** `go1.26.5 windows/amd64`; `src/` existe e está vazio (sem `go.mod`); sem `.gitattributes`; `core.autocrlf=true`.
- **Execução:** implementação delegada a um agente Sonnet. O loop principal verifica: build limpo, `go vet`, `go test -count=1`, smoke via pipe lido e conferência dos 44 MUST da Fase 1 (`61` §3).
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 | concluída (commit `0fba414`) |
  | 1 | **em execução** |
  | 2 a 6 | pendentes |

### 2026-10-23 19:45 — Fase 1 concluída (agente Sonnet + verificação do loop principal)
- **Feito (agente):**
  - `src/go.mod`: módulo `github.com/cslsoftwares/mcpollama`, Go 1.26, **sem `require`**;
  - `src/mcp/types.go` (103 linhas), com os tipos JSON-RPC e MCP escritos à mão;
  - `src/mcpollama-mcp/`: `main.go` (48), `config.go` (226), `logger.go` (61), `server.go` (365), `config_test.go` (199) e `server_test.go` (547). Linhas medidas pelo loop principal; os números relatados pelo agente estavam desatualizados;
  - `.gitattributes` (`* text=auto eol=lf`, `*.go`/`go.mod`/`go.sum` em LF, `*.ps1` em CRLF, `*.exe binary`);
  - binário `mcpollama-mcp.exe` na raiz (ignorado por `.gitignore:2:*.exe`).
- **Achado do agente no smoke:** o pipe do PowerShell 5.1 antepõe um BOM UTF-8 à 1ª linha, e o servidor respondia `-32700`. Agora um BOM no início da linha é descartado (`TestLeadingBOMTolerated`).
- **Verificação do loop principal (2ª via, lida):**
  1. **Gate limpo:** `go clean -cache -testcache`; `go build ./...` = 0; `go vet ./...` = 0; `go test -count=1 -v ./...` com **43 PASS, 0 FAIL** (`ok .../mcpollama-mcp 0.065s`; `mcp` sem testes, só tipos); `gofmt -l .` vazio. O agente mediu **94,3%** de cobertura em `mcpollama-mcp`.
  2. **Falso alarme descartado:** o `gopls` acusou `parseConfig`, `validate`, `secrets`, `safeURL`, `newRedactor` e `debugf` como não usados. A busca de chamadores mostra uso real em `main.go:27,32,35,44` e `server.go:48,105,120,121,201,315`. Era diagnóstico desatualizado do editor.
  3. **Smoke do binário** (`-ldflags -X main.Version=0.1.0-smoke`), com stdin por arquivo (sem BOM), `--ollama-url https://exemplo.invalid`, `--debug` e token-**sentinela** falso em `OLLAMA_API_KEY`:
     - `exit=0`; **7 linhas de stdout, todas JSON válido**; a notificação não teve resposta;
     - `initialize` com `2025-06-18` → `protocolVersion` `2025-06-18`, `serverInfo.version` `0.1.0-smoke`;
     - `ping` → `{}`; `tools/list` → `{"tools":[]}`;
     - método inexistente → `-32601`; JSON inválido → `-32700` com `id` nulo; `tools/call` de `x` → **`-32602`** "Unknown tool: x";
     - **linha de ~100 KB → respondida `{}`**, sem o teto de 64 KB dos moldes;
     - sentinela: **0** ocorrências em stdout e em stderr; 0 CR no stdout;
     - `--version` → `mcpollama-mcp version 0.1.0-smoke`.
  4. **Fim de linha:** `git ls-files --eol` mostra todo o índice em LF (46 `w/lf` e 6 `w/crlf` na cópia de trabalho, sem diff). O `.gitattributes` não exige renormalização.
- **MUST da Fase 1 (44):** 41 atendidos, com teste ou comando listado pelo agente. A conferência por amostra no smoke cobriu PROTO-001/002/003/005/006/007/008/009, SEC-003, OBS-002, TEST-007/015 e BUILD-003. Exceções:
  - **BUILD-001 (parcial, esperado):** `ollama/`, `tools/` e `cmd/dumptools` são das Fases 2 e 3;
  - **BUILD-002 (agora atendido):** o `.exe` é ignorado (`git check-ignore`);
  - **BUILD-003 (atendido):** `var Version`, com `-X main.Version`, que é o nome do requisito (o prompt dizia `version`);
  - **BUILD-011 (atendido pelo loop principal):** este registro;
  - **PERF-004 × CFG-010 (conflito resolvido):** o PERF-004 pedia um teto abaixo de 42.540, redação de antes da mitigação do servidor. Foi **alinhado à decisão P-6** (128.000): `60` 1.2.1 e linha no `61`. O código já usava 128000.
  - **CFG-003 (parcial, esperado):** o `model` por chamada vem com as tools, na Fase 3.
- **Não deu certo / limitações:**
  1. **`-race` não roda nesta máquina:** `CGO_ENABLED=0` e não há `gcc` no PATH (verificado). Fica a pendência **P1-1**: rodar `-race` num ambiente com cgo (ex.: Linux/CI), obrigatório antes da Fase 3, que traz o loop concorrente (PB5-9).
  2. **Comportamentos decididos pelo agente sem requisito explícito** (ficam como estão; podem ser revistos):
     - `id:null` respondido com `id:null`;
     - JSON válido que não é objeto → `-32600`;
     - faltar `jsonrpc` ou `method` → `-32600`;
     - notificação desconhecida nunca recebe resposta;
     - `compact` corta para a 1ª frase;
     - `--ollama-url` vazio não impede o arranque;
     - `--timeout` com default de 120 s.
  3. `--trace-file` e `--audit-log-path` existem, mas só emitem um WARNING de "fase futura" e não gravam nada (SEC-014 e CFG-011 mantidos).
- **Docs atualizados:** `CLAUDE.md` (estado "Fase 1 concluída"; flags e env reais, apontando para o `config.go` como fonte da verdade); `README.md` (tabela de configuração implementada, com 13 linhas e defaults reais).
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 | concluída (`0fba414`) |
  | 1 | **concluída** (commit abaixo) |
  | 2 | próxima: cliente Ollama |
  | 3 a 6 | pendentes |

### 2026-10-23 19:47 — Fase 1 commitada; Fase 2 iniciada
- **Commit da Fase 1:** `67d05bb` ("feat: Fase 1 - esqueleto do protocolo MCP (stdio, JSON-RPC 2.0)"), 15 arquivos, stage por caminho. Pré-commit: varredura de `src/` com `known_hits=0` (16 genéricos = nomes de flag e sentinelas de teste); índice sem `.exe`/`.env`/`.mcp.json`.
- **Fase 2 (cliente Ollama) iniciada:** implementação delegada a um agente Sonnet, só com mocks `httptest` e sem chamar o servidor. O gate de 2ª via contra o servidor real (uma chamada de cada vez) fica com o loop principal. MUST da Fase 2: 29 (`61` §3).

  | Fase | Estado |
  | --- | --- |
  | 0 e 1 | concluídas (`0fba414`, `67d05bb`) |
  | 2 | **em execução** |
  | 3 a 6 | pendentes |

### 2026-10-23 20:05 — Fase 2 concluída (agente Sonnet + verificação e 2ª via ao vivo do loop principal)
- **Feito (agente):**
  - `src/ollama/`: `client.go` (196), `api.go` (361), `errors.go` (206), `types.go` (138);
  - testes `client_test.go` (341), `stream_test.go` (234), `security_test.go` (255), `tls_test.go` (159) e `live_test.go` (108, build tag `live`);
  - `src/redact/` (`redact.New`/`Mask` + teste): o `newRedactor` da Fase 1 foi **movido** para cá e é reusado pelo cliente (SSOT, sem duplicar). A Fase 1 foi ajustada para usá-lo (`logger.go`, `main.go`, `server.go`, `config.go` com `Config.ollamaOptions()`).
  - **Decisão PB5-8: stream interno (`stream:true`) com agregação** e `Options.DisableStream` para não-stream, ambos testados. Motivos:
    - a queda do servidor acontece antes dos cabeçalhos, então nenhum modo resolve;
    - o stream evita a conexão ociosa atrás do nginx (teoria: o `proxy_read_timeout` não foi medido);
    - o texto parcial é preservado em erro.
  - TLS: `SystemCertPool()` + PEM; `--insecure` sem WARNING duplicado; Bearer só no header, recusado em `http://` não-loopback (SEC-009, SHOULD); redirect 3xx vira erro.
  - Erros: 401/403 (HTML reduzido)/404/400/5xx; **conexão derrubada → `ErrConnectionDropped`**, com mensagem sobre reinício, carga a frio de ~36 s e `--max-prompt-chars`; sem retry.
  - Corpo de erro com `LimitReader` de 4 KiB; teto de prompt (128000 runas) aplicado **antes** do envio, com o mock recebendo 0 requisições.
  - Bugs achados pelo agente e corrigidos com teste:
    - `Scanner` com buffer inicial de 64 KiB ignorava um `MaxLineBytes` menor;
    - a ordem da redação (URL antes do token) deixava um resíduo.
- **Verificação do loop principal (lida):**
  1. **Gate limpo:** `go clean -cache -testcache`; build, `vet` e `vet -tags live` = 0; `go test -count=1 -cover -v ./...` com **115 PASS (93 de topo), 0 FAIL**. Cobertura: `mcpollama-mcp` 94,2%, `ollama` 92,1%, `redact` 100%. `gofmt` vazio; `go.mod` sem `require`; 0 literais 32768/65536/131072 fora de teste; 0 endpoints admin fora de teste.
  2. **Falso alarme do `gopls` descartado:** `maxErrBodyBytes`, `userAgent` e `debugf` acusados de não usados, mas em uso em `api.go:275,284,287,290,317,347`.
  3. **2ª via ao vivo** (20:03, `https://ia.cslsolucoes.com.br:8443`, `--tls-ca` da CA de origem, token do `csl-ollama.env` só em memória, **uma chamada de cada vez**):
     - `TestLive1Tags`: PASS, **5 modelos**, os mesmos da sonda independente da B4 (`qwen3-ia`, `qwen3-coder:30b-a3b-q4_K_M`, `nomic-embed-text`, `qwen3.8:27b-64k`, `qwen3.8:27b-q4_K_M`);
     - `TestLive2PS`: PASS; residente era o **`qwen3.8:27b-64k`** (`size_vram` 11.762.516.619, ctx 65536), o modelo do nível `local`, e não o `qwen3-ia`;
     - `TestLive3Unauthorized`: PASS, 401 → "token ausente ou inválido (OLLAMA_API_KEY) [HTTP 401 GET /api/tags]: unauthorized";
     - `TestLive4GenerateShort`: PASS, `response="ok"`, `done_reason=stop`, `prompt_eval=17`, `eval=2`, **`load_ms=21953`** (carga a frio do `qwen3-ia`), `total_ms=22117`;
     - token na saída: **0**.
- **MUST da Fase 2 (29):** todos atendidos, segundo a tabela do agente, com teste por item. O BUILD-011 foi feito pelo loop principal (este registro). O TEST-006 está coberto entre a Fase 1 (stdout/`--debug`) e a Fase 2 (erros e logs do cliente); a parte com tool real vem na Fase 3.
- **Achado operacional (novo):** o `ia-local` (`qwen3.8:27b-64k`) e o mcpollama (`qwen3-ia`) **disputam a VRAM**, e cada troca custa ~22–36 s de carga. Isso é insumo direto da **P-4** (trocar o nível `local` pela tool MCP elimina a disputa) e do timeout da Fase 5.
- **Observações:**
  - o `live_test.go` aceita `OLLAMA_API_KEY_FILE` (caminho de arquivo) **só como conveniência de teste**. O binário continua sem `--api-key-file` (P-1 = não);
  - as mensagens de erro ao usuário estão em pt-BR (textos do OLL-004); código e comentários em inglês;
  - `OLL-008` (timeout de inatividade) e `PERF-007` (disjuntor) são COULD e não foram feitos;
  - `-race` continua pendente (P1-1, sem cgo nesta máquina).
- **Docs:** `CLAUDE.md` com o estado "Fases 1 e 2 concluídas" e o uso da build tag `live`.
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 e 1 | concluídas (`0fba414`, `67d05bb`) |
  | 2 | **concluída** (commit abaixo) |
  | 3 | próxima: tools base (exige a P1-1 `-race` antes do loop concorrente) |
  | 4 a 6 | pendentes |

### 2026-10-23 20:40 — Fase 2 commitada; P1-1 (`-race`) resolvida; Fase 3 iniciada
- **Commit da Fase 2:** `b1c809d` (19 arquivos; varredura de `src/` com `known_hits=0`, índice sem `.exe`/`.env`/`.mcp.json`).
- **P1-1 (`-race`):**
  - perguntado ao owner via AskUserQuestion (opções: instalar gcc via winget, seguir sem `-race`, ou outra máquina). O owner apontou `E:\MCPOllam\deps`, `sdk` e `ZLibSource`;
  - `deps/gcc-mingw-w64/bin/gcc.exe` é o **GCC 16.1.0**, já presente. Usado **só por sessão** (`CC`, `PATH` e `CGO_ENABLED=1`), sem instalar nada.
  - **Resultado:** `go test -race -count=1 ./...` deu exit 0 (`mcpollama-mcp` 1,290 s, `ollama` 1,901 s, `redact` 1,076 s), com **0 `DATA RACE`** nas Fases 1 e 2. **P1-1 resolvida.**
- **Observação:** `deps/` (18.203 arquivos, 1.426,5 MB; `gcc-mingw-w64`, `gcc-linux-musl`, `win32`, `win64`), `sdk/` (2.593 arquivos, 44,3 MB, fontes de compressão) e `ZLibSource/` (798 arquivos, 19,3 MB) estão **não rastreadas e fora do `.gitignore`**. O stage por caminho as mantém fora dos commits. ~~**Pendência P2-1 (owner):** incluí-las no `.gitignore`?~~ **Decidida (20:43, owner: "sim, coloque"):** `/deps/`, `/sdk/` e `/ZLibSource/` acrescentados ao `.gitignore` (linhas 21–23). Verificado com `git check-ignore -v`: as três pastas casam e somem do `git status`. A mudança entra no commit da Fase 3.
- **Fase 3 (tools base) iniciada:** agente Sonnet; 38 MUST (`61` §3); gate com `-race` usando o gcc de `deps/`.

  | Fase | Estado |
  | --- | --- |
  | 0 a 2 | concluídas (`0fba414`, `67d05bb`, `b1c809d`) |
  | 3 | **em execução** |
  | 4 a 6 | pendentes |

### 2026-10-23 21:05 — Fase 3 concluída (agente Sonnet + verificação e 2ª via ao vivo do loop principal)
- **Feito (agente):**
  - `src/tools/`: `registry.go`, `args.go` (helpers de argumento num arquivo só), `schema.go`, `queue.go` (fila de GPU N=1), `usage.go` (linha `[uso]` + `_meta`), `generate.go`, `chat.go`, `embed.go`, `list_models.go` e `catalog.go`, mais 8 arquivos de teste (incluindo `live_test.go` com a tag `live`);
  - `src/cmd/dumptools/` (`-o`, `-check`) e o **`TOOLS.md`** na raiz (190 linhas, 4935 bytes);
  - `src/mcpollama-mcp/backend.go` (cliente lazy + fila), `concurrency_test.go` e `tools_wiring_test.go`;
  - `mcp/types.go` ganhou `Tool.Title`, `Tool.Annotations`, `ToolCallResult.Meta` (`_meta`), e os tipos `ToolHandler`/`ToolEntry` foram movidos para o pacote `mcp`, porque `tools` não pode importar `main`.
  - **Loop concorrente:** goroutine por `tools/call`, `notifications/cancelled` suprime a resposta, um `id` repetido em curso recebe `-32600`, e no EOF o servidor espera as chamadas em curso.
  - O `list_models` **não** passa pela fila, de propósito (não carrega modelo).
  - Teto de saída de 32.000 caracteres, com aviso (TOK-007 parcial).
  - O agente quebrou de propósito 3 pontos (supressão por cancelamento, fila, sincronia do `TOOLS.md`) e os testes pegaram os 3.
- **Verificação do loop principal (lida):**
  1. **Falso alarme do `gopls`:** ~30 "erros de compilação" (`mcp.ToolEntry` indefinido etc.) eram estado antigo do editor. O `types.go` define tudo, e o build limpo passou.
  2. **Gate limpo:** `go clean -cache -testcache`; build, `vet` e `vet -tags live` = 0; `go test -count=1 -cover -v ./...` com **186 PASS, 0 FAIL**. Cobertura: `dumptools` 73,0%, `mcpollama-mcp` 94,2%, `ollama` 92,1%, `redact` 100%, `tools` 94,7%. `gofmt` vazio.
  3. **`-race`** (gcc de `deps/`): exit 0, **0 DATA RACE** nos 5 pacotes.
  4. **`go run ./cmd/dumptools -check`:** "TOOLS.md is in sync".
  5. **Smoke offline** (binário `0.3.0-smoke`, `--ollama-url https://exemplo.invalid`, sentinela em `OLLAMA_API_KEY`):
     - `tools/list` **full = 4000 bytes, compact = 2540 bytes** (−36,5%);
     - 4 tools: `chat`, `embed`, `generate` e `list_models`, todas com `openWorldHint=false`;
     - `prompt:5` → `isError` "`prompt` must be a string (recebido: number)";
     - host inexistente → `isError` "não foi possível falar com …: no such host";
     - sentinela: 0 em stdout e 0 em stderr nos dois modos.
  6. **2ª via ao vivo** (21:00, `:8443`, uma chamada de cada vez, token só em memória):
     - `TestLive1ListModels`: PASS;
     - `TestLive2Generate`: PASS, `[uso] modelo=qwen3-ia prompt_eval_count=17 eval_count=2 total_ms=22156 load_ms=21990 queue_ms=0 fim=stop`. O `qwen3-ia` estava **fora da VRAM** de novo, provavelmente desalojado pelo `ia-local`;
     - `TestLive3Chat`: PASS, `total_ms=65 load_ms=0` (quente);
     - `TestLive4Embed`: PASS, `modelo=nomic-embed-text prompt_eval_count=15 total_ms=6805 load_ms=4760`;
     - token na saída: **0**.
- **MUST da Fase 3 (38):** todos atendidos, segundo a tabela do agente, com teste por item.
  - **TEST-016** (2ª via ao vivo) e **INTEG-012** (em série): feitos pelo loop principal acima.
  - **BUILD-011:** este registro.
  - **TEST-008:** a parte com `--allow-root` fica para a Fase 4.
- **SHOULD extras:** TOOL-004, TOOL-005 (`openWorldHint:false`), TOOL-009, TOOL-012, TOK-005 e TOK-006 (orçamento de 4400 bytes para o `tools/list`). O TOK-003 saiu só no `_meta`.
- **Desvios registrados:**
  - respostas podem sair fora de ordem (casadas por `id`);
  - fila com N=1 fixo, sem `--gpu-slots`, e sem timeout de fila separado (PERF-003, SHOULD);
  - `input` do `embed` com `oneOf` (compatibilidade com os clientes a verificar na Fase 5);
  - mensagem de tipo errado mistura inglês e pt-BR ("`prompt` must be a string (recebido: number)"), como no texto do requisito.
- **Docs:** `README.md` (seção Tools com o `TOOLS.md`, a linha `[uso]` e as anotações); `CLAUDE.md` (estado "Fases 1, 2 e 3 concluídas"); `.gitignore` (P2-1) entra neste commit.
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 a 2 | concluídas |
  | 3 | **concluída** (commit abaixo) |
  | 4 | próxima: tools de arquivo (`--allow-root`, P-10 em aberto) |
  | 5 e 6 | pendentes |

### 2026-10-23 21:08 — Fase 3 commitada; decisões da Fase 4; Fase 4 iniciada
- **Commit da Fase 3:** `dedc305` (32 arquivos, incluindo o `.gitignore` da P2-1; varredura de `src/` com `known_hits=0`).
- **Decisões do owner (via AskUserQuestion):** P-10 com denylist fixa sem override, recusa de todos os caminhos especiais do Windows e map-reduce automático (detalhe na lista de pendências, item 10).
- **Fase 4 iniciada:** implementação em Sonnet; depois, **revisão adversarial de segurança em Opus**, porque a leitura de arquivo local é a maior superfície de ataque do projeto; e o gate do loop principal (limpo, `-race`, smoke e 2ª via ao vivo **dentro do teto**). MUST da Fase 4: 23 (`61` §3).

  | Fase | Estado |
  | --- | --- |
  | 0 a 3 | concluídas (`0fba414`, `67d05bb`, `b1c809d`, `dedc305`) |
  | 4 | **em execução** |
  | 5 e 6 | pendentes |

### 2026-10-23 22:05 — Fase 4: implementação, revisão adversarial, correção do escape crítico e verificação
- **Feito (agente Sonnet):**
  - `src/files/` (`policy.go`, `guard.go`, `guard_test.go`): contenção em 10 camadas: sintaxe Windows, forma, `Clean` + denylist no nome pedido, `EvalSymlinks`, `filepath.Rel`, denylist no nome resolvido, intermediários sem reparse point, abertura via `os.Root`, `Stat` antes e depois com `SameFile` e limite de bytes, e 2º `EvalSymlinks`;
  - `src/tools/filetools.go` e `chunk.go`: `summarize_file`, `extract` e `translate`, com map-reduce, `--max-chunks` (default 40), linha `[arquivo]` + `partes=N` no `[uso]`, conteúdo delimitado por nonce e `temperature` 0,2;
  - `--allow-root`/`MCPOLLAMA_ALLOW_ROOT`; `TOOLS.md` com as 7 tools.
  - **Desvios do agente:**
    - junction sempre recusada, inclusive para dentro (o Go 1.26 não a segue);
    - denylist +3 padrões (`*.env`, `id_ecdsa*`, `id_dsa*`), também aplicada a diretórios;
    - `translate` concatena as partes, sem redução;
    - tetos de tamanho para `instruction`, `format` e `focus`.
- **Revisão adversarial (Opus):**
  - a 1ª execução caiu por **529 (API sobrecarregada)**, sem produzir nada; foi retomada;
  - relatório `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md`, com os testes de ataque em `...testes\`: **232 vetores** (219 bloqueados, **4 escaparam, todos por hardlink**, 7 achados de política, 2 informativos, 3 não testáveis);
  - TOCTOU com ~8.200 leituras e 0 vazamentos; map-reduce com 1.350 combinações e nada acima do teto;
  - veredito: **NÃO SEGURO** no estado entregue, **SEGURO COM RESSALVAS** após a correção do hardlink;
  - o revisor relatou e corrigiu um erro de processo próprio (encoding no PowerShell 5.1).
- **SEC-C-01 (crítico), hardlink: confirmado por 2ª via pelo loop principal.** Binário real, `--allow-root` temporário, URL inexistente como oráculo: `notas.txt` (hardlink → `.env`) e `externo-link.txt` (hardlink → arquivo **fora da raiz**) foram **lidos** e chegaram a tentar o envio; o `.env` direto foi recusado.
  - **Correção aplicada pelo loop principal:** `guard.go` recusa arquivo com `linkCount != 1`, de forma opaca e fail-closed, mais `nlink_windows.go` (`GetFileInformationByHandle`) e `nlink_other.go` (`Stat_t.Nlink`);
  - regressão `TestHardlinksAreRefused`, que também prova que o original volta a ser legível com 1 link;
  - **controle negativo** numa cópia com a correção desligada: o teste **falha** (`hl-env.txt: hard link was read (24 bytes)`), ou seja, pega o defeito;
  - **2ª via após a correção:** os 2 hardlinks são recusados ("arquivo não encontrado ou fora das raízes permitidas"), o `normal.txt` continua legível e há 0 sentinelas na saída.
- **SEC-M-01 (conteúdo ecoado em erro) e SEC-m-01 (nonce reaproveitado na redução):** corrigidos por um agente Sonnet.
  - `sanitizeUpstream` descarta o corpo do upstream nos erros das tools de arquivo, mantendo `Kind`/`Status` e `errors.Is`;
  - a redução usa um nonce novo por rodada, conferido contra os parciais;
  - testes `TestFileToolErrorDoesNotEchoUpstreamBody` (4 subcasos), `TestSanitizeUpstreamKeepsLocalErrors` e `TestReduceUsesFreshNonce`. O agente desfez as correções e os testes falharam.
- **Teste 8.3:** movido para `guard_windows_test.go`, e o `guard_test.go` voltou a compilar para Linux (`GOOS=linux go vet` = 0). **Rodado no C:** (8dot3 habilitado): PASS, com `ENVPRO~1.LOC` negado pelo nome resolvido. O SKIP no D: é só de ambiente.
- **Verificação final do loop principal (lida):**
  - `go clean -cache -testcache`; build, `vet`, `vet -tags live` e `GOOS=linux vet` = 0;
  - `go test -count=1 -cover -v ./...` com **245 PASS, 0 FAIL, 1 SKIP** (8.3 no D:). Cobertura: `files` 90,7%, `tools` 92,7%, `mcpollama-mcp` 94,3%, `ollama` 92,1%, `redact` 100%, `dumptools` 73,0%;
  - **`-race` com 0 DATA RACE**; `gofmt` vazio; `dumptools -check` em sincronia.
- **2ª via ao vivo** (22:00, uma chamada de cada vez, arquivo `ata.txt` de 289 bytes):
  - `TestLive5SummarizeFile`: PASS, `prompt_eval_count=362 eval_count=66 total_ms=25707 load_ms=24738` (carga a frio, o `qwen3-ia` de novo fora da VRAM);
  - `TestLive6Extract`: PASS, `total_ms=230`;
  - `TestLive7Translate`: PASS, `total_ms=948`;
  - `partes=1` nos 3; token na saída: 0.
- **Ressalvas abertas da revisão (decisões do owner):**
  - SEC-M-02: `--allow-root C:\` é aceito e lê `C:\Windows\win.ini`;
  - SEC-M-03: lacunas da denylist (`.npmrc`, `.pypirc`, `.netrc`, `*.ppk`, `.kube\config`, `.docker\config.json`, `.ssh\`, `.aws\`, `.claude.json` etc.);
  - SEC-m-02: vírgula no nome de pasta parte o `--allow-root`;
  - raiz relativa aceita;
  - raízes aninhadas, com a denylist dependendo da ordem;
  - raiz via junction aceita, mas inútil;
  - hardlink entre volumes não testável (NTFS não permite).

### 2026-10-23 22:10 — decisões do owner sobre as ressalvas + commit da Fase 4
- **Decisões (via AskUserQuestion):**
  1. **SEC-M-02 = aceitar com aviso:** raiz de unidade, `%USERPROFILE%` ou diretório do sistema sobem, mas com WARNING explícito em stderr a cada início.
  2. **SEC-M-03 = manter a denylist como está** (P-10 + os 3 padrões do agente). As lacunas listadas pelo revisor ficam **aceitas pelo owner** e documentadas.
  3. **`--allow-root`:** separador `;` (aceita vírgula em nome de pasta), raiz **só absoluta** (relativa recusada na partida) e **raízes aninhadas recusadas**.
  4. **Commit agora; as ressalvas 1 e 3 num commit `fix` próprio**, com o mesmo gate.
- **Docs:** `CLAUDE.md` (estado "Fases 1 a 4 concluídas"; regra de rodar de novo os testes de ataque ao mexer em `src/files/`) e `README.md` (tools de arquivo e flags `--allow-root`/`--max-chunks`).
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 a 3 | concluídas |
  | 4 | **concluída** (commit abaixo), com o `fix` de ressalvas em seguida |
  | 5 e 6 | pendentes |

### 2026-10-23 22:30 — Fase 4 commitada (`e6dfb5b`) + `fix` das ressalvas do `--allow-root`
- **Commit da Fase 4:** `e6dfb5b` (41 arquivos, incluindo o relatório e os testes de ataque em `.workspace/reports`; varredura de `src/` e `reports/` com `known_hits=0`).
- **Fix (agente Sonnet), decisões do owner de 22:10:**
  - separador `os.PathListSeparator` (`;` no Windows): vírgula passa a ser válida em nome de pasta;
  - raiz relativa → erro na partida;
  - raízes aninhadas (nos dois sentidos, ou repetidas) → erro na partida;
  - **raiz via junction → erro na partida**, com mensagem clara: o agente escolheu recusar em vez de suportar, porque o `EvalSymlinks` do Go não atravessa junction;
  - **raiz ampla** (volume, `%USERPROFILE%`, `%SystemRoot%`, `%ProgramFiles%`, `%ProgramFiles(x86)%`, `%ProgramW6432%`, `%ProgramData%`) → **sobe com `WARNING:` em stderr** a cada início;
  - denylist **inalterada**.
  - Testes novos: 6 em `files/guard_test.go` e 2 em `files_wiring_test.go`.
  - **Testes de ataque da revisão rodados de novo** numa cópia: compilaram sem ajuste, 209 PASS, **ESCAPOU=0**, BLOQUEADO=223. Restam 2 ACHADO, ambos aceitos pelo owner: `deny/gaps` (denylist mantida) e `cfg/drive-root` (agora com aviso).
- **Verificação do loop principal (lida):**
  - `go clean -cache -testcache`; build, `vet` e `GOOS=linux vet` = 0;
  - `go test -count=1 -v ./...` com **253 PASS, 0 FAIL, 1 SKIP** (8.3 no D:);
  - `-race` com 0 DATA RACE; `gofmt` vazio; `dumptools -check` em sincronia;
  - **binário real, 5 cenários de partida:**
    - `Cliente, Inc` → exit 0, sem aviso;
    - `.\rel` → **exit 2** "a raiz deve ser um caminho absoluto";
    - raiz aninhada (`a;a\b`) → **exit 2**;
    - `%USERPROFILE%` → exit 0, com **WARNING** "is a volume root, your home directory or a system directory: ALL content ... can be read and sent to the remote Ollama server";
    - raiz normal → exit 0, sem aviso.
- **Observação:** o WARNING está em inglês, como os outros avisos de partida; os erros de partida estão em pt-BR.
- **Commit do fix:** `9ef7935`.

### 2026-10-23 22:45 — Fase 5 iniciada: decisões do owner e registro nos clientes
- **Decisões (via AskUserQuestion):**
  1. **Token literal** nas configs dos clientes (fora do git);
  2. **Claude Code no escopo de projeto** (`.mcp.json` na raiz); o escopo de usuário fica para depois da Fase 6, junto com a P-4;
  3. **P-3 = `.cursor/mcp.json` do projeto**;
  4. `--allow-root E:\MCPOllam` durante a validação.
- **Feito:**
  - **`.gitignore`:** acrescentados `/.cursor/mcp.json` e `/.vscode/mcp.json` (só `.mcp.json` era coberto). `git check-ignore -v` confirmou os 3 **antes** de escrever o token.
  - Os espelhos `.claude/` e `.vscode/` linkam só subpastas específicas do `.cursor/` (`agents`, `commands`, `plans`, `rules`, `skills`, `Templates`, `README.md`, `VERSION.md`), então o `.cursor/mcp.json` **não** é espelhado.
  - Binário `mcpollama-mcp.exe` com `--version` = `0.4.0-9ef7935`.
  - Configs escritas com o token lido do `csl-ollama.env` sem exibi-lo:
    - `.mcp.json` e `.cursor/mcp.json` (`mcpServers`);
    - `.vscode/mcp.json` (`servers`, `type: stdio`);
    - os três com `command` = o binário da raiz, `args` = `--ollama-url https://ia.cslsolucoes.com.br:8443 --tls-ca <CA de origem> --allow-root E:\MCPOllam`, e `env.OLLAMA_API_KEY`.
  - Conferido: JSON válido, token presente, `command` existe, e nenhum dos 3 aparece no `git status`.
- **Claude Code (`claude mcp list`, lido):** `mcpOllamaCSL` → **"⏸ Pending approval (run `claude` to approve)"**. Servidor de escopo de projeto exige aprovação numa sessão interativa; o token do Ollama não apareceu na saída (0).
- **INCIDENTE de processo (meu):** o `claude mcp list` imprime os `args` de **todos** os servidores do usuário. O `mcpTrueNas`, registrado fora deste projeto, tem uma **API key do TrueNAS em texto puro nos `args`**, e ela apareceu na saída e entrou no contexto da sessão. O filtro que eu aplicava cobria só o token do Ollama. **Ação recomendada ao owner:** rotacionar a chave do TrueNAS e movê-la de `args` para `env`. **Lição:** comandos que listam configs de terceiros (`claude mcp list`/`get`) precisam de filtro genérico de segredo, ou não devem ser rodados com a saída exposta. O valor **não** foi registrado em nenhum arquivo.
- **Pendências da Fase 5 (owner):**
  - **P5-1:** aprovar o `mcpOllamaCSL` no Claude Code;
  - **P5-2:** validar no Cursor e no VS Code (GUI), com uma chamada real cada e medindo o timeout de tool (PB5-3);
  - **P5-3:** rotacionar a chave do TrueNAS.

### 2026-10-23 22:16 — Fase 5: Claude Code validado (headless), Cursor e VS Code pendentes
- **Aprovação (decisão do owner: "Eu habilito via settings"):**
  - `enabledMcpjsonServers: ["mcpOllamaCSL"]` acrescentado ao `.claude/settings.local.json` (fora do git), preservando as 109 regras existentes;
  - **não bastou:** o `claude mcp list` continua "Pending approval". A causa, dada pelo próprio CLI, é que o workspace **não está marcado como confiável** (`projects["E:/MCPOllam"].hasTrustDialogAccepted` no `~/.claude.json`). Isso só se resolve abrindo o `claude` interativo aqui uma vez e aceitando o diálogo de confiança. **Não** editei o `~/.claude.json` (config global do usuário).
- **Validação real no Claude Code (headless, `claude -p --mcp-config .mcp.json --strict-mcp-config`, Haiku, só a tool liberada):**
  1. `list_models`: exit 0, 16,9 s, **5 modelos** recebidos pelo cliente; token na saída 0. A tool não tem linha `[uso]` (não chama modelo); o cliente reportou isso corretamente.
  2. `generate` ("Responda apenas: ok"): exit 0, 17,2 s, resposta "ok" e **a linha `[uso] modelo=qwen3-ia prompt_eval_count=14 eval_count=2 total_ms=120 load_ms=0 queue_ms=0 fim=stop` chegou ao modelo do cliente** e foi copiada literalmente. **PB5-5 validado para o Claude Code:** os contadores no texto chegam ao modelo. Token na saída: 0.
- **Segredos de terceiros vistos nesta etapa (não registrados por valor):**
  - **P5-3:** API key do TrueNAS nos `args` do `mcpTrueNas`, exposta pelo `claude mcp list`;
  - **P5-4 (novo):** o `.claude/settings.local.json` deste projeto, herdado do template e de outro projeto, tem **senha `sa` do SQL Server em texto puro** em regras `permissions.allow` (`sqlcmd -P ...`). O arquivo está fora do git, mas o valor entrou no contexto ao ler o arquivo para editá-lo, como manda a skill (ler antes de editar). Recomendação: rotacionar a senha e limpar essas regras.
- **Pendências da Fase 5 (atualizadas):**
  - **P5-1:** abrir o `claude` interativo em `E:\MCPOllam` e aceitar a confiança do workspace;
  - **P5-2:** Cursor (`.cursor/mcp.json` já escrito) e VS Code (`.vscode/mcp.json` já escrito): reiniciar ou recarregar, fazer 1 chamada real (`list_models` e `generate`) e anotar se a linha `[uso]` chega ao modelo e o tempo/timeout de uma geração longa (PB5-3);
  - **P5-3 e P5-4:** rotacionar a chave do TrueNAS e a senha `sa`.
- **Commit:** `8e99a9b` (`.gitignore` + plano).
- **Decisão do owner (22:17):** **esperar os testes de GUI** (P5-1/P5-2) antes da Fase 6. A Fase 6 fica bloqueada até o retorno do owner.

  | Fase | Estado |
  | --- | --- |
  | 0 a 4 | concluídas (`0fba414`, `67d05bb`, `b1c809d`, `dedc305`, `e6dfb5b` + `9ef7935`) |
  | 5 | **em progresso:** Claude Code validado (headless); confiança do workspace, Cursor e VS Code com o owner |
  | 6 | **bloqueada**, aguardando o fim da Fase 5 (decisão do owner) |

### 2026-10-24 17:30 — foco no Claude Code; renomeação do servidor; Fase 5 validada na sessão real
- **Decisões do owner:**
  1. "o foco principal é no claude" → **Cursor e VS Code deixam de bloquear o projeto**: a validação neles vira opcional (P5-2 rebaixada) e a Fase 6 fica desbloqueada;
  2. **renomear o servidor `mcpOllamaCSL` → `mcpOllama`.**
- **Renomeação feita** nos 4 arquivos ignorados pelo git (`.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` e `enabledMcpjsonServers` do `.claude/settings.local.json`), por script, sem exibir o token. Conferido: 1 troca em cada, JSON válido, novo nome presente, 0 restos do nome antigo, os 4 continuam ignorados.
  - Também no exemplo do `README.md`.
  - `src/` e `TOOLS.md` não usam o nome do servidor (busca case-sensitive: 0).
  - As menções antigas neste plano ficam como histórico.
  - Nesta sessão as tools ainda aparecem como `mcp__mcpOllamaCSL__*`; o nome novo vale a partir do próximo início do Claude Code.
- **P5-1 resolvida:** na sessão retomada (2026-10-24), o Claude Code carregou o servidor de projeto, e as 7 tools aparecem como tools da sessão. Validação **interativa e real**, feita pelo próprio loop principal:
  - `list_models`: 5 instalados; em VRAM só o `qwen3.8:27b-64k` (nível `local`), ctx 65536;
  - `summarize_file` de `validacao-empirica.md` (12.548 bytes, 12.313 caracteres, `max_words` 120): resumo correto em pt-BR. **O texto do arquivo não entrou no contexto do Claude, só o resumo.** `[uso] modelo=qwen3-ia prompt_eval_count=4970 eval_count=197 total_ms=25764 load_ms=22023 queue_ms=0 partes=1`. Carga a frio de 22 s, porque o `ia-local` tinha o outro modelo residente.
- **`CLAUDE.md`:** seção "Estado" corrigida (removidos "`tools/list` (ainda vazio)" e "Próxima: Fase 5"; acrescentados o foco no Claude Code, o nome `mcpOllama` e a próxima etapa, Fase 6).
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 a 4 | concluídas |
  | 5 | **concluída para o Claude Code** (foco do owner). Pendências opcionais: P5-2 Cursor/VS Code. Pendências do owner: P5-3/P5-4 (rotação de segredos) |
  | 6 | **desbloqueada**, próxima |
- **Commit:** `3b904ca`.

### 2026-10-24 17:36 — Fase 6 concluída: medição da economia (Claude Code headless)
- **Relatório:** `.workspace/reports/MCPOllam2026-10-24-173500-economia-tokens.report.md`. Evidência bruta (`A.json`, `A2.json`, `B.json`, `T3.json`) em `...economia-tokens.evidencias/`, varrida antes da cópia (`known_hits=0`).
- **Método:** mesmo pedido (resumir `60-Requisitos_mcpollama.md`, 72.114 caracteres), com `claude -p --model haiku --output-format json`:
  - A: `Read`, cache frio;
  - B: `summarize_file` via `--mcp-config .mcp.json --strict-mcp-config`;
  - A2: `Read` repetido com o cache quente, para tirar o efeito da ordem.
- **Medido (lido do JSON):**

  | Rodada | Custo (US$) | Turnos | Turno final: criação de cache | Parede |
  | --- | ---: | ---: | ---: | ---: |
  | A | 0,03944 | 3 | 16.060 | 31,2 s |
  | A2 | 0,03702 | 2 | 23.244 | 23,7 s |
  | B | 0,01243 | 3 | 1.069 | 39,3 s |

  - **Custo −66,4%** (B × A2), −68,5% (B × A);
  - **~22,2 mil tokens do arquivo fora do contexto** do Claude;
  - lado Ollama em B: `prompt_eval_count=27474 eval_count=443 total_ms=18776 load_ms=1`.
- **Qualidade:** os dois resumos acertam 171 / 105 / 52 / 14. O B listou as 12 áreas, com uma imprecisão ("128.000 caracteres falha"); o A repetiu uma frase superada do documento. Qualidade equivalente para orientação.
- **PB5-3 / P-7 (timeout):** o `generate` com `num_predict` 7000 levou **86,7 s no servidor e completou sem corte no Claude Code** (parede 107,3 s, `fim=length`). O cliente aceita pelo menos ~87 s; **o limite efetivo hoje é o `--timeout` de 120 s do mcpollama**. O máximo do Claude Code não foi medido.
- **Não deu certo, e a solução:** o 1º teste de timeout não valeu. No PowerShell 5.1, as aspas duplas internas do argumento foram quebradas na passagem para o `claude.exe`, e o modelo recebeu só "Escreva" (3,9 s). Refeito com aspas simples dentro do prompt. O 2º teste parou sozinho em 1.933 tokens (21 s), então foi forçada uma saída longa determinística (contagem por extenso), o 3º teste.
- **Limitações registradas no relatório:** n=1 por variante; contexto base dominando os totais; um tipo de tarefa só; Haiku como cliente; servidor quente em B.
- **Pendências do owner:**
  - **P-7:** default do `--timeout` (120 s hoje; uma geração no teto de 32.768 tokens leva ~6 min);
  - **P-4:** escopo de usuário e troca do nível `local`;
  - P-8;
  - P5-2 (opcional), P5-3 e P5-4.
- **Fases:**

  | Fase | Estado |
  | --- | --- |
  | 0 a 5 | concluídas (5 no Claude Code) |
  | 6 | **concluída** (commit abaixo) |
- **Commit:** `6bc3b48`.

### 2026-10-24 17:40 — decisões finais do owner (P-7, P-4)
- **P-7 = manter o `--timeout` em 120 s** (default do código, sem mudança). Fica documentado que gerações acima de ~8 mil tokens com o modelo frio podem estourar o limite e recebem o erro claro de timeout.
- **P-4 = manter o `mcpOllama` só neste projeto** (`.mcp.json` da raiz). Sem escopo de usuário e sem troca do nível `local` do `CLAUDE.md` global (`ia-local.ps1`/`qwen3.8:27b-64k` continua). Consequência registrada: a disputa de VRAM entre `qwen3-ia` e `qwen3.8:27b-64k` continua (cargas a frio de ~22–36 s ao alternar).
- **Docs:** "provisório" removido do `--timeout` no `CLAUDE.md` e no `README.md`.
- **Pendências restantes, todas do owner e nenhuma bloqueante:**
  - P-8 (`rank_files` e tools assíncronas);
  - P5-2 (Cursor/VS Code, opcional);
  - P5-3/P5-4 (rotação da chave do TrueNAS e da senha `sa`);
  - reparo da GPU 3 (voltar a 4 GPUs e 131072);
  - versionar o pack `.cursor/`.
- **Plano:** todas as fases (0–6) concluídas.
- **Commit:** `19c3b8c`.

### 2026-10-26 11:25 — "só modelos otimizados para o hardware" (decisão do owner)
- **Evidência (medida):**
  - `qwen3-ia` (MoE 30,5B, Q4_K_M): **100% na GPU** (20,17 de 20,17 GiB, `/api/ps` às 11:17), ~97 tok/s;
  - `qwen3.8:27b-64k` (denso 27,3B): 11,0 GiB em VRAM para 16,5 GiB de pesos (`list_models` de 24/10), ou seja, **offload para a CPU**, ~22 tok/s (guia do servidor);
  - `nomic-embed-text`: 0,85 GB, 100% na GPU;
  - `qwen3-coder:30b-a3b-q4_K_M`: base redundante do `qwen3-ia`.
- **Abrangência decidida (AskUserQuestion, as 4 opções):**
  1. allowlist no mcpollama;
  2. nível local (`ia-local.ps1`);
  3. `CLAUDE.md` global;
  4. remover os outros do servidor. O owner depois autorizou: "pode remover você".
- **Feito:**
  1. **Nível local:**
     - backups `*.bak-20261026-1120` de `ia-local.ps1`, `csl-ollama.env` e `CLAUDE.md` global;
     - no `csl-ollama.env`, só a linha `OPENAI_MODEL` trocada (`qwen3.8:27b-64k` → `qwen3-ia`), por script, sem exibir o token. As 4 chaves foram preservadas e o CRLF mantido;
     - comentário do cabeçalho do `ia-local.ps1` atualizado; o contexto (65536) já casava;
     - **testado:** `-Check` OK com `model=qwen3-ia`; tarefa só de texto em **3,6 s**; tarefa com a ferramenta `Read` (ler o `README.md` e resumir) correta em **5,7 s** (antes ~40–90 s). O tool calling pela API OpenAI-compatível funciona.
  2. **`CLAUDE.md` global:** linha do nível `local` com o modelo novo e os tempos medidos. **`CLAUDE.md` do projeto:** o nível `local` agora usa o mesmo `qwen3-ia`, e a disputa de VRAM acabou.
  3. **Servidor** (1 conexão SSH `lx-ai01`, usuário de operação, sem root):
     - antes: 5 modelos, disco 55 GB usados;
     - FROM do `qwen3-ia` = FROM da base = `sha256-1194192c…` (pesos compartilhados), condição para remover a base;
     - `ollama rm qwen3.8:27b-64k qwen3.8:27b-q4_K_M qwen3-coder:30b-a3b-q4_K_M`: os 3 "deleted";
     - depois: `qwen3-ia` e `nomic-embed-text`, **disco 39 GB usados (~16 GB liberados)**;
     - teste local no servidor: `qwen3-ia` → "ok" (`load_duration` 20,6 s, porque a remoção o tirou da VRAM).
     - **2ª via (`list_models` do mcpollama):** instalados apenas `qwen3-ia` e `nomic-embed-text`; `qwen3-ia` em VRAM com 20,2 GiB e ctx 65536.
- **Não deu certo (sem efeito no resultado):**
  - o pipe do PowerShell 5.1 para o `ssh` pôs um **BOM** na 1ª linha (`set -u` falhou) e um `\r` na última (`ollama ps` falhou);
  - o `ls` do blob deu "Permissão negada" (o usuário `claiton` não lê `/usr/share/ollama`); a integridade foi provada pelo `generate` e pela 2ª via.
  - **Lição:** para enviar script por `ssh` a partir do PowerShell 5.1, gravar o script em arquivo UTF-8 sem BOM com LF e usar `ssh host 'bash -s' < arquivo` (via `cmd /c`), ou `[Console]::OutputEncoding` sem BOM.
- **Pendências:**
  - allowlist no código do mcpollama (agente Sonnet em execução);
  - o owner atualizar a tabela de modelos do `CONEXAO-IA.md` (§4).

### 2026-10-26 11:35 — allowlist de modelos no mcpollama (agente Sonnet + correção e verificação do loop principal)
- **Feito (agente):**
  - `src/tools/allowlist.go`: `SplitModels` (`;` ou `,`), `ModelAllowed` (exato, `:latest` implícito, case-sensitive), `checkModel`/`modelArg`;
  - as 6 tools com `model` passam por `modelArg` **antes** de qualquer acesso ao Ollama;
  - `list_models` filtra pela allowlist, mostra "Fora da allowlist (ocupa VRAM)" quando houver e "Modelos permitidos neste servidor";
  - flag `--allowed-models`/`MCPOLLAMA_ALLOWED_MODELS` com validação na partida (exit 2);
  - testes novos em `allowlist_test.go` e `config_test.go`; `TOOLS.md` regenerado.
- **Defeito de especificação (meu), achado na verificação:** o prompt pedia o default "= `--model` + `--embed-model` efetivos". Assim, qualquer `--model` entrava na lista e a partida com `--model qwen3.8:27b-64k` **subia (exit 0)**: a restrição "só modelos otimizados" não valia.
  - **Correção:** o default passou a ser o **conjunto fixo `qwen3-ia;nomic-embed-text`** (`config.go`, `allowedModelList`); usar outro modelo exige `--allowed-models` explícito.
  - O teste antigo `TestAllowedModelsDefaultsToEffectiveModels` foi substituído por `TestAllowedModelsDefaultIsTheTunedSet`, que cobre: default, `--model` fora → erro de partida, e opt-in explícito com `--allowed-models`.
- **Verificação do loop principal (lida):**
  - gate limpo: build, `vet` e `vet -tags live` = 0; **262 PASS, 0 FAIL, 1 SKIP**; `-race` com 0 DATA RACE (rodado de novo depois da correção); `gofmt` vazio; `dumptools -check` em sincronia;
  - binário `0.5.0-19c3b8c`;
  - partida com `--model qwen3.8:27b-64k` → **exit 2** "não está na lista de modelos permitidos (--allowed-models: qwen3-ia, nomic-embed-text)"; partida default → exit 0;
  - **ao vivo (servidor real, token só em memória):**
    - `generate` com `model: qwen3.8:27b-64k` → `isError` "modelo qwen3.8:27b-64k não permitido neste servidor; permitidos: qwen3-ia, nomic-embed-text";
    - `generate` default → "ok" em `total_ms=55`;
    - `list_models` → só os 2 modelos e a lista de permitidos;
    - token na saída: 0.
- **Docs:** `README.md` (linha `--allowed-models` na tabela) e `CLAUDE.md` (flag na lista de configuração).
- **Observação:** a sessão atual do Claude Code continua com o processo antigo do servidor (nome `mcpOllamaCSL`, binário anterior). O binário novo e o nome `mcpOllama` valem a partir do próximo início do Claude Code.
- **Commits:** `6acd0d3` (allowlist + docs) e `6596bec` (`.gitignore` com `*.exe~`).

### 2026-10-26 11:32 — encerramento (ordem do owner: "ok, finalize")
- **Verificação após o reinício do Claude Code pelo owner:**
  - as tools carregaram como `mcp__mcpOllama__*`, confirmando o nome novo;
  - o `list_models` traz a linha "Modelos permitidos neste servidor: qwen3-ia, nomic-embed-text", que só existe na 0.5.0, confirmando o binário novo;
  - o processo `mcpollama-mcp` em execução é `E:\MCPOllam\mcpollama-mcp.exe`, e `--version` = `0.5.0-19c3b8c`;
  - o `mcpollama-mcp.exe~` antigo foi apagado (já não estava em uso).
- **`CONEXAO-IA.md`** (cópia local `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\`, fora do repositório): linha "Modelos disponíveis" atualizada (só os otimizados; os 3 removidos e o porquê; allowlist do MCP) e data de atualização. A seção de 3 GPUs/64k já estava em dia, atualizada pelo owner no incidente. **A cópia no servidor (`/root/CONEXAO-IA.md`) não foi atualizada**: exigiria SSH como root.
- **Estado final:**

  | Fase / frente | Estado |
  | --- | --- |
  | 0 a 6 | concluídas |
  | Só modelos otimizados (mcpollama, nível local, `CLAUDE.md` global, servidor) | concluída |
  | Binário em uso | `0.5.0-19c3b8c` |

- **Pendências que ficam (todas do owner, nenhuma bloqueante), numeradas:**
  1. **P5-3:** rotacionar a API key do TrueNAS (exposta nos `args` do `mcpTrueNas`) e movê-la para `env`;
  2. **P5-4:** rotacionar a senha `sa` do SQL Server e limpar as regras herdadas do `.claude/settings.local.json`;
  3. reparar a GPU 3 (riser, cabo ou PSU) e depois voltar a 4 GPUs, `num_ctx` 131072 e o teste de prompt grande;
  4. copiar o `CONEXAO-IA.md` atualizado para `/root/` no servidor;
  5. opcionais: P-8 (`rank_files` e tools assíncronas), P5-2 (Cursor/VS Code), versionar o pack `.cursor/`, e trocar as senhas de `referencias/MCPVMWare/.mcp.json`.

### 2026-10-26 11:40 — documentação do servidor atualizada (ordem do owner) + correção do `ollama-warm`
- **Conexões SSH:** 3, todas `lx-ai01-root`, por chave, para inventário, download e upload. Mais a de 11:25 (`lx-ai01`), são 4 no dia, sem incidente de fail2ban.
- **Inventário:**
  - 3 documentos em `/root` (`CONEXAO-IA.md` 600, `OLLAMA-SETUP.md` 644, `OLLAMA-SETUP-DETALHADO.md` 644), todos com menções desatualizadas;
  - varredura de segredos nas cópias: `known_hits=0`; as strings longas eram só caminhos;
  - o `CONEXAO-IA.md` do servidor = a cópia local menos as 2 edições de hoje, sem divergência.
- **Achado:** `/usr/local/bin/ollama-warm` + `ollama-warm.service` (enabled) **pré-carregava o `qwen3.8:27b-64k` no boot**:
  - depois da remoção, falharia no próximo reboot (`set -euo pipefail` + `curl -sf`);
  - explica o `qwen3.8` residente sozinho depois dos reboots do incidente (inferência compatível com o observado em 24/10).
  - **Decisão do owner:** trocar para `qwen3-ia`.
- **Feito:**
  1. `ollama-warm`: modelo → `qwen3-ia` (mantidos `num_ctx` 65536 e `num_predict` 1), mais o comentário; `Description` do service atualizada.
  2. 3 documentos editados por um agente Sonnet com a lista de fatos medidos:
     - modelos (só os 2 otimizados, a remoção e o porquê, ~16 GB liberados);
     - 3 GPUs/64k atual e 4/128k como alvo pós-reparo;
     - `ollama-warm`;
     - clientes (`mcpOllama` 0.5.0 com allowlist e nível `local` com `qwen3-ia`);
     - **chaves SSH em `D:\Users\claiton.linhares\.ssh\`** (o owner informou durante a tarefa; os documentos diziam `C:\`);
     - datas de atualização.
     - O conferido pelo loop principal: `num_gpu 999`/`num_ctx 65536` batem com o `/api/show` de 24/10; as menções a `qwen3.8` que sobraram são só históricas; 0 `C:\Users\...\.ssh`; varredura 0; `bash -n` do script ok.
  3. **Upload (1 conexão):**
     - backup dos 5 originais em `/root/doc-backup-20261026/`;
     - `install` preservando dono/modo (600/644/755/644);
     - **md5 no servidor igual ao local nos 5** (`3f639f3d4dd4`, `e85a724e1d70`, `93038c64e911`, `ba909fd49075`, `12a92b37df1b`);
     - `bash -n` ok; `daemon-reload` ok; `ollama-warm` enabled;
     - **`ollama-warm` executado: exit 0**, `/api/ps` com o `qwen3-ia` residente;
     - `/tmp/docup` removido.
  4. Cópia local `D:\...\.ssh\.workspace\Backup\ia\CONEXAO-IA.md` sincronizada (md5 igual ao do servidor). Os 2 `OLLAMA-SETUP*` existem só no servidor.
- **Ressalva:** o `OLLAMA-SETUP-DETALHADO.md` tinha fim de linha misto (420 CRLF e 13 LF), e a edição normalizou tudo para CRLF. Não afeta a leitura; o original está no backup.
- **Pendência (4) da lista acima, copiar o `CONEXAO-IA.md` para `/root`:** **fechada.**
- **Commit:** `4b98b34`.

### 2026-10-26 11:50 — token como argumento e executável em `D:\MCP` (decisões do owner)
- **Decisões (ordem direta do owner):**
  1. "OLLAMA_API_KEY tem que ser um argumento do executável": o token passa de `env` para `--api-key` nos `args` das configs dos clientes. O código já aceitava `--api-key`; nada mudou em `src/`. A P-1 continua: sem `--api-key-file`.
  2. "o exe não vai ficar na pasta do projeto, vai ficar aqui D:\MCP": o executável passa a ser `D:\MCP\mcpollama-mcp.exe`, junto de `mcpvmware-mcp.exe` e `TruenasMcp.exe`, que já estavam lá.
- **INCIDENTE (do owner, registrado sem o valor):** ao colar o `.mcp.json` no chat, o owner expôs o **token do Ollama em texto puro** na conversa. **Recomendação: rotacionar o token** (fonte `/root/API-Key` 2ª linha, cópia do nginx `/etc/nginx/ollama/api-key` e o mapa `auth.conf`; nos clientes, o `csl-ollama.env` e as 3 configs). **Pendência P6-1.**
- **Feito:**
  - nas 3 configs (`.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json`), por script, sem exibir o token: `--api-key <token>` acrescentado aos `args`, `env` removido e `command` = `D:\MCP\mcpollama-mcp.exe`. Conferido: JSON válido, 8 args, o `--api-key` seguido do token, sem `env`, o `command` existe, e os 3 continuam ignorados pelo git;
  - binário copiado para `D:\MCP\` (md5 igual ao da raiz; `--version` = `0.5.0-19c3b8c`);
  - **teste real** (`claude -p --mcp-config .mcp.json --strict-mcp-config`, Haiku): `list_models` → "Instalados (2):", exit 0. O token, passado por argumento, autenticou (sem 401); token na saída: 0. Antes do `D:\MCP`, o mesmo teste também passou com o binário da raiz.
- **Docs:**
  - `README.md`: exemplo de config com `command` em `D:\MCP`, `--allow-root` e `--api-key` nos `args`; nota de segurança (decisão do owner, o risco da lista de processos e as 3 configs no `.gitignore`); linha `--api-key` da tabela;
  - `CLAUDE.md`: local do executável, build com `-o D:\MCP\...` (Windows e Linux), smoke a partir de `D:\MCP`, nota do `.exe~`, Fase 5 e lista de flags.
- **Pendências:**
  - **P6-1:** rotacionar o token do Ollama;
  - **P6-2:** depois de reiniciar o Claude Code, que hoje ainda roda o binário da raiz, apagar o `E:\MCPOllam\mcpollama-mcp.exe` antigo, que deixa de ser usado.

### 2026-10-26 12:25 — teste de stress (ordem do owner: "faça um teste de stress")
- **Decisões do owner (AskUserQuestion):** os três tipos (mcpollama local com mock; ao vivo moderado; servidor no limite, com risco de reboot). Em caso de queda: "Parar, investigar logs e tentar 1 vez de novo".
- **Execução:**
  1. **Mock** (`src/mcpollama-mcp/stress_test.go`, build tag `stress`, 6 cenários; escrito por um agente Sonnet). 2ª execução no loop principal: `go test -tags stress -count=1 -v -run Stress ./mcpollama-mcp` → exit 0, **8 PASS / 0 FAIL**. Resultados: 500 chamadas concorrentes sem perda nem duplicata, concorrência máxima no backend 1, `ping` p99 1 ms, cancelamento em massa correto, goroutines 6→6, linha de 8 MB recusada sem derrubar o servidor, map-reduce sem passar do teto.
  2. **Ao vivo:** driver `stressdrv` (cliente MCP stdio real, token por scrub).
     - Linha de base às 11:51.
     - Rajada: **20/20 OK**, p95 111,7 s, `ping` máx. 1 ms.
     - Sustentado de 10 min: **148/148 OK**, p95 15,3 s, fila máx. 14,7 s, ~1,07 milhão de tokens de entrada.
  3. **Limite:**
     - **1ª tentativa (12:07): o servidor estava fora do ar.** Houve um reboot **ordenado** às 12:07, entre o sustentado e o limite, antes de qualquer prompt grande. As 6 chamadas falharam ao conectar, e a 7ª estourou o timeout. O driver parou, como manda a decisão do owner.
     - **Investigação** (2 conexões SSH root):
       - não houve `crash`;
       - o `/var/log/messages` não tem Xid nem AER antes da queda;
       - havia uma shell root interativa (`sudo su -` às 11:54, vinda de 10.100.2.30);
       - a topologia PCIe mudou;
       - o journal não persistiu o boot anterior.
     - **Nova tentativa (12:13): 8/8 OK.** Foram 5 prompts de 58.783 tokens em ~21–24 s cada e 3 gerações de 7.000 tokens em ~87 s cada.
     - **2ª via (SSH 12:19):** sem reboot, `NRestarts=0`, Xid 0, AER não-corrigível 0, 6 AER corrigíveis em `00:1b.4`.
- **Deu certo:** o mcpollama aguentou todos os cenários **sem nenhum defeito de código**. O Ollama, com 3 GPUs e 64k, aguentou prompts de ~90% do contexto. A 1ª tentativa também provou, ao vivo, que o cancelamento por timeout chega ao Ollama (`srv stop: cancel task`).
- **Não deu certo:** a 1ª tentativa do limite foi perdida pelo reboot de 12:07, e a causa não está provada (provável intervenção manual). Os logs do desligamento se perderam porque o journal não está persistente.
- **Achados:**
  - **A1:** a espera na fila conta para o `--timeout`;
  - **A2:** o journal não persiste entre boots;
  - **A3:** uma geração de 7.000 tokens (~87 s) fica perto do timeout de 120 s;
  - **A4:** a GPU com defeito é excluída por **índice**, a topologia mudou e não há UUID registrado (índice 3 hoje = `06:00.0` / `GPU-d7166024-…`).
- **Artefatos:** relatório `.workspace/reports/MCPOllam2026-10-26-122500-teste-stress.report.md` e evidências `.evidencias/` (varredura de segredos: `known_hits=0`; a ocorrência genérica analisada é falso positivo).
- **Pendências:**
  - **P7-1:** o owner confirmar o reboot de 12:07;
  - **P7-2:** excluir a GPU com defeito pelo UUID e registrar o UUID no `CONEXAO-IA.md`;
  - **P7-3 (decisão opcional):** contar o `--timeout` só a partir da saída da fila;
  - **P7-4:** journal persistente (`Storage=persistent`);
  - **P7-5:** remover a linha duplicada `CUDA_VISIBLE_DEVICES=0,1` do drop-in do `ollama.service`.

### 2026-10-26 12:55 — ativação da 4ª GPU (pedido do owner: "ativei a outra placa, pode analisar se ficou ok")
- **Análise inicial (12:25):**
  - A GPU 3 (`06:00.0`, `GPU-d7166024-…`) estava ligada direto na porta `00:1c.3`, com 0 AER, mas o Ollama ainda a excluía.
  - O contexto estava fixado em 3 lugares: drop-in, Modelfile `num_ctx` e `ollama-warm`.
- **Decisões do owner (AskUserQuestion):**
  - "4 GPUs e volta a 131072";
  - o reboot de 12:07 foi a manutenção física dele (**P7-1 fechada**);
  - depois da 1ª falha: "teste novamente";
  - depois da 2ª falha: "Restaurar o backup exato".
- **Execução:**
  1. **Aplicação (12:27):**
     - backup em `/root/gpu4-backup-20261026/`;
     - 4 UUIDs, 131072 nos 3 lugares;
     - carga OK: 4 GPUs, 100% GPU, 22 s.
  2. **Teste no limite (5 × ~115 mil tokens), duas execuções:** reboots às 12:29, 12:31, 12:32, 12:37 e 12:42, ou seja, **5/5 quedas**.
  3. **Journal (agora persistente):** enxurrada de RxErr na porta **`00:1b.4` = GPU `03:00.0` (`GPU-13d97d49-…`)**, com 51 e 279 eventos até o corte. Um dos reboots aconteceu durante a carga do modelo, sem prompt.
  4. **Restauração (12:52):**
     - backup exato aplicado: índices 0,1,2 e 64k;
     - validação com 1 prompt de 58.783 tokens: OK em 21,9 s;
     - 2ª via: sem reboot, Xid 0, mas **`00:1b.4` com 8 AER corrigíveis** depois de 1 chamada.
- **Deu certo:**
  - a GPU reativada (`06:00.0`) está sã;
  - a restauração voltou a um estado estável;
  - o journal persistente permitiu provar a porta culpada.
- **Não deu certo:**
  - 4 GPUs/131k é inviável enquanto o link `00:1b.4` não for reparado;
  - **erro meu:** o driver só reconhecia erros de conexão em inglês e não parou na 1ª queda, o que causou 3 reboots a mais. **Solução:** no modo limite, o driver para em qualquer erro (confirmado às 12:41).
- **Correção de um erro meu:** a "linha duplicada `CUDA_VISIBLE_DEVICES=0,1`" (P7-5) estava só no `.bak` e nunca esteve ativa. A P7-5 não procede.
- **Artefatos:**
  - relatório `MCPOllam2026-10-26-122500-teste-stress.report.md` §6;
  - evidências: 18 arquivos novos em `.evidencias/`;
  - varredura de segredos: `known_hits=0` (a ocorrência genérica já analisada é falso positivo).
- **Pendências:**
  - **P7-6:** reparo físico do link da GPU `03:00.0` (porta `00:1b.4`);
  - **P7-7:** decisão sobre excluir a `03:00.0` por UUID até o reparo;
  - **P7-8:** ciclo de ordenação `nvidia-powercap` ↔ `ollama-warm` (o warm não roda na partida);
  - **P7-9:** journald `SyncIntervalSec` curto;
  - **P7-10:** registrar o mapa UUID/barramento/porta e o episódio no `CONEXAO-IA.md`, com autorização do owner;
  - **P7-2** continua aberta: a exclusão é por índice, e o índice 1 é a placa ruim.

### 2026-10-26 13:11 — nova repetição: 4 GPUs com 64k aprovada (pedido do owner: "repita o teste")
- **Encontrado:** o owner tinha mudado o drop-in às 12:57 para `0,1,2,3` com 64k, mas sem `daemon-reload`, então o serviço seguia com 3 GPUs.
- **Aplicado:** `daemon-reload` e restart. Resultado: 4 GPUs, 100% GPU, carga em 22 s.
- **Teste no limite: 8/8 OK.** Os prompts de 58.783 tokens rodaram em ~18–20 s (mais rápido que com 3 GPUs) e as gerações de 7.000 tokens em ~87–89 s.
- **2ª via:**
  - sem reboot, Xid 0, 0 erros não-corrigíveis;
  - `00:1b.4` com +3 corrigíveis (16 no total);
  - GPU reativada (`00:1c.3`) com 0.
- **Deu certo:** 4 GPUs com 64k é estável. A combinação que cai é 4 GPUs com 131k.
- **Não deu certo:** nada nesta etapa. O link `00:1b.4` segue marginal, com ~1 erro corrigível por chamada pesada.
- **Pendências:**
  - **P7-6** continua: reparo físico antes de tentar 131k de novo;
  - **P7-2** suspensa: com as 4 GPUs em uso, a exclusão por índice não é risco;
  - **P7-8, P7-9 e P7-10** continuam.
- **Artefatos:** relatório §7; 9 evidências novas.

### 2026-10-26 13:35 — potência (170→200→170 W) e carga máxima contínua por 10 min (pedidos do owner)
- **200 W** (13:20):
  - corte seco no 1º prompt pesado (~644 W nas GPUs), reboot às 13:21, sem Xid nem AER;
  - inferência: fonte no limite (não provada);
  - decisão do owner: voltar a 170 W.
- **Erro meu:**
  - o backup `.170w`, feito ~10 s antes do corte, ficou com 0 bytes (gravação do XFS perdida no
    corte). A reversão o copiou e esvaziou a unit, que o systemd viu como `masked`;
  - também atribuí o mascaramento ao owner antes de investigar, e estava errado;
  - **solução:** unit recriada a partir da cópia íntegra, `sync`, 170 W em vigor, backup regravado.
    Os demais arquivos editados hoje estão íntegros.
- **Teste máximo de 10 min a 170 W** (modo `max` novo no driver: 2 workers, prompts únicos de ~58
  mil tokens, sem intervalo):
  - 22 chamadas OK; **reboot às 13:32, depois de ~5 min**;
  - GPUs com média de 548 W e pico de 624 W; GPU 0 a 88 °C;
  - 42 linhas de AER corrigível na `00:1b.4`, em frequência crescente, com `Data Link Layer` e
    `Multiple`; 0 Xid.
- **Deu certo:**
  - o driver agora para na 1ª falha, e encerrei o worker restante para não carregar o servidor
    recém-reiniciado;
  - o monitor de potência gravou a evidência até o corte.
- **Não deu certo:** a configuração atual (4 GPUs, 64k, 170 W) **não aguenta carga máxima
  contínua**. Ela aguenta uso real, com intervalos entre as chamadas.
- **Pendências:**
  - **P8-1:** potência e modelo da PSU;
  - **P8-2:** teste para isolar a causa (4 GPUs a 130 W **ou** 3 GPUs sem a `03:00.0` a 170 W);
  - **P8-3:** refrigeração da GPU 0;
  - **P7-6** e **P7-8** continuam.
- **Artefatos:** relatório §8; 18 evidências novas (incluindo `power-200w.csv`, `power-170w.csv` e
  `w170-max10m.txt`).

### 2026-10-26 13:47 — 150 W: carga máxima de 10 min aprovada (pedidos do owner: "coloque em 150W" e "repita o ultimo teste")
- **Aplicado:** 150 W persistente. Antes de gravar, conferi que a unit não estava vazia; gravação atômica e `sync`.
- **Teste máximo de 10 min: 30/30 OK** (630 s, ~1,76 milhão de tokens de prefill).
  - prefill de 20,2 s por 58,8 mil tokens, só ~1,8% mais lento que a 170 W;
  - GPUs com média de 518 W e pico de 566 W; GPU 0 com máximo de 83 °C.
- **2ª via:** sem reboot, Xid 0, 0 erros não-corrigíveis.
- **Alerta:** **1.471 erros corrigíveis na `00:1b.4`** (91 de `Data Link Layer`); as demais portas com 0.
- **Deu certo:** 4 GPUs, 64k e 150 W aguentaram a carga máxima contínua, e é a configuração em uso.
- **Não deu certo:** nada falhou. O link `00:1b.4` está muito degradado.
- **Pendências:**
  - **P9-1 (urgente, físico):** reparo do link da GPU `03:00.0`;
  - **P9-2:** monitorar os erros AER da `00:1b.4`;
  - **P8-1** (PSU), **P8-3** (GPU 0), **P7-6**, **P7-8**, **P7-9** e **P7-10** continuam.
- **Artefatos:** relatório §9; 6 evidências novas.

### 2026-10-26 14:30 — resposta ao report de integração da sessão GestorERP (`MCPOllam2026-10-26-141000-teste-integracao-claude-code.report.md` §7)
- **Decisões do owner (AskUserQuestion):**
  - P1: `MAX_LOADED_MODELS=2` mais documentação;
  - P2–P4: medir;
  - `qwen3-coder:30b-a3b-q4_K_M` (recriado às 12:57 junto da restauração do `qwen3-ia`): manter;
  - P5: adicionar a raiz do GestorERP.
- **P1 (fechado):**
  - **Servidor:** `OLLAMA_MAX_LOADED_MODELS=2` (backup `.maxloaded1`, gravação atômica, `sync`, restart). Aceite pelo cliente real: `embed`→`generate` com `load_ms=0` em 3/3; 2 modelos residentes.
  - **Código:** descrição do `embed` atualizada (`src/tools/embed.go`); `TOOLS.md` regenerado.
  - **Gate:** `vet` OK, `go test -count=1 ./...` verde, `tools/list` = 4.309 de 4.400 bytes. A 1ª versão estourou os testes de orçamento e foi encurtada.
- **P2 (fechado):** a "alucinação" vinha da **linha 8 do README, desatualizada** ("Estado: concepção ... src/ ainda vazia").
  - Corrigido no README: linha 8, diagrama do §2 com 4 GPUs e 150 W, linha "Contexto" do §5.
  - "Concepção" nos resumos: 8/20 → **0/20**. Sem mudança de código.
- **P3 (medido):** direto 5/25; "passo a passo + `RESULTADO:`" **15/15**, com ~90–140 tokens a mais. Recomendação de uso; documentar fica para o owner.
- **P4 (aprovado):** cos12 = 0,552 > cos13 = 0,336 e > cos23 = 0,486.
- **P5 (feito):**
  - `.mcp.json` real do GestorERP: `--allow-root "E:\MCPOllam;E:\Dropbox\Claiton Linhares\GestorERP"`.
  - Teste ponta a ponta (`claude -p` com Haiku e a config do GestorERP): leitura do `CLAUDE.md` OK; leitura do `.mcp.json` **recusada** pela lista fixa.
  - Falta reiniciar a sessão do GestorERP para valer lá.
- **Não deu certo:**
  - 2ª rodada de medição com **34/71 HTTP 429** (limite de taxa do nginx com respostas do cache de ~44 ms). Descartada; a 3ª, com 1 s entre chamadas, deu 71/71.
  - O argumento do `claude -p` quebrou nas aspas duplas (PowerShell 5.1). Refeito com aspas simples.
- **Erro meu:** em uma mensagem de status escrevi "14:22" no cabeçalho, horário inventado (o hook seguinte marcou 14:21:00).
- **Artefatos:**
  - report §7;
  - evidências: 10 arquivos em `…141000-teste-integracao-claude-code.evidencias/` (varredura: 0 segredos conhecidos; as 47 ocorrências genéricas são menções a "token" e `--api-key`);
  - `README.md`, `src/tools/embed.go`, `TOOLS.md`.
- **Pendências:**
  - **P10-1:** reiniciar o Claude Code no GestorERP;
  - **P10-2 (opcional):** documentar a recomendação "passo a passo" na descrição de `generate`/`chat`;
  - **P10-3:** recompilar o binário em `D:\MCP` com a nova descrição do `embed`;
  - **P9-1** (link `00:1b.4`, 1.932 erros às 14:16) continua.
- **P10-3 fechada (14:29):** binário D:\MCP\mcpollama-mcp.exe =  .5.1-0b2aab6 (o antigo foi renomeado para .exe~ pelo go build, porque estava em uso). Smoke do 	ools/list OK, com a nova frase do embed. Os clientes pegam a versão nova ao reiniciar; depois disso, apagar o .exe~.

### 2026-10-26 14:50 — §8 do report de integração (sessão GestorERP) + resposta §9, R1 e R2
- **Correções ao §8:**
  - o binário testado era o `0.5.1-0b2aab6` (processo iniciado às 14:35:01; exe gravado às 14:29:25);
  - os commits `0b2aab6` e `b9ec039` já existiam;
  - R3: o restart das 14:16 **não** explica a mudança. Deu 401 às 14:18 e 415 às 14:21 e 14:23, sem restart entre elas; a saída é estável dentro de uma rodada e varia entre rodadas.
- **Decisões do owner:** R1 medir; R2 documentar só no `generate`.
- **R1:** contar direto 15/30 (50%); listar em JSON e contar no script **15/15**.
  - **Erro meu:** a 1ª rodada da listagem falhou 15/15 por bug no parser (cortava no `]` da linha `[arquivo]`). Corrigido e refeito.
- **R2:** frase no `generate` (commit `10e879a`). `tools/list` = 4.390 de 4.400 bytes (sobram 10). Binário `D:\MCP` = `0.5.2-10e879a`, smoke OK.
- **Artefatos:** report §9 (`c72d9df` + este commit); evidências `r1-count.txt` e `r1-count2.txt`.
- **Pendências:**
  - **P11-1:** reiniciar os clientes para carregarem o 0.5.2 e depois apagar o `.exe~`;
  - **P11-2:** o próximo texto em qualquer tool exige rever `toolsListBudget`;
  - R4.2 (cofre / CLAUDE.md do GestorERP) e P9-1 (PCIe) continuam.

---

## Fase 7 — Extração de documentos (`.docx`, `.pptx`, `.pdf`) + OCR

- **Brief:** `.workspace/plans/MCPOllam2026-10-28-132000-extracao-docx-pdf-pptx.brief.md`. Ele é a SSOT dos requisitos; este plano registra as decisões, a execução e as pendências.
- **Estado:** em andamento (7.0).

### 2026-10-28 14:15 — decisões do owner (AskUserQuestion) e instalação das DLLs
- **Decisões:**
  - **D-1/D-2:** go-pdfium em modo WebAssembly (wazero), **condicionado ao spike 7.0**. É a 1ª dependência externa do `go.mod`, só para PDF, com versão fixada e licenças a confirmar. DOCX e PPTX ficam só com a stdlib.
  - **D-3:** flag nova `--max-doc-bytes` / `MCPOLLAMA_MAX_DOC_BYTES`, default 32 MiB (33554432), só para `.docx`, `.pptx` e `.pdf`.
  - **D-4:** OCR na etapa 7.9; até lá, PDF sem texto é recusado com mensagem clara.
  - **D-5:** notas do apresentador e comentários do Word são incluídos, sempre rotulados.
  - **D-6:** versão 0.6.0 (e 0.7.0 com OCR).
  - **D-13:** teto do `TestToolsListBudget` sobe para 4.700 bytes; descrições com o acréscimo mínimo ("text (UTF-8), .docx, .pptx or .pdf file"); orientações e avisos vão na resposta da tool.
  - **D-8:** usar a `libtesseract64.dll` **5.3.3 pronta**. Compilar a 5.5.2 vira pendência opcional.
  - **D-11:** copiar os modelos `best` e `fast`; o padrão sai da medição (§11.6 do brief), com escolha pela flag `--ocr-tessdata`.
  - **D-9/D-10:** OCR em processo filho (`--ocr-worker` + Job Object), **só no Windows**; no Linux, "OCR indisponível".
  - **D-12:** **sim**, `.png` e `.jpg` também entram na 7.9 (stdlib, `DecodeConfig` antes de decodificar, teto de megapixels); `.tif` fica de fora.
  - **D-7** (ligar OCR, opt-in por `--ocr-lib`): implícita na escolha da D-8; confirmar na 7.9.
- **Instalação (ordem do owner: "coloque as DLLs necessárias no mesmo local do .exe"):**
  - `D:\MCP\libtesseract64.dll`:
    - 5.965.824 bytes;
    - SHA-256 `964B0972348F6283B1721F9131DA59721CE46FB4F7A4E294399B1169C7E71669`;
    - procedência: `referencias/tessract-group/libtesseract64.dll`, binário de repositório Delphi de terceiro;
    - conferido com `objdump -p` (deps MinGW): PE32+ x64; importa **só** `KERNEL32.dll` e `msvcrt.dll`; exporta `TessVersion`, `TessBaseAPIInit3`, `SetImage`, `GetUTF8Text`, `MeanTextConf`…
  - `D:\MCP\tessdata\best\por.traineddata`: 8.159.939 bytes, `711DE9DB…ADB04`.
  - `D:\MCP\tessdata\best\osd.traineddata`: 10.562.727 bytes, `9CF5D576…96B00FF`.
  - `D:\MCP\tessdata\fast\por.traineddata`: 1.982.756 bytes, `C4932B93…ED58B7DEB`.
  - `D:\MCP\tessdata\fast\osd.traineddata`: 10.562.727 bytes, `9CF5D576…96B00FF` (o mesmo `osd` nas duas variantes).
  - Todas as cópias têm hash igual ao da origem.
  - **Desvio do brief:** o §11.5 sugeria `D:\MCP\ocr\`. Por ordem do owner, a DLL fica na raiz `D:\MCP\`, ao lado do `.exe`, e os modelos em `D:\MCP\tessdata\{best,fast}\`. O caminho é passado por `--ocr-lib` (absoluto) e `--ocr-tessdata`.
  - O go-pdfium em WASM **não** precisa de DLL: o `.wasm` vai embutido no binário.
- **Licenças:**
  - Tesseract: Apache-2.0 (`tesseract/LICENSE`);
  - Leptonica: licença própria, tipo BSD (`leptonica/leptonica-license.txt`, "Copyright (C) 2001-2020 Leptonica");
  - tessdata: Apache-2.0 (a confirmar no report da fase).
- **Pendências:**
  - **P7.0-1:** spike do motor de PDF (corpus de 15 PDFs fora do git) e report só com métricas;
  - **P7.0-2:** compilar o Tesseract 5.5.2, se o owner quiser (opcional, D-8).

### 2026-10-28 14:25 — 7.1 concluída: base do pacote `src/doctext/` (agente Sonnet + gate do loop principal)
- **Feito (só stdlib, sem I/O; nenhum arquivo fora de `src/doctext/`):**
  - `doctext.go`: `Format`, `Limits` + `DefaultLimits()`, `Result`, `Reason`, `Error`, `ReasonOf` e `Detect` (extensão **e** assinatura; OLE → senha ou 97-2003).
  - `zipsafe.go`: `OpenPackage` em memória, com limites de entradas, de bytes por entrada e de bytes totais reais. Recusa nomes com `..`, `\`, absolutos ou duplicados (sem diferenciar maiúsculas), entrada cifrada e método não suportado. Resolvedor de `.rels`; `External` é ignorado.
  - `xmlsafe.go`: decoder em streaming com limite de profundidade e de runas; recusa qualquer diretiva (DTD); helpers de namespace por URI (transicional e Strict).
  - `normalize.go`: `NormalizeText` (CRLF, BOM, controle, regra de 1% de U+FFFD).
  - Testes: `detect_test.go`, `security_test.go`, `normalize_test.go` e `fuzz_test.go`; fixtures geradas em código.
- **Gate (rodado pelo loop principal, cache limpo):**
  - `gofmt` vazio; `go vet ./...` OK; `go build ./...` OK; `go test -count=1 ./...` verde nos 8 pacotes.
  - Relato do agente: `-race ./doctext` OK; `FuzzOpenPackage` 60 s, 499.571 execuções; `FuzzXMLSafe` 60 s, 3.746.797 execuções; os dois PASS.
- **Medido no teste:**
  - zip bomb de 100 MiB com o limite de entrada padrão (64 MiB): **recusada**, mas o pico de heap chegou a **156 MiB** (`io.ReadAll` dobra o buffer);
  - com limite de 1 MiB, ou com cabeçalho mentiroso: pico menor que 1 MiB.
- **Desvios do brief, aceitos e registrados:**
  1. recusa **qualquer** `xml.Directive`, não só `DOCTYPE`;
  2. duplicata de nome detectada sem diferenciar maiúsculas;
  3. o pacote inteiro é recusado se **qualquer** entrada for cifrada ou usar método não suportado;
  4. `ReasonLegacyExcel` com mensagem própria para `.xls`;
  5. `.doc`, `.ppt` e `.xls` sem assinatura OLE seguem como texto;
  6. `MaxTextRunes` padrão de 16 Mi runas, aplicado por parte XML;
  7. não há `Format` para OLE (o `Detect` devolve erro);
  8. em `.rels`, `%2F` é decodificado; alvo com esquema ou unidade de disco recusa o pacote.
- **Pendências:**
  - **P7.1-1:** o pico de 156 MiB com o limite de 64 MiB fica dentro do critério do brief (menos de 512 MiB), mas avaliar na 7.7 baixar `MaxEntryBytes` ou ler sem dobrar o buffer;
  - **P7.1-2:** a mensagem de U+FFFD é de PDF; para DOCX e PPTX ficaria imprecisa (ajustar na 7.6);
  - **P7.1-3:** `Has("word/document.xml")` e `Has("ppt/presentation.xml")` entram nas etapas 7.2 e 7.4;
  - **P7.1-4:** o teto de texto somando as partes, com `ReasonTooLong`, fica com o `Extract`.
- **Observação:** há arquivos de outras frentes no working tree, que **não** entraram no commit: o report de integração alterado e 2 evidências novas, o brief ainda não versionado e um `MCPOllam.rar` na raiz.

### 2026-10-28 14:30 — 7.0 spike do motor de PDF (1ª rodada) + decisões do owner
- **Report:** `.workspace/reports/MCPOllam2026-10-28-142444-spike-motor-pdf.report.md` (agente Sonnet). Só métricas: o loop principal conferiu 0 CPF, CNPJ ou "R$" e nenhum caminho de cliente. Corpus em `.workspace/spike-pdf/` (ignorado), renomeado `c01`…`c15`.
- **Resultado (15 PDFs: 8 com texto, 6 só imagem, 1 não abriu em nenhum motor):**
  - A (go-pdfium v1.21.4 WASM, wazero v1.12.0):
    - 52.288 runas extraídas;
    - 21,5 acentos por 1000 runas; **0 U+FFFD**;
    - 23 ms por PDF; 0 panic; determinístico em 15/15;
    - **renderiza** (300 dpi, cinza, de 0,23 a 1,28 s por página).
  - B (ledongthuc): 36.861 runas; 17,4 acentos por 1000; **13,2 U+FFFD por 1000**; 9,7 ms por PDF; não renderiza.
  - Concordância de ordem de leitura: LCS 0,955.
  - Custos de A:
    - **~317 MB de working set** por processo;
    - **inicialização de 4,6 a 10,8 s sem cache** (0,14 a 0,19 s com o cache de compilação do wazero, 21,6 MB em disco);
    - binário mínimo de 12,4 MB (o aumento no servidor é estimado em ~+10 MB, não medido).
  - Fragilidades do corpus: só 33 páginas com texto; sem referência `pdftotext`; tabelas não avaliadas; causa da falha do c01 não confirmada.
- **Achado:** a toolchain instalada é **go1.27.0** (`GOTOOLCHAIN=auto`); o `CLAUDE.md` diz 1.26.5. O `go.mod` (`go 1.26`) é compatível. Corrigir a documentação na 7.8 (**P7.0-3**).
- **Decisões do owner (AskUserQuestion, 14:29):**
  - **D-1: ampliar o spike antes de fixar** (mais 20 PDFs com texto, incluindo tabelas e multipágina; investigar o c01; medir o working set com cache e a reabertura).
  - **D-14 (nova): cache de compilação só por flag** `--pdf-cache-dir`. Sem a flag, não há cache (a regra "não grava em disco sem flag" fica mantida). Nos `.mcp.json`, usar `D:\MCP\cache\pdfium`.
  - **D-15 (nova): liberar o motor WASM depois de 10 min ocioso** (flag `--pdf-idle`; 0 = nunca libera).
- **Estado:** 7.0 em andamento (ampliação); 7.2/7.3 (DOCX) e 7.4 (PPTX) em andamento em paralelo.

### 2026-10-28 14:45 — 7.2, 7.3 e 7.4 concluídas (agentes Sonnet em paralelo + gate e commits do loop principal)
- **Commits:**
  - `feb5561` — 7.4, PPTX: `pptx.go`, `pptx_test.go` (15 testes), `pptx_testdata_test.go`, `fuzz_pptx_test.go`;
  - `d3476a8` — 7.2, DOCX: `Extract` em `doctext.go` (dispatcher com `recover` e `ctx`), `docx.go`, `docx_test.go`, `pdf_stub.go` (stub temporário até a 7.5);
  - `9914cad` — 7.3, numeração: `docx_numbering.go`, `docx_numbering_test.go`, mais o `FuzzDOCX` em `fuzz_test.go`.
- **Gate do loop principal:**
  - com tudo junto e cache limpo: `gofmt` vazio, `vet` OK, `go test -count=1 ./...` verde, `-race ./doctext` OK (6,0 s);
  - **cada commit verificado sozinho num worktree limpo** (`go vet ./...` + `go test ./...`): os 3 verdes.
- **Erro de processo (meu), corrigido antes de qualquer push:**
  - na 1ª sequência, o commit da 7.2 (`aee916a`) **não compilava sozinho**: o `FuzzDOCX` do `fuzz_test.go` usa o helper `nbAbs`, que fica no teste da 7.3. O agente tinha afirmado o contrário, e eu commitei sem verificar;
  - a verificação em worktree pegou o erro;
  - **solução:** `git reset --soft feb5561` e nova sequência, com o `fuzz_test.go` no commit da 7.3;
  - **lição:** verificar cada commit intermediário num worktree **antes**, não depois.
- **Desvios dos agentes, aceitos e registrados:**
  - **DOCX:**
    1. notas de fim saem como `[nota de fim N]`, com referência `[fim N]` no corpo, porque os ids colidem com os das notas de rodapé;
    2. notas e comentários saem numa linha só, com o rótulo;
    3. célula de continuação de `vMerge` fica vazia (mantém as colunas); `gridSpan` sai uma vez só;
    4. `PendingRevisions` também liga com `pPrChange`, `rPrChange` e similares;
    5. em `AlternateContent`, a 1ª ramificação (`Choice`; ou `Fallback` se não houver `Choice`);
    6. o texto da caixa de texto sai antes do resto do parágrafo que a contém;
    7. extra: `m:t` (texto matemático) e `w:ptab`;
    8. a numeração trata também `pStyle` do nível, `numStyleLink`/`styleLink`, `isLgl` e o `lvl` dentro de `lvlOverride`;
    9. `w:start` ausente vale 0, como na especificação.
  - **PPTX:**
    1. nota com rel inexistente é ignorada (slide ausente recusa);
    2. `sldNum`, `dt` e `ftr` dentro do slide não são filtrados (o brief só manda ignorar o `sldNum` nas notas);
    3. `Slides` = número de entradas no `sldIdLst`; `hMerge` omitido; `vMerge` vira célula vazia.
- **Problema aberto (crítico para o critério do §7.5 do brief):**
  - nos fuzz `FuzzOpenPackage` e `FuzzPPTX`, o contador fica em **0 execuções/s** depois de 20 a 50 s; no `FuzzDOCX`, ~15 s em 0/s; o `FuzzXMLSafe` roda normal;
  - a entrada não foi capturada. Pode ser laço ou alocação no caminho do ZIP (DoS) ou artefato do motor de fuzz no Windows;
  - **delegado a um agente Opus** (14:43) para isolar a causa com dados antes de corrigir.
- **Pendências:**
  - **P7.2-1:** apagar o `pdf_stub.go` quando a 7.5 entrar;
  - **P7.2-2:** o teste do `recover` usa o hook `testHookExtract`, porque não há ponto natural de panic;
  - **P7.4-1:** avaliar se `dt`/`ftr`/`sldNum` dentro do slide devem ser filtrados (ruído);
  - **P7.x-1:** travamento do fuzz (em investigação).

### 2026-10-28 14:43 — 7.0 concluída: spike ampliado + D-1 fixada
- **Ampliação** (mesmo report, seções 12 e 13; agente Sonnet):
  - 20 PDFs novos com texto (`c16`…`c35`); corpus total de 35 PDFs, 26 com texto, 133 páginas;
  - sigilo conferido pelo loop principal: 0 CPF, CNPJ ou "R$", nenhum caminho nem nome original; `mapa.txt` ignorado pelo git.
- **Resultado total (35 PDFs):**
  - **A** (go-pdfium WASM): 255.506 runas; **0 U+FFFD**; nenhuma página perdida; 40,8 ms por PDF;
  - **B** (ledongthuc): 138.171 runas; 9,5 U+FFFD por 1000; **perdeu texto sem avisar** em 15 páginas de 4 PDFs;
  - 0 panic e determinístico (35/35) nos dois; `/Count` confere em 18/18;
  - concordância: LCS 0,961, Jaccard 0,612;
  - tabelas (6 PDFs): divergência em 4, e sem `pdftotext` não dá para saber qual leitura está certa.
- **c01:** é senha de **abertura** (RC4 40 bits, R=3; a senha vazia não valida contra o `/U`), ou seja, recusa legítima. O mesmo vale para `c16` e `c17`. **Só 1 PDF da pasta** tem senha apenas de permissão (`c02`), e ele abre.
- **Cache de compilação do wazero** (21,6 MB em disco):
  - init: **113 a 150 ms** com cache, contra 4.935 ms sem;
  - working set: **68 a 81 MB** com o motor aberto e cache, contra ~330 MB sem;
  - motor fechado: 47 a 51 MB residuais (base do processo: 7,6 MB). **Liberar o motor devolve só ~20 a 30 MB.**
- **Não medido:** referência `pdftotext` (não existe na estação); PDF do Word com texto (não existe na pasta); memória por PDF.
- **Decisões do owner (AskUserQuestion, 14:42):**
  - **D-1 FIXADA:** **go-pdfium v1.21.4 (MIT) + wazero v1.12.0 (Apache-2.0)**, em modo WebAssembly, sem cgo;
  - **D-15 REVISTA:** **motor residente** (a flag `--pdf-idle` cai). Liberar devolveria só ~25 MB, o que não compensa a complexidade;
  - continua valendo a D-14: o cache só por `--pdf-cache-dir` (sem a flag, nada em disco).
- **Pendências:**
  - **P7.0-4:** identificar a versão do PDFium dentro do `.wasm` e auditar os componentes de terceiros dele (licenças);
  - **P7.0-5:** filtrar caracteres de controle na saída do A (`c30`: 7,7 por 1000) — o `NormalizeText` já faz;
  - **P7.0-6:** não prometer fidelidade de tabela em PDF sem referência.
- **Erro meu de processo:** os cabeçalhos "14:42" e "14:46" das minhas mensagens anteriores foram **estimados**, não lidos (o horário real conferido era 14:41:50). A regra é usar só o hook ou o `Get-Date`.

### 2026-10-28 15:03 — 7.5 concluída: extração de PDF (agente Sonnet + commit e verificação do loop principal)
- **Commit `c216351`:**
  - novos: `pdf.go`, `pdf_test.go`, `pdf_testdata_test.go` (gera os PDFs em código, inclusive os cifrados RC4 40/128 bits), `fuzz_pdf_test.go`;
  - apagado: `pdf_stub.go`;
  - **`go.mod`/`go.sum`:** 1ª dependência externa: go-pdfium v1.21.4 (MIT), wazero v1.12.0 (Apache-2.0); transitivas go-commons-pool/v2 v2.1.2 (Apache-2.0), google/uuid v1.6.0 (BSD-3), x/sys v0.48.0 e x/text v0.43.0 (BSD-3). O `go 1.26` virou `go 1.26.0`, como o go-pdfium exige.
- **API:**
  - `PDFOptions{CacheDir}`;
  - `ConfigurePDF(opts)`: antes do 1º uso; depois que o motor iniciou, devolve erro;
  - `ClosePDF()`: idempotente.
  - Motor residente e preguiçoso, com semáforo de 1 vaga que respeita o `ctx`. Cada extração usa uma instância nova do pool, então nenhum estado passa entre documentos.
- **Endurecimento achado pelo agente:** o go-pdfium, por padrão, **monta o volume inteiro** no WASM e **escreve no `os.Stdout`**, o que corromperia o JSON-RPC. Foi configurado com `FSConfig` vazio e stdout/stderr descartados.
- **Verificação do loop principal (worktree limpo no commit):**
  - `gofmt` vazio; `vet` OK; `go test -count=1 ./...` verde (doctext 15,9 s);
  - `go list -deps ./mcpollama-mcp` confirma que o main ainda não importa doctext, pdfium nem wazero, então o binário não muda até a 7.6.
  - Relato do agente: `-race ./doctext` OK (180 s); `FuzzPDF` 60 s, 8.879 execuções sem falha, mas a cobertura do Go não enxerga dentro do WASM (fuzz pouco guiado).
- **Medições (agente):**
  - init **sem cache 6,0 a 6,75 s**; **com cache 123 ms**;
  - 1,15 ms por página em texto sintético (não comparável aos ~7 ms por página dos PDFs reais do spike);
  - render: 72 dpi dá 612×792; 144 dpi dá 1224×1584.
- **Desvios:**
  1. o PDFium troca o `-` de fim de linha por `\x02`. Foi tratado em `pdfiumSoftHyphens`: só junta se a próxima letra for minúscula; senão vira `-\n`. Hífen literal depois de um não-letra nunca junta;
  2. uma página que falha em `pageText` torna o documento `ReasonMalformed` (nunca perde texto em silêncio);
  3. um PDF truncado no meio pode ser recuperado pelo PDFium; o teste aceita texto ou recusa tipada, nunca panic.
- **Pendências:**
  - **P7.5-1:** o `ctx` só é checado entre páginas; uma página hostil e lenta não é interrompível, e o `--timeout` do servidor continua sendo a rede de proteção (avaliar na 7.7);
  - **P7.5-2:** sem teto de memória do WASM (~320 MB sem cache; 68 a 81 MB com cache, segundo o spike);
  - **P7.5-3:** `pdfRenderPage` valida só dpi entre 1 e 1200; a 7.9 precisa de teto de megapixels;
  - **P7.5-4:** na 7.6, chamar `ConfigurePDF` antes do 1º PDF e `ClosePDF` no encerramento;
  - **P7.0-4:** versão do PDFium dentro do `.wasm` (5,8 MB) ainda não identificada;
  - **P7.5-5:** estratégia de fuzz para o WASM, que a cobertura não guia.
- **Ordem do owner (15:02):** "coloque o agente para ir dando atualização para o plano origem conforme for concluindo". O brief ganhou o **§12 "Estado da execução"** (tabela por etapa) e o checklist do §10 foi marcado (6 itens). Todo agente novo recebe a instrução de atualizar o §12 ao concluir; o agente Opus em execução foi avisado por mensagem.

### 2026-10-28 15:12 — "travamento" do fuzz: artefato do motor de fuzz + melhoria de memória no `zipsafe` (agente Opus)
- **Diagnóstico com evidência:**
  - o alvo de fuzz foi instrumentado temporariamente (watchdog de 5 s com stack, registro acima de 200 ms, estatísticas por worker) e rodado com `GODEBUG=fuzzdebug=1`;
  - **(b) confirmada:** todo período em 0 exec/s coincide com uma **minimização** no log:
    - `FuzzOpenPackage`: minimizações de 29,6 e 32,8 s, e o contador pula de 24 para 346.867 ao fim delas;
    - `FuzzDOCX`: 54,8, 13,6 e 27,3 s;
    - `FuzzPPTX`: a semente de 9.714 bytes estoura o `-fuzzminimizetime` padrão de 60 s.
  - O motor (`internal/fuzz/worker.go:238-276`) só soma as execuções no fim da minimização. Se ela estoura o tempo, o worker é encerrado e a contagem se perde;
  - **(a) refutada:** ~2,1 M execuções instrumentadas, pior caso de **29,4 ms**; o watchdog de 5 s nunca disparou. Com 16 workers houve chamadas de 0,2 a 2 s, mas reexecutadas isoladas levaram no máximo 1,05 ms (contenção de CPU);
  - **(c) refutada como causa:** os alvos usam `MaxEntryBytes` de 1 MiB.
- **Melhoria aplicada mesmo assim (commit `b9e8fcc`):**
  - `Package.Read` recusa **antes** de descompactar uma parte que declara tamanho acima do limite;
  - lê com buffer **pré-dimensionado** (`readBounded`), contando os bytes reais;
  - um cabeçalho que mente para cima cobra o orçamento `MaxTotalBytes`.
  - Isso se apoia no `archive/zip`, que dá `ErrFormat` ou `ErrUnexpectedEOF` quando o tamanho real diverge do declarado (conferido no fonte da stdlib).
- **Números (verificados pelo loop principal em worktree limpo):**
  - zip bomb de 100 MiB: pico **156 → 0 MiB** (recusa antecipada);
  - parte de 64 MiB: **157 → 64 MiB** alocados, e de 81 ms para 22 a 30 ms.
- **Testes novos:**
  - `TestReadPresizedAtDefaultLimit`, `TestReadLyingUpwardChargesBudget`, `TestReadBoundedOwnCount`;
  - `TestZipBomb` passa a exigir recusa sem alocar.
- **Gate do commit `b9e8fcc` (worktree):**
  - loop principal: `gofmt` vazio, `vet` OK, `go test -count=1 ./...` verde;
  - agente: `-race ./doctext` OK (183 s); os 4 fuzz, 120 s com `-parallel 1`, PASS (1.228.149 / 410.750 / 717.011 / 1.391 execuções).
- **Mudança de comportamento aceita:** um `styles.xml` ou `numbering.xml` com cabeçalho que mente para cima agora gasta 64 MiB dos 256 MiB do orçamento (o documento já está corrompido nesse caso).
- **Pendências:**
  - **P7.x-1 FECHADA** (artefato de ambiente);
  - **P7.1-1 FECHADA** (o pico de memória foi resolvido);
  - **P7.7-1:** no fuzz longo de 30 min da 7.7, usar `-fuzzminimizetime 5s` ou sementes menores, para a contagem não se perder.

### 2026-10-28 15:35 — 7.6 concluída: integração (agente Sonnet + 2ª via e commit do loop principal)
- **Commit `a1930e8`** (18 arquivos):
  - **guard** (`src/files`): `DocExtractor` + `Extracted` + `SetExtractor` + `ReadContext` + `MaxDocBytes` + `ErrNotDocument`/`ReasonDocument`.
    - `files` não importa `doctext`; sem extrator, o documento continua recusado como binário;
    - o teto é escolhido pela extensão do nome resolvido, antes de abrir;
    - os passos 1 a 10 do guard não mudaram, e o extrator é o último passo.
  - **adaptador `docs.go`** (`mcpollama-mcp`): `ConfigurePDF` na partida, sem iniciar o motor, e `ClosePDF` no encerramento.
  - **flags:** `--max-doc-bytes`/`MCPOLLAMA_MAX_DOC_BYTES` (32 MiB) e `--pdf-cache-dir`/`MCPOLLAMA_PDF_CACHE_DIR` (caminho absoluto, criado se não existir).
  - **rodapé `[arquivo]`:** ganha `formato=`, `paginas=`, `paginas_sem_texto=`, `slides=` e `revisoes_pendentes=`; o `translate` com documento acrescenta `[aviso] tradução devolvida como texto…`.
  - **descrições mínimas** e `TOOLS.md` regerado.
  - **`doctext`:** a mensagem de U+FFFD passa a depender do formato (fecha a P7.1-2).
- **Números (agente; o binário foi conferido pelo loop principal):**
  - `tools/list` full: 7.582 → 7.762 B; compact: 4.291 → 4.381 B;
  - payload do teste com as tools de arquivo: 7.548 → 7.728 B; payload das 4 tools base: inalterado (4.390 B);
  - **binário: ~10,9 MB → 22,5 MB** (22.510.592 B no worktree), com o WASM do PDFium;
  - partida (`initialize` + `tools/list`): 21–25 ms (250 ms a frio). Com `--pdf-cache-dir`, o diretório é criado vazio, ou seja, o motor não inicia na partida.
- **Segurança:**
  - roteiro de ataques da Fase 4 + 18 vetores de documento (agente): 0 escapou; `-race` sem data race;
  - **2ª via independente do loop principal:** 247 vetores, **241 bloqueados, 0 ESCAPOU, 0 FAIL**, 2 ACHADO **já conhecidos da Fase 4**:
    - denylist sem `.npmrc`, `.pypirc`, `.netrc`, `.pgpass`, `.htpasswd`, `putty.ppk`;
    - `--allow-root C:\` aceito (decisão SEC-M-02 de 23/10: aceitar com aviso).
- **Gate do commit (worktree, loop principal):** `vet` OK; `go test -count=1 ./...` verde; `dumptools -check` in sync.
- **Desvios e observações do agente:**
  1. **budget:** o teto que estourava era o `toolsListBudgetFull` (7.600, das tools de arquivo), que subiu para **7.800**; o `toolsListBudget` (4 tools base, 4.390 B, inalterado) também subiu para 4.700, como a D-13 mandava. **Levar ao owner;**
  2. `.doc`/`.ppt`/`.xls` com conteúdo que não é OLE voltam ao caminho de texto;
  3. **denylist casa por prefixo ou sufixo do nome inteiro:** `prod.env.pdf` e `chave.pem.docx` **não** são barrados (política pré-existente). **Levar ao owner;**
  4. os campos novos do rodapé vão no fim da linha `[arquivo]`;
  5. `MaxTextRunes` = `MaxPromptChars × MaxChunks` (5.120.000), com teto de 16 Mi; o `--timeout` é aplicado dentro do adaptador;
  6. `splitChunks` inalterado; o teste prova que nenhum marcador é cortado em 9 tamanhos de parte.
- **Pendências:**
  - **P7.6-1:** smoke com PDF real pelo binário (vai para a 7.7);
  - **P7.6-2:** decisão sobre os tetos de budget (D-13);
  - **P7.6-3:** decisão sobre a denylist em nomes compostos.

### 2026-10-28 15:40 — decisões do owner pós-7.6 + início da 7.7
- **Decisões (AskUserQuestion):**
  - **D-13 revista:** `toolsListBudgetFull` fica em 7.800 (tools de arquivo, 7.728 medido) e `toolsListBudget` (base) **volta a 4.400** (4.390 medido, inalterado). Commit `3d98ed2`; testes de budget e catálogo verdes.
  - **Denylist:** **mantida** a política atual (casa por prefixo ou sufixo do nome inteiro; `prod.env.pdf` e `chave.pem.docx` não são barrados; faltam `.npmrc` e similares). Registrada como pendência conhecida **P7.6-3 (aceita)**.
  - **7.7:** tudo, em sequência segura.
- **7.7 em execução (3 frentes paralelas):**
  1. **Fuzz de 30 min por alvo** (`FuzzDOCX`, `FuzzPPTX`, `FuzzPDF`), num **worktree isolado** em `3d98ed2` (scratchpad `wt-fuzz`), com `-fuzzminimizetime 5s -parallel 8 -timeout 45m`; logs em `scratchpad\fuzz30-<alvo>.log`.
  2. **Revisão adversarial** (agente Opus):
     - ataques de integridade (texto que some ou duplica em silêncio), parser (variantes de zip bomb, `.rels`, XML, PDF hostil, FS e stdout do WASM), guard + teto por extensão, vazamento, determinismo e memória;
     - testes `*_adv_test.go` movidos para `…revisao-adversarial-fase7.testes/` ao fim; correções só como proposta.
  3. **Ao vivo e aceite** (agente Sonnet):
     - binário do HEAD num caminho temporário (não `D:\MCP`), uma chamada por vez, com o corpus do spike e 2 `.docx` copiados para a pasta ignorada;
     - aceite com `--allow-root "A:\Monitor\CSL Telecom\Contratos"` (unidade de rede: se o guard recusar, para e registra);
     - **numeração conferida por 2ª via no Word via COM** (`ListFormat.ListString`), só contando acertos, sem conteúdo.
- **Regra do owner mantida:** cada agente acrescenta a sua linha no §12 do brief (`7.7a`, `7.7b`) ao concluir.

### 2026-10-28 15:52 — 7.7b concluída: testes ao vivo + aceite com os contratos reais (agente Sonnet)
- **Report:** `.workspace/reports/MCPOllam2026-10-28-155000-fase7-ao-vivo-aceite.report.md`. Só métricas: o loop principal conferiu 0 CPF, CNPJ, "R$", nomes de arquivo originais e token.
- **Binário:** HEAD `ce8d248`, num caminho temporário (não `D:\MCP`). Uma chamada por vez, 0 HTTP 429, modelo residente. Os tempos podem estar inflados pelo fuzz paralelo.
- **Ao vivo:**
  - PDF: 5 chamadas, de 1,9 a 15,3 s de parede;
  - DOCX: 3 chamadas, de 5,3 a 7,7 s;
  - **recusas 4/4 corretas** (2 digitalizados e 2 com senha, de 7 a 70 ms, mensagens opacas);
  - `paginas=` igual ao PyMuPDF em 4/4.
  - 1ª chamada de PDF por processo: **+8,9 s** com o cache vazio (que ela preenche), **+7,7 s** sem cache, **+0,6 s** com o cache preenchido. Depois disso, a extração leva de 91 a 148 ms (PDF) e de 65 a 114 ms (DOCX).
- **Aceite §7.7, todos os itens passaram:**
  - a raiz de rede `A:\Monitor\CSL Telecom\Contratos` (`\\WN-DC03\Financeiro`) **foi aceita pelo guard**, sem afrouxar nada;
  - `summarize_file` em DOCX: OK;
  - `extract` de cláusulas: OK;
  - **numeração conferida por 2ª via no Word 16 via COM** (`ListFormat.ListString`, somente leitura e invisível):
    - **d01: 83/83**;
    - **d02: 83/84** (o 84º é um parágrafo numerado vazio, sem texto a alinhar);
    - os marcadores saem como `- `, conforme o brief;
  - PDF com texto: `paginas=9`, igual à referência;
  - PDF digitalizado e `.doc`: recusas do §6.
- **Defeitos de produção:** nenhum.
- **Observações:**
  - a linha `[arquivo]` traz `nome=` (o nome do arquivo). Isso não é novidade: quem chama já mandou o `path` completo, então não é vazamento novo. Registrado para o owner;
  - **não há `.pptx` no acervo**: PPTX validado só com fixtures sintéticas;
  - o agente não leu o resumo para conferir se ele cita as cláusulas certas (sigilo); a numeração entregue ao modelo foi conferida;
  - o `Quit` do Word falhou no PowerShell 5.1, e o agente encerrou os 2 `WINWORD` dele com `Stop-Process`, sem salvar nada.
- **Dados de cliente fora do git** (ignorados, conferido):
  - `.workspace/spike-pdf/docx/` e `mapa-docx.txt`;
  - **`.workspace/spike-pdf/numchk/`, com texto extraído e JSON do Word, ou seja, conteúdo de cliente.**
- **Pendência:**
  - **P7.7-2:** apagar `numchk/` (e, ao fim da fase, decidir se o corpus `spike-pdf/` fica ou sai).

### 2026-10-28 16:09 — consulta do owner: compartilhamento Samba/FTP no servidor de IA e módulo Linux (só resposta, nada executado)
- **Pergunta:** montar uma pasta Samba ou FTP no servidor para colocar os dados, ou um módulo Linux para upload de arquivos maiores.
- **Resposta (aceita pelo owner, "ok, entendido"):** **não agora.**
  - O arquivo não vai ao servidor: a estação extrai e envia **só o texto**. O limite real é o contexto do modelo, contornado pelo map-reduce, e não o tamanho do arquivo.
  - Os dados já estão no `\\WN-DC03\Financeiro` (`A:`), aceito pelo guard; copiar para o servidor de IA criaria mais uma cópia de documento de cliente.
  - O host de IA está exposto pelo Cloudflare Tunnel e hoje é instável (PCIe/PSU).
  - FTP é texto puro.
- **Candidata a Fase 8 (se a triagem em volume virar rotina):**
  - **worker Linux** do próprio mcpollama numa máquina da LAN, **não** a da GPU;
  - leitura **somente leitura** do compartilhamento do `WN-DC03`;
  - **jobs assíncronos** (padrão `tasks/` do `TruenasMcp`);
  - OCR com a `libtesseract64.so` (reabre a D-10);
  - junta-se à pendência "tool de inventariar pasta" (brief §9). Exige revisão de segurança própria.

### 2026-10-28 16:20 — decisões do owner (pendências) + rodada 2 do aceite + ajustes no servidor
- **Decisões (AskUserQuestion, 16:10–16:12):**
  1. **Dados de cliente:** apagar o texto extraído já; o corpus fica até o fim da Fase 7.
     - Feito: apagados `numchk/` (9 arquivos), `owner-out/` (12) e `extall/` (4).
     - `out/` e `out_prev/` foram conferidos (204 JSON, só métricas, nenhum campo de texto) e ficam.
  2. **7.8 aprovada:** binário 0.6.0 em `D:\MCP\` + `--pdf-cache-dir D:\MCP\cache\pdfium` nos `.mcp.json` do MCPOllam e do GestorERP, quando a 7.7 fechar verde.
  3. **D-7:** OCR **opt-in** por `--ocr-lib` + `--ocr-lib-sha256`.
  4. **D-11:** modelo de OCR **`best` por padrão**. A medição best × fast da 7.9 fica só como registro.
  5. **Projeto jurídico:** "não, o caminho será informado no contexto, nada hardcode". Nada é preparado em `.mcp.json` de projeto jurídico.
     - **Implicação técnica a levar ao owner na 7.8:** o servidor só lê dentro do `--allow-root`; um caminho "informado no contexto" só funciona se estiver numa raiz permitida.
     - Alternativa a avaliar: suportar `roots` do protocolo MCP (raízes declaradas pelo cliente).
  6. **Servidor:** os 3 ajustes autorizados.
- **Rodada 2 do aceite** (documentos do owner em `Documents`; commit `9148331`; agente Sonnet):
  - **PPTX real conferido com o PowerPoint (COM):**
    - `Slides.Count` 9 = 9 e 150 = 150;
    - ocultos 1 = 1; com notas 144 = 144;
    - **razão de palavras por slide: 1,000** em todos os 146 slides com texto.
  - **DOCX:** numeração 22/22 com o Word; palavras 2.449 (Word) contra 2.539 (nossa extração), razão 1,037 por causa dos marcadores.
  - **PDF:** `paginas=` igual ao PyMuPDF em 4/4 (550, 338, 15, 14).
  - **Ao vivo:**
    - livro de 550 páginas resumido em 10 partes e 11 chamadas, **192,8 s**, sem estourar o `--timeout` de 120 s, que vale por chamada;
    - PPTX de 43 MB recusado por tamanho (teto de 32 MiB);
    - DOCX vazio recusado.
  - **Achado menor:** `revisoes_pendentes=sim` falso positivo num DOCX com `<w:ins/>` **vazio** dentro de `w:numPr` (modelo da Microsoft de 1999); o Word conta 0 revisões.
    - É conservador (erra para o lado de "não fechado").
    - Correção sugerida: só sinalizar `w:ins` com conteúdo. **P7.7-3**, junto com as correções da revisão adversarial.
  - **Observação:** a tool levou 192,8 s no total. Confirmar se o cliente MCP (Claude Code) aceita esse tempo numa chamada só; antes, 87 s foram validados. **P7.7-4**.
- **Servidor (SSH; backups em `/root/srv-backup-20261028/`):**
  1. **Ciclo `nvidia-powercap` ↔ `ollama-warm`:**
     - causa: `After=multi-user.target` no powercap + `ollama-warm After=powercap` + `multi-user` esperando o warm;
     - correção: `After=nvidia-persistenced.service` (gravação atômica + `sync`);
     - **`systemd-analyze verify default.target`: 2 ciclos → 0**; `systemctl start ollama-warm` → `Result=success` (286 ms, modelo residente).
     - A prova no boot real fica para o próximo reboot.
  2. **journald:** drop-in `/etc/systemd/journald.conf.d/10-sync.conf` (`Storage=persistent`, `SyncIntervalSec=10s`), conferido com `systemd-analyze cat-config`; `systemd-journald` reiniciado.
  3. **`CONEXAO-IA.md`:**
     - 10 trechos desatualizados corrigidos: 3 → 4 GPUs, 240 → 150 W, `MAX_LOADED_MODELS=2`, base `qwen3-coder` recriada, warm, checklist e resumo;
     - nova seção "2026-10-26/28": mapa UUID ↔ barramento ↔ porta, link `00:1b.4`, histórico de potência e reboots, ajustes de 28/10;
     - **md5 idêntico na cópia local e no servidor** (`c6f115c0…`); perms 600; backup local `.bak-20261028`.
     - **Erro meu:** usei de novo uma função chamada `R`, que é **alias de `Invoke-History`** no PowerShell (armadilha já registrada nesta sessão), e as substituições não foram aplicadas. Restaurei do backup e refiz com `Rep1`: 10/10 aplicadas.
  - **Observação:** o servidor tinha reiniciado em **2026-10-27 às 12:04**, fora desta sessão.

### 2026-10-28 16:30 — 7.7a concluída: revisão adversarial (Opus) + decisões do owner sobre as correções
- **Report e testes de ataque:** `.workspace/reports/MCPOllam2026-10-28-161147-revisao-adversarial-fase7.report.md` + `.testes/` (testes de ataque, 7 diffs de correção + `fase7-todas.diff`, logs de gate). Commit `e7b05be`; `src/` ficou limpo (conferido).
- **158 vetores** (errata do agente, 17:04: o 1º total, de 176 com 92 BLOQUEADO, incluía 18 linhas `VEC|doc/` do teste existente `filetools_docs_test.go`): 74 BLOQUEADO, 30 FIEL, 34 INFO, 10 ACHADO, **10 SILENCIOSO**, **0 ESCAPOU**. Nada vazou; o servidor ficou de pé em todos.
- **Achados:**
  - **Altos:**
    - **A1:** DOCX de 1,2 KB → ~1 GiB alocado (pico de 293 MiB). O `lvlText` é copiado em cada parágrafo numerado e, dentro de tabela, não conta no teto;
    - **A2:** PDF de 33 KB rodou 16 a 29 s com `--timeout` de 2 s e +2,9 a 5,4 GiB, porque o `ctx` só é checado entre páginas e não há teto de memória do WASM; ele também bloqueia os outros PDFs.
  - **Médios (texto que some em silêncio):**
    - **M1:** `fldChar` desbalanceado apaga o resto da parte;
    - **M2:** `w:altChunk` descartado;
    - **M3:** `numbering.xml` quebrado ou com mais de 100 mil elementos → toda a numeração some.
  - **Baixos:**
    - **B1:** nome de parte com outra caixa pula cabeçalho e notas;
    - **B2:** cabeçalho apontando para `document.xml` duplica o corpo;
    - **B3:** slide repetido no `sldIdLst` duplica (100 mil repetições = 15 a 24 s de CPU);
    - **B4:** PPTX perde o `Fallback` sem `Choice`;
    - **B5:** timeout no PPTX com mensagem em inglês;
    - **B6:** a junção de hífen no PDF apaga hífens reais ("obriga-se" → "obrigase").
  - Determinismo OK (DOCX e PPTX 50×, PDF 20×).
- **Correções provadas numa cópia:** suíte verde, 0 SILENCIOSO, 0 ESCAPOU; A1 com pico de 0,4 a 6 MiB; A2 parando em 2,0 s com +223 a 440 MiB.
- **Não testado:** limite de 4 GiB do WASM; Office real nos casos-limite; calibração no corpus real; `-race` nos testes de ataque; binário Linux.
- **Decisões do owner (AskUserQuestion, 16:28):**
  1. **aplicar A1 + A2 + M1 + B1–B5 + P7.7-3** (falso positivo de revisões), com o teto do A2 calibrado no corpus real;
  2. **M2:** avisar no rodapé (`conteudo_importado=sim`; extrair texto, HTML ou DOCX importado; se não suportado, `[aviso]`);
  3. **M3:** avisar no rodapé (`numeracao=indisponivel`);
  4. **B6:** manter sempre o hífen real (só juntar o soft-hyphen `\x02` do PDFium); **texto oculto entra rotulado `[oculto]`**.
- **Execução:** o agente Opus da revisão foi retomado para aplicar em `src/` em 2 passos (1: correções provadas + calibração; 2: M2, M3, B6 e texto oculto), com commit e gate do loop principal entre os passos.

### 2026-10-28 17:08 — correções da revisão adversarial, passo 1 (agente Opus + prova e commit do loop principal)
- **Aplicado em `src/doctext`:**
  - `docx.go`: **A1** (célula conta no teto ao entrar), **M1** (campo aberto no fim da parte → recusa), **B1**, **B2** e **P7.7-3** (`ins`/`del`/`moveFrom`/`moveTo` só contam como revisão pendente se tiverem conteúdo);
  - `docx_numbering.go`: **A1** (`lvlText` com mais de 128 runas vira `- `);
  - `pdf.go`: **A2** (teto do WASM de **8192 páginas = 512 MiB**, interrupção pelo `ctx`, `Kill` da instância no timeout);
  - `pptx.go`: **B3** (`r:id` repetido → recusa), **B4** (`Fallback` sem `Choice` passa a ser lido), **B5** (timeout tipado);
  - `zipsafe.go`: **B1** (busca de parte sem diferenciar maiúsculas);
  - `hardening_test.go`: 7 testes de regressão.
- **Calibração do teto do PDF no corpus real** (40 PDFs, c01–c35 e o05–o09, só ids e números; CSV em `.testes/passo1-calibracao-teto-pdf.csv`, conferido sem nomes):
  - tetos de 4 GiB, 512, 256 e 128 MiB → **0 divergências** (30 extraem iguais; 3 senhas e 7 sem texto recusados nos 4);
  - maior uso do WASM: **25,4 MiB**; livro de 550 páginas: 18,1 MiB em ~14 s;
  - teto escolhido: 512 MiB (~20× o maior uso).
- **Prova do loop principal:**
  - gate na árvore: `gofmt` vazio, `vet` OK, `go test -count=1 ./...` verde;
  - **os 7 testes de regressão FALHAM no código antigo** (worktree no HEAD anterior + o arquivo de teste) **e PASSAM no novo** (7/7 nos dois sentidos).
- **Agente:**
  - `-race` em todos os pacotes: 0 data race;
  - ataques da Fase 7 (158 vetores): **0 ESCAPOU**. Restam só os 3 SILENCIOSO e o 1 ACHADO que são M2, M3 e B6 (passo 2). A bomba de PDF agora para em 2,0 s (+223 a 332 MiB; antes até +5,4 GiB);
  - Fase 4: 227 vetores, 0 ESCAPOU, 2 ACHADO de política já aceitos.
- **Errata:** o report 7.7a e o brief foram corrigidos pelo agente para 158 vetores; o número deste plano também foi corrigido acima.

### 2026-10-28 17:10 — fuzz de 30 min por alvo concluído
- **Execução:** worktree isolado em `3d98ed2` (código **anterior** às correções da revisão), `-fuzzminimizetime 5s -parallel 8`.
  - **`FuzzDOCX`:** 30,1 min, **28.299.843** execuções, 648 entradas no corpus, **PASS**.
  - **`FuzzPPTX`:** 30 min, **40.984.381** execuções, 438 no corpus, **PASS**.
  - **`FuzzPDF`:** 30 min, **615.504** execuções (o WASM é lento e a cobertura do Go não o enxerga), 168 no corpus, **PASS**.
  - **0 entradas de falha salvas** (sem `testdata/fuzz`).
- **Resumos** (marcos de 5 min) versionados em `.workspace/reports/MCPOllam2026-10-28-161147-revisao-adversarial-fase7.testes/fuzz30/`. O worktree `wt-fuzz` foi removido.
- **Leitura honesta:** o fuzz guiado por cobertura **não** encontrou A1 (amplificação de `lvlText`) nem A2 (PDF lento com memória sem teto), que a revisão adversarial achou com vetores construídos. O critério do brief §7.5 (30 min por alvo sem panic e sem travar) foi cumprido, mas fuzz verde não substitui a revisão.
- **Pendência:**
  - **P7.7-5 (opcional):** repetir o `FuzzDOCX` e o `FuzzPDF` no código já corrigido, em fase futura.

### 2026-10-28 17:42 — correções da revisão adversarial, passo 2 (M2, M3, B6, texto oculto)
- **Aplicado (agente Opus):**
  - **M2:** `altChunk` com TXT (UTF-8), HTML ou DOCX importado (profundidade 1, mesmo orçamento e teto) é extraído no lugar.
    - O HTML passa por um leitor simples, sem DTD e sem entidades definidas.
    - Tipo não suportado → `[aviso] parte do conteúdo não pôde ser lida`.
    - Rodapé `conteudo_importado=sim`.
    - Novos `docx_import.go` e `docx_import_test.go`.
  - **M3:** `numbering.xml` (ou `styles.xml`) quebrado ou acima do teto → texto sem números + `numeracao=indisponivel`. O teto subiu de 100.000 para 300.000 itens (o caso válido de 50.005 `w:num` voltou a ter números).
  - **B6:** o PDFium devolve **o mesmo `\x02` para hífen real e para soft-hyphen**, sem como distinguir (medido).
    - Regra aplicada: `\x02` antes de minúscula junta a linha e **mantém** o hífen ("obriga-se", "ex-empregado"); em qualquer outro caso o hífen e a quebra voltam; `-` literal no fim da linha fica.
    - Custo: a quebra silábica também mantém o hífen ("con-trato"), o que é coerente com a decisão "manter hífen sempre".
  - **Texto oculto:**
    - **DOCX:** `w:vanish` direto, por estilo (caractere e parágrafo, `basedOn`, padrões do documento) → linha `[oculto] <texto>`. Simplificação: a alternância do `vanish` entre estilos aninhados não é calculada.
    - **PPTX:** formas, grupos e tabelas com `hidden="1"` rotulados.
    - Rodapé `texto_oculto=sim`.
    - **PDF: limitação registrada.** O modo de renderização 3 só sai por chamada por caractere e é a camada de OCR dos PDFs digitalizados; rotulá-lo erraria no caso mais comum.
- **Campos novos do rodapé** (só quando se aplicam): `numeracao=indisponivel`, `conteudo_importado=sim`, `texto_oculto=sim`. As descrições das tools não mudaram (`TOOLS.md` igual; budget 4.400/7.800).
- **Verificação:**
  - agente: gate com `-race` em todos os pacotes, 0 data race;
  - ataques da Fase 7: 158 vetores, **0 ACHADO, 0 SILENCIOSO, 0 ESCAPOU**; bomba de PDF para em 2,0 s;
  - Fase 4: 227 vetores, 0 ESCAPOU;
  - **loop principal:** a árvore staged (sem o `src/ocr/` e o `go.mod` do agente do OCR, que estão em andamento) foi verificada num worktree limpo: `gofmt` vazio, `vet` OK, todos os pacotes `ok`.
- **Observação:** o `src/ocr/` e as mudanças em `go.mod`/`go.sum` (`x/image`, `x/sys` direto) são da 7.9a, ainda em execução, e ficaram fora deste commit.

### 2026-10-28 17:50 — 7.8 concluída: binário 0.6.0, cache nos clientes, validação real + 7.9a commitada
- **Binário:**
  - **`0.6.0-4538523`**, gerado num worktree limpo no `4538523` (`CGO_ENABLED=0`, `-trimpath`), para não incluir o OCR em andamento;
  - 22.608.384 B; smoke de partida + `tools/list`: 44 ms, descrições novas presentes;
  - instalado em `D:\MCP\mcpollama-mcp.exe`. O em uso virou `.exe~` (o `.exe~` antigo 0.5.1 foi apagado); hash igual ao gerado.
- **Clientes** (decisão do owner): `--pdf-cache-dir D:\MCP\cache\pdfium` inserido antes do `--api-key` nos `.mcp.json` do **MCPOllam** e do **GestorERP**.
  - Inserção de texto, sem exibir o token; backup no scratchpad, porque contém o token;
  - JSON válido, 10 args; o do MCPOllam segue ignorado pelo git;
  - `D:\MCP\cache\pdfium` foi criado e preenchido (22 MB) na 1ª chamada de PDF.
- **Validação real pelo Claude Code** (`claude -p --model haiku --mcp-config <projeto> --strict-mcp-config`, documentos do owner `o01`/`o03`/`o07`/`o05`):
  - MCPOllam DOCX: `formato=docx revisoes_pendentes=nao`, OK;
  - GestorERP PDF: `formato=pdf paginas=15 paginas_sem_texto=0`, OK;
  - MCPOllam PPTX: `formato=pptx slides=9`, OK;
  - **tool longa (P7.7-4 FECHADA):** livro de 550 páginas, 10 partes, 11 chamadas, `total_ms=170302`, parede de 206,6 s, **OK**. O Claude Code aceita uma tool de ~3,5 min.
- **Docs** (agente Sonnet, conferido pelo loop principal; commit `2899b7d`):
  - **README:** estado, tools com documentos, flags, seção "Documentos" com marcadores, rodapé, recusas, limites, motor e licenças, cache, binário e limitações;
  - **`CLAUDE.md`:** estado, regra de repetir os testes de ataque da Fase 7 em mudanças no `doctext`, flags, **toolchain go1.27.0** (P7.0-3 fechada) e layout;
  - 0 segredos.
- **7.9a commitada (`9b86439`):** pacote `src/ocr/` (agente Opus).
  - Worker isolado: o filho nasce **suspenso**, entra no Job Object (teto de memória, kill-on-close) e só então roda.
  - Hash conferido no pai e no worker, com o arquivo da DLL travado contra troca; caminho UNC recusado.
  - Ambiente do filho sem segredos; stdout do worker limpo (até `puts` do msvcrt); `FuzzOCRFrame` 120 s, 1,46 M execuções, PASS.
  - **Medido (A4 a 300 dpi):** best 4,6 a 6,0 s por página, conf 95, 12/12 palavras-chave; fast 1,6 a 1,7 s, conf 95; working set do worker com pico de 82 MiB.
  - **Desvio:** a DLL 5.3.3 não exporta OSD. Com conf < 60, faz uma 2ª passada girada 180° (cabeça para baixo recuperada; 90° já é lido direto).
  - **Verificação do loop principal no worktree:** suíte verde, `CGO_ENABLED=0 go build ./...` OK, `GOOS=linux` (`ocr` e `main`) OK, `go mod tidy -diff` vazio.
- **Em execução:** 7.9b (integração do OCR, agente Opus).

### 2026-10-28 19:06 — 7.9b concluída + binário 0.7.0 + OCR ligado nos clientes + limpeza do corpus
- **7.9b (agente Opus; commit `9f09aaa`):**
  - 9 flags `--ocr-*`: `lib`, `lib-sha256`, `tessdata` (padrão `<dir da DLL>\tessdata\best`), `workers` (4 nesta máquina), `page-timeout` (60 s), `max-pages` (30), `max-mem` (1 GiB), `max-megapixels` (40), `dpi` (300);
  - qualquer problema → `WARNING: OCR disabled` e o servidor sobe sem OCR;
  - modo interno `--ocr-worker`, tratado antes de flags e env; o worker nunca vê o `--api-key`;
  - **PDF:** OCR só nas páginas sem texto, em paralelo e em ordem;
    - marcadores `[página N — OCR conf=..]`, baixa confiança (menos de 60), `girada=180`, falhou e tempo esgotado;
    - página branca pulada por triagem a 50 dpi; dpi reduzido por megapixels;
  - **imagens `.png`/`.jpg`:** `DecodeConfig` antes de decodificar; a bomba PNG de 100.000×100.000 foi recusada com 1,2 KB alocados;
  - **saída:** rodapé com `paginas_ocr`, `conf_ocr_media`, `paginas_ocr_falhas` (e `_nao_processadas`) + `[aviso]` de conferência;
  - o `doctext` continua sem importar `ocr` (o OCR entra por `Limits.OCR`).
  - **Ataques:** Fase 4, 0 ESCAPOU; Fase 7, 166 vetores (8 de imagem novos), 0 ESCAPOU e 0 SILENCIOSO. Erro do agente, corrigido: um fixture JPEG incompleto dava falso ESCAPOU.
  - **Aceite (agente, só métricas):**
    - 26 páginas OCR em 7 PDFs, 0 falhas, conf de 54 a 92 (média 88);
    - o08: 12 páginas em 18,8 s;
    - **3/3 valores conferidos** contra a imagem (página do owner);
    - **limitação:** 4 de 11 linhas de tabela sobre fundo sombreado sumiram, inclusive o total, e um símbolo de moeda saiu trocado.
  - **Verificação do loop principal (worktree limpo):** suíte verde + **tag `ocr` com a DLL real** verde; `TOOLS.md` in sync; budget full **7.800/7.800** (sem folga), base 4.400.
  - **Report:** `.workspace/reports/MCPOllam2026-10-28-183855-fase7-9-ocr.report.md` (sigilo conferido: as ocorrências de nome de arquivo eram genéricas).
- **Binário 0.7.0-9f09aaa** (23.032.320 B; `CGO_ENABLED=0`, `-trimpath`, worktree em `9f09aaa`):
  - smoke com hash certo: `OCR enabled: Tesseract 5.3.3, lang por, models best, workers 4`; com hash errado: `WARNING: OCR disabled…` e as tools seguem;
  - **partida medida (com o fuzz ocupando a CPU):**
    - sem OCR, `initialize` em 124 a 212 ms (a 1ª execução levou 21,5 s: varredura do antivírus no `.exe` novo, confirmado pela repetição);
    - **com OCR, 0,7 a 3,6 s até o `initialize` e 3 a 8,6 s para encerrar** (espera a sonda). O agente afirmou que a sonda não atrasava o handshake, e isso não se confirmou. **P7.9-1:** medir de novo sem carga e, se o atraso persistir, tirar o trabalho síncrono da partida.
- **Instalação em `D:\MCP\`:**
  - **o `Remove-Item` foi bloqueado pela proteção da pasta** (correto). Instalado só com renomeação e cópia: o 0.6.0 virou `mcpollama-mcp-0.6.0.exe~`, e o `.exe~` (0.5.x) continua lá;
  - **pendência do owner:** apagar os 2 `.exe~` depois de reiniciar os clientes.
- **Aceite final do §11.9 (contrato digitalizado c07, driver local, sem o conteúdo sair da máquina):** `extract` de partes, CPF/CNPJ, valor e data → `paginas_ocr=9 conf_ocr_media=91 paginas_ocr_falhas=0` + `[aviso]` presente; JSON de 891 caracteres **não exibido**; 75 s (OCR + 13,6 s de modelo).
- **Docs** (agente Sonnet; commit `8451449`): as 9 flags, a seção OCR, marcadores, rodapé, limites e limitações no README e no `CLAUDE.md`. O agente corrigiu um número que eu tinha passado (best de 4,5 a 6,7 s por página, conforme o report).
- **Decisões do owner (AskUserQuestion, ~19:00):**
  - **OCR ligado no MCPOllam e no GestorERP:** `--ocr-lib` + `--ocr-lib-sha256` inseridos antes do `--api-key`, sem exibir o token, backup no scratchpad, JSON válido com 14 args;
  - **validação pelo Claude Code** nos 2 projetos (o08, 12 páginas): `paginas_ocr=12 conf_ocr_media=90 paginas_ocr_falhas=0` + `[aviso]` + OK;
  - **corpus apagado:** `.workspace\spike-pdf\` (287 arquivos, 208,7 MB), mais os arquivos do driver de aceite no scratchpad que guardavam nomes originais de contratos (10 arquivos). Nenhum documento sobrou.
- **Erro meu:** o cabeçalho "18:57" de uma mensagem foi estimado (o real era ~18:55).