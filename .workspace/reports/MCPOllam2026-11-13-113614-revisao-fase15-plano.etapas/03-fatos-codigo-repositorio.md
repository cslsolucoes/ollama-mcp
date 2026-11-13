# Etapa 03: fatos de código, repositório e arquivos

**Modelo:** Sonnet 5.5 · **Hora real:** 2026-11-13 11:47 (`date`) · **Plano revisado:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (177 linhas, igual ao HEAD `7fbe5cc`).
**Regra seguida:** somente leitura no repositório e em `D:\MCP`; nenhum arquivo do repositório alterado (`git status --short src` vazio antes e depois). Sem ssh/scp/curl, sem `mcp__mcpOllama__*`, sem git que escreva, sem abrir credencial nem transcrição.

## Fontes lidas

- Plano: `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (inteiro) e `...\reports\...etapas\00-indice-e-metodo.md` (inteiro).
- Código: `E:\MCPOllam\src\files\guard.go`, `policy.go`, `nlink_other.go`, `guard_test.go`; `src\mcpollama-mcp\config.go`, `main.go`, `server.go`, `backend.go`, `ocr.go`, `ocr_test.go`, `config_test.go`, `station.go`, `station_wmi_other.go`; `src\ocr\ocr_other.go`, `ocr.go`, `pool.go`; `src\tools\registry.go`, `filetools.go`, `allowlist.go`; `src\go.mod`.
- Repositório: `CHANGELOG.md` (linhas 1-100), `CLAUDE.md` (HEAD e árvore de trabalho), `README.md`, `docs\README.md`, `docs\INSTALACAO.md` (índice, D.2, E.1, G.1), `deploy\servidor-ia\nvidia-powercap.service`, `.workspace\plans\MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (§11), `.workspace\plans\concluidos\MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` (linha 145), `.workspace\reports\MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md` e pasta `.testes`.
- `D:\MCP` (somente `ls`, `go version -m`, `sha256sum` dos binários): listagem, `Documentation\Ollama`, `tessdata`, `cache`.
- `referencias\Brief_Referencias\60-Requisitos_mcpollama.md:272` (MCPO-REQ-BUILD-004).
- Não usei os recuperados do scratchpad (a etapa é de fatos de código; tudo abaixo foi lido ou executado agora).

**Execuções feitas (somente leitura):**
1. `go list -deps` e `go version -m` (dependências do binário).
2. `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` em `src/`.
3. `go test -count=1 ./files ./mcpollama-mcp` (Windows).
4. Um programa descartável em `...\scratchpad\f4check` (módulo próprio com `replace` para `E:/MCPOllam/src`, conteúdo **falso** `FAKE-NOT-A-TOKEN`, pasta temporária apagada ao fim) para provar o comportamento de `files.Denied` e `Guard.Read` (F-3). Erro de processo meu: um `go list` inicial em `src/` saiu com `GOFLAGS=-mod=mod`, que poderia reescrever `go.mod`/`go.sum`; conferi com `git status` e `git diff --stat`: nada mudou.

---

## Achados

### F-1 (F1) Arquivos por plataforma e linhas de `guard.go` do §2 item 1

- **Onde no plano:** §2 item 1 (linha 32).
- **O que o plano diz:** "`station_wmi_{windows,other}.go`, `files/nlink_{windows,other}.go`, `ocr/*_windows.go` e `ocr/ocr_other.go`. O `files/guard.go` já ramifica por `runtime.GOOS` (`guard.go:120,200`) e trata `/` como raiz larga demais (`guard.go:207`)."
- **O que a evidência mostra:**
  - Todos os arquivos existem: `src/mcpollama-mcp/station_wmi_windows.go` e `station_wmi_other.go` (`//go:build !windows`, `wmiMACs()` devolve `nil`, linhas 3-6); `src/files/nlink_windows.go` e `nlink_other.go` (`//go:build !windows`, `Stat_t.Nlink`, linhas 1-21); `src/ocr/job_windows.go`, `stdio_windows.go`, `tesseract_windows.go` (3 arquivos) e `src/ocr/ocr_other.go` (`//go:build !windows`, `platformSupported = false` na linha 15).
  - O `//go:build` fora de gateway/cmd/testes só existe nesses pares (conferido por `grep -rln "^//go:build"`); não há outro arquivo de plataforma que o plano esqueceu.
  - `runtime.GOOS` em `files/guard.go`: linhas **120** (`checkSyntax(r, runtime.GOOS == "windows")`), **200** (`same()` do `BroadRoots`) e **340** (`windows := runtime.GOOS == "windows"` em `ReadContext`). O plano cita 120 e 200; falta a 340.
  - `guard.go:207` é `broad := filepath.Dir(r) == r // C:\ or /`: confere.
  - `SplitRoots`: o comentário que cita `os.PathListSeparator` está em `guard.go:223-226` e o uso no código em **`guard.go:229`**. O changelog "22:45" cita `guard.go:223`: é o comentário, não a linha do `Split`.
- **Classificação:** confirmado (com 2 linhas a corrigir).
- **Gravidade:** informativo.
- **Reparo proposto:** errata **E-n** no §2 item 1: "`runtime.GOOS` em `guard.go:120, 200 e 340`; `SplitRoots` usa `os.PathListSeparator` em `guard.go:229` (comentário em 223-226). Conferido em 2026-11-13 (revisão)." E-n no changelog "22:45", item C(1), logo após "`guard.go:223`": `**[errata E-n]** o uso está em guard.go:229; 223 é o comentário.`

### F-2 (F1, F5, §7) `/` e o home são aceitos com aviso, não recusados; o aviso de "diretório de sistema" não existe no Linux

- **Onde no plano:** §7, linha "Rodar como root" (linha 132); §2 item 1 (linha 32).
- **O que o plano diz:** "o MCP lê qualquer arquivo permitido pelo `--allow-root`; **`/` como raiz já é recusado**, mas `--allow-root /root` seria amplo".
- **O que a evidência mostra:**
  - `NewGuard` (`files/guard.go:110-159`) **não** recusa `/`: exige só caminho absoluto, existente, diretório, sem junction e sem aninhamento. `BroadRoots` (`guard.go:185-221`) apenas **marca** a raiz larga (`/`, home, diretórios de sistema) e o servidor loga `WARNING: --allow-root %q is a volume root, your home directory or a system directory: ALL content of that tree (except the fixed denylist) can be read...` (`server.go:178-185`). Não há `return error`. A revisão adversarial da Fase 4 já registrou a decisão do owner "aceitar, com aviso" (SEC-M-02, opção (b), `...fase4-arquivos.report.md:119-127`).
  - `DefaultBroadDirs` (`guard.go:170-179`) só monta a lista de diretórios de sistema com variáveis do Windows (`SystemRoot`, `ProgramFiles`, `ProgramFiles(x86)`, `ProgramW6432`, `ProgramData`). No Linux a lista é **vazia**: só `/` e o home (`os.UserHomeDir()`, em root = `/root`) disparam o aviso. `/etc`, `/usr`, `/proc`, `/var`, `/root/.ssh` como raiz **não avisam**.
- **Classificação:** refutado ("já é recusado" é falso; é aceito com aviso).
- **Gravidade:** importante (alimenta a etapa 04 e a questão 9).
- **Reparo proposto:** errata **E-n** na linha de §7: "`/` como raiz **não** é recusado: é aceito e só gera `WARNING` na partida (`server.go:178-185`; decisão SEC-M-02 b da Fase 4). No Linux o aviso cobre apenas `/` e o home do usuário (`guard.go:170-179` só lista diretórios de sistema do Windows): `--allow-root /etc` ou `/proc` sobem **sem aviso**. Mitigação: `--allow-root` só em pastas específicas e conferir o `stderr` na partida." Nova linha em §7 "Aviso de raiz larga não cobre diretórios de sistema do Linux" com a mesma mitigação. Mudança de código fica fora do plano (item da S2 só se o owner decidir; ver "Fora do escopo").

### F-3 (F4) Arquivo `.mcp.json.bak-windows-20261112` dentro da raiz permitida É LIDO pelas tools de arquivo (token entra no prompt)

- **Onde no plano:** changelog 22:20 (linha 167) e §7 "Token em `ps`/`cmdline`" (linha 133) e §7 "Rodar como root".
- **O que o plano diz:** backup em `/root/treino/.mcp.json.bak-windows-20261112` ("agora `0600`; **contém o token em texto**"); `--allow-root /root/treino` no novo `.mcp.json`. O plano não diz que a lista fixa deixa esse nome passar.
- **O que a evidência mostra:**
  - A lista fixa é `files/policy.go:92-119` (`Denied` → `deniedName`), aplicada a **cada segmento** do caminho (diretórios inclusive; `policy.go:92-99`), em minúsculas e sem pontos/espaços finais (`normComponent`, `policy.go:77-79`). Regras, todas sobre o nome já normalizado `n`: igualdade exata com `.git`, `.mcp.json`, `settings.local.json` (`policy.go:105`); prefixo `.env` ou sufixo `.env` (107); sufixo `.pem`, `.key`, `.pfx`, `.p12` (109-110); prefixo `id_rsa`, `id_ed25519`, `id_ecdsa`, `id_dsa` (112-113); contém `credential` ou `secret` (115). Não há glob nem expressão regular.
  - Como `.mcp.json` é **igualdade**, `.mcp.json.bak-windows-20261112` não casa com nenhuma regra: não termina em `.env/.pem/.key/.pfx/.p12`, não começa com `.env`/`id_*`, não contém `credential`/`secret`.
  - Provado executando `files.Denied` e `Guard.Read` (módulo descartável, sem tocar o repositório; a política é a mesma em qualquer SO):

    | Nome (dentro da raiz) | `Denied` | `Guard.Read` |
    | --- | --- | --- |
    | `.mcp.json.bak-windows-20261112` | **false** | **LIDO** (46 bytes, texto devolvido) |
    | `mcp.json.exemplo` | false | LIDO |
    | `mcp.json` (sem ponto) | false | n/a |
    | `.mcp.json` | true | RECUSADO ("lista fixa de nomes sensíveis") |
    | `.MCP.JSON`, `.mcp.json.` | true | n/a |
    | `mcpollama.env` | true (sufixo `.env`) | RECUSADO |
    | `.env.bak` | true (prefixo `.env`) | n/a |
    | `.credentials.json` | true (contém `credential`) | n/a |
    | `.mcp.json.bak`, `.mcp.json~` | **false** | n/a |

  - Consequência: com `--allow-root /root/treino`, um `summarize_file`, `extract` ou `translate` sobre `/root/treino/.mcp.json.bak-windows-20261112` lê o arquivo (regular, `nlink`=1 se não houver hardlink, texto, tamanho menor que o teto de 4 MiB) e **envia o conteúdo ao modelo**. O `CLAUDE.md` do projeto registra que o gateway grava **prompt e resposta completos** em `content.db` (0600, 7 dias, texto visível ao admin; seção Fase 10.L). O token acabaria no prompt, possivelmente na resposta e no `content.db`.
  - Pré-condição: alguém (o Claude do cliente, ou um prompt injetado em outro arquivo da raiz) pedir esse caminho; com uma única raiz as tools aceitam também caminho relativo a ela (`guard.go:345` `g.absolute`; `guard_test.go:146`, FILE-002). Não foi chamada nenhuma tool real nem lido o arquivo do servidor: a prova é no código e num arquivo falso.
- **Classificação:** confirmado (código + execução com conteúdo falso). Estado do arquivo no servidor: não verificável sem o servidor (depende de o `.bak` ainda existir).
- **Gravidade:** **crítico**.
- **Reparo proposto (texto pronto):**
  1. Em §7, nova linha: "**Backup com token dentro do `--allow-root`.** `/root/treino/.mcp.json.bak-windows-20261112` (token em texto, `0600`) está dentro da raiz `/root/treino`. A lista fixa (`files/policy.go:105`) nega só `.mcp.json` por igualdade: o nome com sufixo `.bak-...` **não é negado** e é lido por `summarize_file`/`extract`/`translate`, enviando o token ao modelo e ao `content.db` do gateway (prompt e resposta, 7 dias). Provado em 2026-11-13 (revisão) com `files.Guard.Read` e conteúdo falso. Mitigação: tirar o arquivo da raiz (ou removê-lo) **por decisão do owner**; nunca renomear segredo para fora do padrão da lista. Alternativa de não fazer: manter, aceitando o risco."
  2. Em §8, **questão nova (a partir da 13):** "O `.bak-windows-20261112` com o token em texto deve sair de `/root/treino`? (a) mover para `/root/MCP/` (fora da raiz do MCP, `0600`) (recomendado); (b) apagar, já que o token está em `mcpollama.env`; (c) manter (risco: F-3). Impacto: nenhum serviço usa o arquivo. Rollback: `mv` de volta. Não fazer: o token continua legível pelas tools." Nenhum item da revisão altera o servidor.
  3. Marca `**superado em 2026-11-13 (revisão):**` no changelog 22:20 só se o owner decidir; sem decisão, errata **E-n** ao lado de "agora `0600`; contém o token em texto": `**[errata E-n]** o nome do backup não é coberto pela lista fixa (ver §7, 'Backup com token dentro do --allow-root').`

### F-4 (F4) Nomes comuns de segredo que a lista fixa NÃO cobre (alvo do §7 e do `--allow-root /root`)

- **Onde no plano:** §7 linha "Rodar como root" (linha 132), questão 9 (linha 150).
- **O que o plano diz:** "`--allow-root` só em pastas específicas; avaliar usuário sem privilégio".
- **O que a evidência mostra:** executado `files.Denied` (mesma prova do F-3): **não negados** `.mcp.json.bak`, `.mcp.json~`, `mcp.json`, `.bashrc`, `.netrc`, `.claude.json`, `token.txt`, `api_key.txt`, `authorized_keys`, `shadow`. O changelog 22:35 diz que `CLAUDE_CODE_OAUTH_TOKEN` está no `/root/.bashrc` e que há `/root/.claude/.credentials.json` (este **é** negado, contém `credential`). A revisão da Fase 4 (SEC-M-03, `report.md:129-151`) já listou as lacunas (`.npmrc`, `.pypirc`, `.netrc`, `.pgpass`, `.htpasswd`, `*.ppk`, `*.jks`, `*.keystore`, `*.kdbx`, `*.ovpn`, `terraform.tfstate`, `kubeconfig`, `.kube`, `.docker\config.json`, `.ssh\config`, `.aws`, `.claude.json`, `token.txt`...) e a P-10 ("denylist fixa sem override") deixou a ampliação como decisão do owner; a lista em `policy.go:92-119` **continua igual**.
- **Classificação:** confirmado.
- **Gravidade:** importante (só vira exposição se a raiz sair de `/root/treino`; com `--allow-root /root`, o `.bashrc` com o token OAuth e o `.claude.json` ficam legíveis).
- **Reparo proposto:** em §7, linha "Rodar como root", acrescentar: "Com `--allow-root /root` (home), a lista fixa **não** cobre `.bashrc` (onde o changelog 22:35 diz haver `CLAUDE_CODE_OAUTH_TOKEN`), `.claude.json`, `.netrc`, `.ssh\config`, `known_hosts`, `*.bak` de configs (SEC-M-03 da Fase 4, nunca ampliada; P-10). Por isso a raiz **nunca** deve ser `/root`." Remissão ao `report.md` da Fase 4. Ampliar a lista é decisão do owner e fica **fora do plano** (reportar).

### F-5 (F2) `TestSplitRoots` assume `;`; como corrigir sem tautologia

- **Onde no plano:** changelog 22:45, item B e C(1) (linhas 163); S2 (linha 116).
- **O que o plano diz:** "`TestSplitRoots` (`files/guard_test.go:125`) espera o separador `;` e no Linux o código usa `:` (`os.PathListSeparator`, `guard.go:223`, correto)"; correção: "`TestSplitRoots` com `os.PathListSeparator`".
- **O que a evidência mostra:** `guard_test.go:122-130`: `SplitRoots(" a ; ;b;; c ")` e `strings.Join(got, "|") != "a|b|c"` no `Fatalf` da linha **125** (confere). No Linux `os.PathListSeparator` é `:`, a entrada vira um único item e o teste falha. A entrada com `;` fica em literal na linha 123.
- **Classificação:** confirmado.
- **Gravidade:** importante (gate da S2: "verde nos dois sistemas").
- **Reparo proposto:** na S2, trocar a frase por: "`TestSplitRoots`: manter **literais por plataforma**, sem derivar a entrada de `os.PathListSeparator` (derivar tornaria o teste tautológico: ele passaria com qualquer separador). Entrada `" a ; ;b;; c "` e esperado `a|b|c` quando `runtime.GOOS == "windows"`; entrada `" a : :b:: c "` e esperado `a|b|c` nos demais. Acrescentar a asserção do separador **errado**: fora do Windows, `SplitRoots("/a;/b")` tem **1** item (`;` é caractere válido de nome, como a vírgula do SEC-m-02); no Windows, `SplitRoots(`C:\a;D:\b`)` tem **2** itens. Prova por mutação: trocar `os.PathListSeparator` por `';'` em `guard.go:229` deve fazer o teste falhar no Linux; trocar por `':'` deve fazê-lo falhar no Windows." Errata **E-n** no changelog 22:45, item C(1): `guard.go:223` → `guard.go:229`.

### F-6 (F2, F11) `TestWrongHashServerStarts` assume a mensagem de hash; no Linux a mensagem é a da plataforma (D-10)

- **Onde no plano:** changelog 22:45, item B e C(1); S2 (linha 116); §4.3(a) (linha 90).
- **O que o plano diz:** "espera o erro de hash do OCR e no Linux o servidor sobe com o aviso 'OCR disabled: OCR indisponível nesta plataforma' (comportamento correto, D-10)"; correção: "`t.Skip` fora do Windows ou outra expectativa". §4.3(a): "mensagem de erro do `--ocr-lib` no Linux, para dizer claramente 'OCR só no Windows' em vez de falhar de forma obscura".
- **O que a evidência mostra:**
  - `mcpollama-mcp/ocr_test.go:164-178`: o `Fatalf` da linha **173** exige `"WARNING: OCR disabled: "` **e** `"SHA-256"`. Confere.
  - No Linux, `setupOCR` (`ocr.go:103-150`) chama `c.ocrConfig()` e depois `newPoolFn` = `ocr.NewPool`, que **antes de checar o hash** devolve `ErrUnsupportedPlatform` (`ocr/pool.go:65-67`), cujo texto é `ocr: OCR indisponível nesta plataforma` (`ocr/ocr.go:81`). Mensagem impressa por `lg.logf` (`ocr.go:109`), no **stderr** (`main.go:64`, o `syncWriter` do `stderr`):
    `WARNING: OCR disabled: ocr: OCR indisponível nesta plataforma; the file tools run without OCR (scanned PDFs and images are refused)`
  - **Nunca é fatal** no Linux (comentário `ocr.go:20-23`; `main.go:64-70`).
  - **Ressalva ao plano:** `ocrConfig` (`ocr.go:32-77`) valida **antes** da plataforma: caminho absoluto (`ocr.go:37`), hash presente (40) e números. Com um `.mcp.json` copiado do Windows (`--ocr-lib D:\MCP\libtesseract64.dll`), `filepath.IsAbs` é **falso** no Linux e o aviso sai como `--ocr-lib must be an absolute path`, **não** como "indisponível nesta plataforma". Só com caminho Linux absoluto, hash e demais números válidos aparece a mensagem de plataforma. Isto vem da leitura do código; não foi executado no Linux.
  - `TestSetupOCRWarnings` (`ocr_test.go:133-160`) **passa trivialmente** no Linux: os casos `hash differs`, `missing dll` e `no models` caem todos no erro de plataforma e o teste só exige `"WARNING: OCR disabled: "`. Ele passa, mas deixa de exercitar o hash e os modelos fora do Windows. `TestProbeFailureTurnsOCROff` já usa `t.Skip` (`ocr_test.go:184-186`, "OCR is Windows only (D-10)").
- **Classificação:** confirmado (o teste), refutado em parte (§4.3(a): a mensagem de plataforma já é clara, mas fica atrás das validações de caminho/hash).
- **Gravidade:** importante.
- **Reparo proposto:** na S2, trocar "com `t.Skip` fora do Windows ou outra expectativa" por: "**não** usar `t.Skip` neste teste: ele passaria a ignorar justamente o aviso da D-10 no Linux. Manter as asserções comuns (exit 0, 1 linha, `summarize_file` presente, sem o caminho da raiz no stderr) e ramificar a expectativa do `stderr`: no Windows exigir `SHA-256`; fora dele exigir `OCR indisponível nesta plataforma` (texto de `ocr/ocr.go:81`). Mutação: remover o `if !platformSupported` de `ocr/pool.go:65` deve quebrar o teste no Linux." Em §4.3(a), acrescentar: "A mensagem já existe e a partida nunca falha (`ocr.go:103-150`); o que pode confundir é a **ordem**: com caminho Windows (`D:\...`) o aviso diz 'deve ser absoluto' (`ocr.go:37`) em vez de 'OCR só no Windows'. Candidato a ajuste só se o owner quiser (verificar a plataforma primeiro em `setupOCR`)."

### F-7 (F2) Binários de teste executados no servidor dependem dos fontes ao lado (`TestNoLiteralContextLimits`)

- **Onde no plano:** changelog 22:45, item B (linha 163): "Um terceiro FAIL (`TestNoLiteralContextLimits`) foi erro meu do envio (não copiei `src/mcp`)"; S1(c) (linha 115).
- **O que a evidência mostra:** `mcpollama-mcp/config_test.go:193-205` lê `config.go`, `server.go`, `main.go`, `logger.go` e `../mcp/types.go` do **diretório corrente** (exige o cwd = pasta do pacote, com `../mcp/types.go`). Confere com o relato do plano. Outros testes também dependem do cwd: `mcpollama-mcp/tools_wiring_test.go:409` e `tools/registry_test.go:169` leem `../../TOOLS.md`; `ollama/security_test.go:216-231` faz `filepath.Glob("*.go")` e falha com "no source files found" sem os fontes. O plano acerta a causa mas só cita `src/mcp`.
- **Classificação:** confirmado (e ampliado).
- **Gravidade:** informativo.
- **Reparo proposto:** em S1(c) e no changelog 22:45 (errata, não corpo): "binário de teste compilado com `go test -c` exige, ao lado, os `.go` do pacote, `../mcp/types.go` e `../../TOOLS.md`; sem eles os FAIL são de ambiente, não do produto (`config_test.go:194`, `tools_wiring_test.go:409`, `tools/registry_test.go:169`, `ollama/security_test.go:216`)". Isto também vale para a "não executado" de `gateway/*`.

### F-8 (F3) O `--help` do `--allow-root` já cita `:` no Linux

- **Onde no plano:** changelog 22:45, item C(2) (linha 163); S2/S4b (linhas 116, 119).
- **O que o plano diz:** "o `--help` (`config.go:144`) e a documentação dizem só `;`, o que engana"; correção "do texto do `--help`".
- **O que a evidência mostra:** `config.go:144-146` (bloco de `--help`, constante de texto): `--allow-root DIR[;DIR]  absolute directories the file tools may read, separated by ;` e, **na linha seguinte (145)**, `(: on Linux), not nested; without it`. O texto já diz "`:` on Linux" desde `9ef7935` (2026-10-23, `git blame` da linha 144-145). O trecho enganoso é a **primeira linha** (`DIR[;DIR]`, `separated by ;`) e o comentário do campo `AllowRoot` (`config.go:59`: "separated by ; (os.PathListSeparator)"). A documentação confirma só `;`: `docs/INSTALACAO.md:764` ("várias pastas absolutas separadas por `;`") e `D:\MCP\Documentation\Ollama\02-mcp-json-do-zero.md:24` ("Várias raízes: `E:\Projeto1;E:\Projeto2`").
- **Classificação:** refutado em parte (a alegação "diz só `;`" é falsa para o `--help`; é verdadeira para `docs/INSTALACAO.md` e a documentação do `D:\MCP`).
- **Gravidade:** informativo (reduz o escopo da correção de código; o ponto principal é a documentação).
- **Reparo proposto:** errata **E-n** no changelog 22:45, item C(2): `**[errata E-n]** o --help já diz "(: on Linux)" na linha seguinte (config.go:145); a primeira linha (config.go:144, "DIR[;DIR] ... separated by ;") e o comentário config.go:59 omitem o ":", e a documentação (docs/INSTALACAO.md:764; D:\MCP\Documentation\Ollama\02-mcp-json-do-zero.md:24) cita só ";".` Na S2: "ajuste de texto do `--help` opcional (primeira linha)"; na S4/S4b: separador `:` obrigatório na documentação Linux, com exemplo `--allow-root /root/treino:/root/outra`.

### F-9 (F3) Listas em flags: só duas, e uma depende do SO

- **Onde no plano:** não há (o plano trata só `--allow-root`).
- **O que a evidência mostra:**
  - `--allow-root`/`MCPOLLAMA_ALLOW_ROOT`: `os.PathListSeparator` (`;` no Windows, `:` nos demais) (`guard.go:227-235`).
  - `--allowed-models`/`MCPOLLAMA_ALLOWED_MODELS`: `;` **ou** `,` em qualquer SO (`tools/allowlist.go:15`, `strings.FieldsFunc(s, r == ';' || r == ',')`; `config.go:134`). Os nomes têm `:` (`qwen3.8:27b`), por isso o separador **não** é `:` nem muda no Linux.
  - Demais flags (`--ocr-*`, `--client-label`, `--timeout`, `--max-*`) não são listas. `tools/reportar_erro.go:122` separa por `,` um campo de argumento de tool, não uma flag.
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** na S4b(a) e na §4.4, tabela de 2 linhas: "`--allow-root`: `:` no Linux, `;` no Windows; `--allowed-models`: `;` ou `,` nos dois (os nomes têm `:`)". Não trocar o `;` do `--allowed-models` por `:` ao converter a documentação (erro provável de substituição mecânica).

### F-10 (F5) `reportar_erro` NÃO depende do `--allow-root`; o §1 item 3 o agrupa errado

- **Onde no plano:** §1 item 3 (linha 25): "(`list_models`, `generate`, `chat`, `embed` e, **com `--allow-root`**, `summarize_file`, `extract`, `translate`, `reportar_erro`)"; S6 (linha 121).
- **O que a evidência mostra:** `tools/registry.go:99-109`: `New` registra sempre `list_models`, `generate`, `chat`, `embed` e `reportar_erro` (comentário na linha 105: "needs no --allow-root"); só com `d.Files != nil` acrescenta `summarize_file`, `extract`, `translate` (`registry.go:107-109`; `FileToolNames = {extract, summarize_file, translate}`, `tools/filetools.go:58`). São **5 tools base** e **3 de arquivo** (8 no total, o que bate com o `tools/list` do changelog 21:55). `docs/INSTALACAO.md:764` ainda diz "ficam só as 4 base" (desatualizado, fora do plano).
- **Classificação:** confirmado (o plano erra o agrupamento).
- **Gravidade:** informativo.
- **Reparo proposto:** errata **E-n** no §1 item 3: "`reportar_erro` faz parte das 5 tools base e **não** exige `--allow-root`; com `--allow-root` aparecem só `summarize_file`, `extract` e `translate` (`tools/registry.go:99-109`)". Em S6, o gate "cada tool responde" deve listar `reportar_erro` entre as que funcionam **sem** `--allow-root` (ver etapa 05 para o que falta testar).

### F-11 (F6) Versões das dependências e o que entra no binário

- **Onde no plano:** §2.1 linhas 46, 47, 49.
- **O que o plano diz:** `go-pdfium v1.21.4`, `wazero v1.12.0`, `go-commons-pool v2.1.2`, `google/uuid v1.6.0`, `golang.org/x/sys v0.49.0`, `golang.org/x/text v0.43.0` "(lista de `go version -m` do binário Linux)"; `modernc.org/sqlite`, `x/image`, `libc`, `bigfft` "usadas pelo gateway e pelo OCR/imagens, não estão no binário"; "**não confirmei** por inspeção que o `.wasm` está embutido".
- **O que a evidência mostra:**
  - `go version -m D:\MCP\mcpollama-mcp` lista exatamente essas 6 dependências e versões (`dep github.com/klippa-app/go-pdfium v1.21.4`, `wazero v1.12.0`, `go-commons-pool/v2 v2.1.2`, `google/uuid v1.6.0`, `golang.org/x/sys v0.49.0`, `golang.org/x/text v0.43.0`). `GOOS=linux CGO_ENABLED=0 go list -deps ./mcpollama-mcp` (módulos) dá o mesmo conjunto; no Windows também. `src/go.mod`: `go-pdfium v1.21.4`, `wazero v1.12.0`, `x/image v0.46.0`, `x/sys v0.49.0`, `modernc.org/sqlite v1.61.0` (diretas); `uuid v1.6.0`, `go-commons-pool/v2 v2.1.2`, `x/text v0.43.0` etc. (`// indirect`).
  - `modernc.org/*` só é importado por `src/gateway/{content,store,treino,...}` (fora do binário do MCP). **`golang.org/x/image` só aparece em testes com tag `ocr`** (`mcpollama-mcp/ocr_real_test.go`, `ocr/ocr_real_test.go`), não no OCR de produção: "pelo OCR/imagens" é impreciso.
  - O `.wasm` embutido pode ser provado **estaticamente**: `go list -deps` do MCP inclui o pacote `github.com/klippa-app/go-pdfium/internal/pdfium_wasm` (e `implementation_webassembly`), que embute o módulo; a prova funcional já veio de ler um PDF no Linux (changelog 22:45).
- **Classificação:** confirmado (com 1 imprecisão informativa).
- **Gravidade:** informativo.
- **Reparo proposto:** em §2.1 linha 49, errata **E-n**: "`modernc.org/*`: só no gateway; `x/image`: só em testes com tag `ocr`, não no OCR de produção." Na linha 47, marca `**superado em 2026-11-13 (revisão):** o pacote do motor WASM (`go-pdfium/internal/pdfium_wasm`) consta em `go list -deps ./mcpollama-mcp` e a leitura de PDF no Linux foi provada no changelog 22:45 (3.F fechado)`.

### F-12 (F7) CHANGELOG, tags e "0.10.3-b1faec3" ainda descrevem o código

- **Onde no plano:** D15-1 (linha 100); S7 (linha 122); S8 (linha 123); §2 item 2.
- **O que o plano diz:** "Versão: 0.10.4 só se houver mudança de código; ... o binário continua `0.10.3`"; S7: "CHANGELOG (seção 'Não lançado' ou versão, conforme D15-1)".
- **O que a evidência mostra:**
  - `CHANGELOG.md`: **não há `0.10.4`**; **há** a seção `## Não lançado` (linha **53**) e `## mcpollama` com `### [0.10.3]` na linha 73 como a versão mais recente. A entrada 0.10.3 já diz que o binário Linux foi "atualizado para 0.10.3-b1faec3 em 2026-11-07 20:07 (compilado com `GOOS=linux`, `CGO_ENABLED=0`; backup `.bak-0.10.2`; **não executado em Linux**)": ficou vencida pela Fase 15 (executado no `lx-ai01`).
  - Tags: `mcpollama-v0.8.1`, `v0.9.0`, `v0.9.1`, `v0.9.2`, `v0.10.0`, `v0.10.1`, `v0.10.2`, `v0.10.3`; `mcpollama-v0.10.3` aponta para `b1faec3ecacb`; tags do gateway até `gateway-v0.13.4`.
  - `git diff --stat b1faec3 HEAD -- src/mcpollama-mcp src/tools src/files src/doctext src/ollama src/mcp src/redact src/ocr src/go.mod src/go.sum`: **vazio**. A única mudança em `src/` desde `b1faec3` é `src/gateway/monitor/index.html` e `responsive_test.go` (gateway 0.13.4). `git diff b1faec3 4ae3a87 -- src`: vazio.
  - Logo `0.10.3-b1faec3` continua descrevendo o código do MCP. Detalhe: o binário Linux em `D:\MCP` traz `vcs.revision=4ae3a87e43329de4f57033c1212840ac60359fe9` (commit só de docs, 2026-11-07 20:02:55) com o rótulo `-X main.Version=0.10.3-b1faec3`; o `.exe` traz `vcs.revision=b1faec3e...` (ver F-13). Mesmo código, hash curto do rótulo diferente do `vcs.revision` do Linux.
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** em D15-1, acrescentar "Conferido em 2026-11-13 (revisão): `git diff b1faec3 HEAD` nos pacotes do MCP e no `go.mod/go.sum` é vazio; não há `0.10.4` no `CHANGELOG.md` e `0.10.3` é a última entrada de `mcpollama`". Em S7, apontar a linha a atualizar: "`CHANGELOG.md:81` (`não executado em Linux`) deixa de valer" e a seção `Não lançado` (linha 53) como destino da nota da Fase 15, **sem número de versão** (D15-1).

### F-13 (F8) Comando de build Linux do `CLAUDE.md`, `-trimpath` e reprodutibilidade do hash

- **Onde no plano:** §2 item 2 (linha 33: "gerado por `GOOS=linux go build` (CLAUDE.md, seção Comandos)"); §4.2 (linhas 84-86); S3 (linha 117); §7 "Dois binários Linux divergirem" (linha 136).
- **O que o plano diz:** build fixo `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=<versão>-<hash>"`; "`SHA256SUMS` do binário gravado ao lado (como no `Build-Release.ps1` do molde, MCPO-REQ-BUILD-004) e conferido na instalação"; "S3 gera um só e copia".
- **O que a evidência mostra:**
  - `CLAUDE.md:84` (HEAD e árvore de trabalho; a outra sessão não mexeu nessa linha): `$env:GOOS='linux'; go build -o D:\MCP\mcpollama-mcp ./mcpollama-mcp; Remove-Item Env:GOOS   # binário Linux → D:\MCP`. **Não tem** `CGO_ENABLED=0`, **nem** `-X main.Version`, nem `-trimpath`. O mesmo em `README.md:314` e `AGENTS.md:84`. (O `CGO_ENABLED=0` acontece implicitamente no cross-compile.)
  - `go version -m D:\MCP\mcpollama-mcp` (linhas `build`): `-buildmode=exe`, `-compiler=gc`, `-ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `GOARCH=amd64`, `GOOS=linux`, `GOAMD64=v1`, `vcs=git`, `vcs.revision=4ae3a87e...`, `vcs.time=2026-11-07T23:02:55Z`, `vcs.modified=false`. **Sem `-trimpath`**, sem `-s -w`. O `.exe`: mesmos campos, `GOOS=windows`, `vcs.revision=b1faec3e...`, `vcs.time=2026-11-07T04:27:15Z`.
  - O binário implantado (sha256 `d61f5e82…40bd0333`, 22.770.050 bytes; confere com o changelog 21:55 "`d61f5e82…0333`") foi, portanto, feito com um comando **diferente** do documentado no `CLAUDE.md` e do proposto no §4.2. Refazer o build pelo §4.2 produz **outro hash** (e outro tamanho, por `-s -w`) para a mesma versão e o mesmo código; além disso o `go build` grava `vcs.revision`/`vcs.time`, então um rebuild a partir de HEAD também muda o hash. O `SHA256SUMS` só prova **cópia fiel**, não "build reproduzido".
  - Não existe script de build no repositório: `Build-Release.ps1` só existe em `referencias/MCPVMWare/tools/` (somente leitura, não versionado). MCPO-REQ-BUILD-004 (`60-Requisitos_mcpollama.md:272`) o exige com `-trimpath -s -w` e `SHA256SUMS.txt`, e a matriz o marca "Resolvido" (`61-Matriz_Cobertura.md:246`) sem script próprio.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:**
  1. Errata **E-n** no §2 item 2: "o `CLAUDE.md:84` documenta `GOOS=linux go build` **sem** `CGO_ENABLED=0` nem `-X main.Version`; o binário implantado foi feito com `-ldflags "-X main.Version=0.10.3-b1faec3"`, sem `-trimpath` e sem `-s -w`, a partir do commit `4ae3a87` (`go version -m`, lido em 2026-11-13)."
  2. §4.2: acrescentar "O `SHA256SUMS` garante **cópia** (mesmo arquivo no destino), não reprodutibilidade: o `go build` embute `vcs.revision`/`vcs.time`; para hash estável usar `-buildvcs=false`. O hash da §4.2 **não** será o do binário de 07/11 (`d61f5e82…40bd0333`) porque o comando muda (`-trimpath -s -w`)." Em §7 "Dois binários Linux divergirem": adicionar "a S3 **substitui** o binário de `D:\MCP` e o de `/root/MCP` pelo mesmo arquivo, com backup `.bak-<versão>`".
  3. S3: incluir "criar o script de build/`SHA256SUMS`" como entregável explícito **ou** declarar que o comando documentado é manual (hoje a §4.2 diz "repetível" e não há script). Decisão do owner: nova questão (a partir da 13) "script de build no repositório (`tools/Build-Release.ps1`, novo arquivo de código/ferramenta) ou comando documentado?".

### F-14 (F9) `D:\MCP`: estado real

- **Onde no plano:** §2 item 5 (linha 36), §2.1 linhas 50-51, §4.1 (linhas 68-80).
- **O que o plano diz:** `mcpollama-mcp.exe` e `mcpollama-mcp`, backups `.bak-<versão>` por versão, `mcpollama-gateway`, `cache/pdfium`, `tessdata/` e `libtesseract64.dll`, `Documentation/Ollama/` com 7 arquivos + `README.md` + `.bak-<data>`.
- **O que a evidência mostra (`ls -la --time-style=long-iso /d/MCP`):**

  | Item | Tamanho | Data |
  | --- | --- | --- |
  | `mcpollama-mcp` (Linux) | 22.770.050 | 2026-11-07 20:07 |
  | `mcpollama-mcp.bak-0.10.1` | 22.769.930 | 2026-11-05 12:02 |
  | `mcpollama-mcp.bak-0.10.2` | 22.769.938 | 2026-11-05 12:57 |
  | `mcpollama-mcp.exe` | 23.280.640 | 2026-11-07 16:04 |
  | `.exe.bak-0.9.2` / `-0.10.0` / `-0.10.1` / `-0.10.2` | 23.170.560 / 23.275.008 / 23.279.104 / 23.279.104 | 10-31 22:21 / 11-04 13:06 / 11-05 12:02 / 11-05 12:57 |
  | `mcpollama-gateway` | 19.382.276 | 2026-11-08 18:53 |
  | `libtesseract64.dll` | 5.965.824 | 2026-05-05 02:09 |
  | `cache\pdfium\wazero-v1.12.0-amd64-windows` | pasta | 2026-10-28 17:43 |
  | `tessdata\best` e `fast`: `por.traineddata`, `osd.traineddata` | 8.159.939 e 10.562.727 (best); 1.982.756 e 10.562.727 (fast) | 2026-05-05 |
  | `TruenasMcp.exe`, `mcpvmware-mcp.exe` | 15.545.344; 21.637.632 | 2026-08-09; 2026-08-25 |

  - Confere: 22.770.050 B e 2026-11-07 20:07 (§2 item 2); o `.exe` tem backups por versão (4); o Linux tem **só 2** (`0.10.1` e `0.10.2`; não há `0.10.0`, `0.9.x` nem `0.10.3-anterior`); o `cache/` tem a subpasta de **Windows** (`wazero-v1.12.0-amd64-windows`); a DLL tem 5,9 MB.
  - `Documentation/` **não** tem só `Ollama/`: tem também `LEIA-ME-IA.md` (7.976 B), `README.md` (9.341 B), `TrueNAS/` e `VMware/` (2026-11-04/05). `Documentation/Ollama/`: 7 numerados (`01` a `07`), `README.md` e **6** `.bak-<data>` (`03-parametros.md.bak-20261108`, `04-tools.md.bak-20261107` e `-20261108`, `06-solucao-de-problemas.md.bak-20261108`, `README.md.bak-20261107` e `-20261108`): 14 arquivos. Os `.bak` são **por data**, não por versão.
  - O backup por versão do Linux só existe até onde o owner/Claude fez `mv` antes de sobrescrever: não há política garantida.
- **Classificação:** confirmado (com detalhes a acrescentar).
- **Gravidade:** informativo.
- **Reparo proposto:** em §2 item 5, acrescentar a tabela acima (com data de leitura 2026-11-13) e a nota: "`Documentation/` tem também `LEIA-ME-IA.md`, `README.md`, `TrueNAS/`, `VMware/` (fora desta fase); backups do binário Linux existem só para 0.10.1 e 0.10.2; os `.bak` da documentação são por data". Na §4.1, a linha "`mcpollama-mcp.bak-<versão>` (um backup por troca, mesma convenção do `D:\MCP`)" ganha "(no `D:\MCP` hoje: `bak-0.10.1`, `bak-0.10.2`)".

### F-15 (F9) A "fonte única" da documentação no repositório (§4.4) não existe: a documentação de uso só existe em `D:\MCP`, fora do git

- **Onde no plano:** §4.4 (linha 94): "A fonte **única** é o repositório (`docs/`, ver questão 7); as cópias em `D:\MCP\Documentation` e `/root/MCP/Documentation` são geradas dela, não editadas à mão"; S7 (linha 122); questão 7 (linha 148); §2 item 6.
- **O que a evidência mostra:**
  - `git ls-files | grep -iE "pre-requisitos|mcp-json-do-zero|03-parametros|04-tools|07-checklist|LEIA-ME-IA|Documentation/Ollama"`: **vazio**. Os 7 documentos de uso (`01-pre-requisitos-e-copia` a `07-checklist-ia`) **não estão versionados** em lugar nenhum do repositório; só existem em `D:\MCP\Documentation\Ollama` (e na cópia do servidor, feita a partir dali).
  - `docs/` tem 5 arquivos: `INSTALACAO.md`, `MANUAL-GATEWAY.md`, `README.md`, `RELATAR-ERROS-IA.md`, `CONTRATO-DADOS-TREINO.md` (`git ls-files docs`). O `docs/INSTALACAO.md` cobre a **estação Windows** (D.1-D.5), `E.1` (`.mcp.json` com `D:\\MCP\\mcpollama-mcp.exe`) e G.1 (build/troca): **nenhuma seção de cliente MCP no Linux** (o servidor Linux aparece só como servidor de IA, seção B). Não existe "documentação do cliente/instalação do mcpollama-mcp" Linux no repositório.
  - A questão 7 do plano é uma pergunta aberta, mas o §4.4 e a S7 a tratam como fato.
- **Classificação:** refutado como afirmação factual ("a fonte única é o repositório"); válido como proposta, mas falta o passo de criá-la.
- **Gravidade:** importante (S4, S4b e S7 dependem de onde está a fonte; sem a importação inicial, "gerar a partir de `docs/`" não tem origem).
- **Reparo proposto:** no §4.4, marca `**superado em 2026-11-13 (revisão):**` sobre "A fonte **única** é o repositório" com: "Hoje **não é**: os 7 documentos de `Documentation/Ollama` só existem em `D:\MCP` (fora do git; `git ls-files` vazio). Para tornar o repositório a fonte única é preciso um passo novo, **antes** da S4: importar os 7 + README para `docs/` (ou `docs/ollama/`), em Windows e em versão Linux, com backup, e depois gerar as cópias. Isso é decisão da questão 7 e cria arquivos em `docs/` (área não protegida pelo `CLAUDE.md`, mas é conteúdo novo)." Em S7, trocar "`docs/` (fonte única)" por "`docs/` (fonte única, **depois** da importação)". Em §2 item 6, acrescentar "(fora do git)".

### F-16 (F10) Referências a outros planos e à revisão da Fase 4

- **Onde no plano:** frontmatter/`Relacionados` (linha 15); D15-7 (linha 106); §4.3 e §7 ("testes de ataque da Fase 4").
- **O que o plano diz:** "Fase 14 (`0.10.3`, é a versão-base)"; "metodologia da §11 do plano da Fase 12"; "(§11.1 da Fase 12)"; "testes de ataque da Fase 4".
- **O que a evidência mostra:**
  - "§11 do plano da Fase 12": existe em `.workspace\plans\MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`, linha **515** (`## 11. Metodologia de trabalho`), `### 11.1 Regras do owner` na 517 (inclui "**Push e tags só com autorização do owner**", ~linha 539); 11.2 testes e gates (547), 11.3 implantação (567), 11.4 ambiente e armadilhas (579). Confere. O plano **não dá o caminho** do arquivo.
  - Fase 14: o plano está **movido** (árvore de trabalho, **ainda sem commit**) de `.workspace/plans/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` para `.workspace/plans/concluidos/` (o `git status` mostra `D` na origem e `??` em `concluidos/`). O plano da Fase 15 cita "Fase 14" só pelo nome, sem caminho; portanto **não há link quebrado**. Um caminho adicionado agora precisa ser o de `concluidos/` e só vale depois que a outra sessão commitar a mudança.
  - O relatório da revisão adversarial da Fase 4 existe: `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md` e `.testes/` (`files/` com 4 arquivos, `mcpollama-mcp/e2e_attack_test.go`, `tools/tools_attack_test.go`, `correcao-proposta/`). O plano não dá o caminho (CLAUDE.md o cita).
  - **Relevante para o Linux e ausente do plano:** esses testes de ataque foram escritos e rodados no **Windows 11** (`report.md:7`); o próprio relatório marca como **não testável** a hipótese **SEC-m-08** (TOCTOU com FIFO no Linux: entre `rt.Stat` e `rt.Open` trocar por FIFO travaria o `open`; `report.md:221-225`) e `guard.go:394` ainda faz `rt.Open(rel)` sem `O_NONBLOCK` (código atual). `Mkfifo` não aparece em nenhum arquivo de `.testes`. O SEC-m-07 ("`guard_test.go` não compila para Linux") foi corrigido (existe `files/guard_windows_test.go`; `GOOS=linux go vet ./...` passa, F-19).
- **Classificação:** confirmado (referências válidas e sem link quebrado); o ponto SEC-m-08 é lacuna.
- **Gravidade:** importante (SEC-m-08 e "repetir os testes de ataque" no Linux entram na etapa 04).
- **Reparo proposto:** em `Relacionados`, acrescentar os caminhos: "plano da Fase 12: `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` §11; Fase 14: `.workspace/plans/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` (movido para `plans/concluidos/` na árvore de trabalho em 2026-11-13, sem commit); revisão da Fase 4: `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md`". Em §7, linha `src/files/`: "os testes de ataque da Fase 4 foram escritos no Windows; **SEC-m-08 (FIFO, Linux) nunca foi testado** (`report.md:221-225`; `guard.go:394` sem `O_NONBLOCK`). Rodá-los no Linux é parte da S1(c) ou da S2 e exige adaptação (`syscall.Mkfifo`); não confundir com `go test ./files`."

### F-17 (F11) Texto exato do aviso do OCR no Linux e onde sai

- **Onde no plano:** §2 item 3 (linha 34); changelog 22:45 (item C(3)).
- **O que o plano diz:** `ocr/ocr_other.go` com `platformSupported = false` e `OpenTesseract` devolvendo `ErrUnsupportedPlatform`; "o servidor sobe com o aviso 'OCR disabled: OCR indisponível nesta plataforma'".
- **O que a evidência mostra:** `ocr/ocr_other.go:15` (`const platformSupported = false`), `:21` (`OpenTesseract` → `ErrUnsupportedPlatform`). O caminho do MCP, porém, é `NewPool` (`ocr/pool.go:65-67`), não `OpenTesseract` (que só roda no worker filho). `ErrUnsupportedPlatform = errors.New("ocr: OCR indisponível nesta plataforma")` (`ocr/ocr.go:81`). Mensagem final (`mcpollama-mcp/ocr.go:108-110`), em **stderr** (`main.go:64`):
  `WARNING: OCR disabled: ocr: OCR indisponível nesta plataforma; the file tools run without OCR (scanned PDFs and images are refused)`
  O plano resume como "OCR disabled: OCR indisponível nesta plataforma": falta o prefixo `ocr: ` e a cauda; para testes `Contains` o trecho do plano serve.
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** em §2 item 3, errata **E-n** com o texto exato acima; "o ponto de bloqueio é `ocr.NewPool` (`pool.go:65`); `OpenTesseract` só roda no worker (`ocr_other.go:21`)". Ver F-6 para a ordem das validações.

### F-18 (F12) `nvidia-powercap.service` e `CLAUDE.md` ainda descrevem +1500/+150

- **Onde no plano:** changelog 22:45, "Achado de documentação" (linha 157).
- **O que o plano diz:** a unit foi alterada **às 18:12 de hoje (12/11)** para memória e núcleo 0; "o `CLAUDE.md` e `deploy/servidor-ia/nvidia-powercap.service` do repositório ainda descrevem o estado de 08/11 (+1500/+150) e precisam ser atualizados (fora do escopo desta fase; **não alterei**)".
- **O que a evidência mostra:** `deploy/servidor-ia/nvidia-powercap.service:2` ("offset de memoria +1500 e nucleo +150"), `:14-17` (comentário e `ExecStart` com `oc set "$i" mem 1500` e `oc set "$i" graphics 150`); `CLAUDE.md:186` ("memória +1500 MHz e núcleo +150 MHz") e `:197-199`. Último commit do arquivo: `c4fa349` (2026-11-08). O repositório está, de fato, no estado de 08/11. O estado do servidor (0/0) **não é verificável sem o servidor**.
- **Classificação:** confirmado para o repositório; servidor, não verificável sem o servidor.
- **Gravidade:** informativo (fora do escopo do plano).
- **Reparo proposto:** nenhum no plano além de manter a frase e marcar "não reverificado na revisão de 2026-11-13, que não acessou o servidor". Reportar ao owner em "Fora do escopo".

### F-19 (F13) Resultados de `go vet` (Linux) e `go test` (Windows)

- **O que a evidência mostra:**
  - `cd E:\MCPOllam\src; GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...`: **código 0, sem diagnóstico** (12,6 s). O `vet` compila os `_test.go`, então **todos os testes compilam para Linux** (inclusive `gateway/*` e `cmd/*`).
  - `go test -count=1 ./files ./mcpollama-mcp` (Windows): `ok github.com/cslsoftwares/mcpollama/files 1.968s`, `ok github.com/cslsoftwares/mcpollama/mcpollama-mcp 5.144s`.
  - Depois das execuções, `git status --short src` vazio (nada alterado).
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** em §6.1 (nova, "Resultado dos testes no Linux") e no §3 fato B: "2026-11-13 (revisão): `GOOS=linux go vet ./...` código 0 em `src/` (compila todos os testes para Linux); `go test -count=1 ./files ./mcpollama-mcp` verde no Windows. Não reverificado no Linux (a revisão não acessou o servidor)." Isto **não** fecha o `go test` num Linux (as 2 falhas de F-5/F-6 só aparecem lá).

### F-20 (§2 item 4) Cabeçalho da estação e rótulo no Linux

- **Onde no plano:** §2 item 4 (linha 35).
- **O que a evidência mostra:** `station.go:144-172`: `Computer` = `COMPUTERNAME` ou `hostname`; usuário de `user.Current()` (sem domínio se não houver `\` ou `/`); `Domain` = `USERDOMAIN` ou o domínio do `user.Current()`; MAC = WMI (nil no Linux, `station_wmi_other.go:6`) → `PLACA` → interface ativa com IPv4 privado (`interfaceMAC`). Rótulo padrão `<host>/<user>` (`config.go:508-529`, `defaultLabel`; cai em `<host>/<pid>` sem usuário). Confere com o plano, inclusive `<hostname>/root` como root.
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** nenhum.

---

## Não verificado nesta etapa

- Qualquer estado do servidor `lx-ai01` (existência hoje de `.mcp.json.bak-windows-20261112`, `/root/MCP`, `/etc/hosts`, `nvidia-powercap` 0/0, binário implantado). A revisão não acessou o servidor.
- Execução do binário e do `go test` num Linux (o `go vet` para Linux só compila; as 2 falhas de F-5/F-6 foram lidas no changelog 22:45 e confirmadas por leitura de código, não reexecutadas).
- A mensagem do OCR com caminho Windows no Linux (F-6): inferida do código (`ocr.go:37`), não executada em Linux.
- Se `Guard.Read` no Linux se comporta como no Windows para o F-3: a **política** (`Denied`) é independente de SO e foi executada no Windows; a leitura (`Guard.Read`) foi executada no Windows; no Linux o caminho de código é o mesmo (`rt.Open`, `linkCount`), mas não foi rodado.
- Conteúdo das transcrições e dos recuperados (não necessários para esta etapa).

## Fora do escopo do plano (só reportar)

1. **Lista fixa de nomes sensíveis (`files/policy.go:92-119`):** ampliar para `.mcp.json*`, `.npmrc`, `.netrc`, `.bashrc`, `.claude.json`, `*.tfstate`, `kubeconfig`, `.ssh`, `.aws`, nomes com `token`/`password`/`apikey` (SEC-M-03 da Fase 4, P-10 "sem override" deixou como decisão do owner). Mudar `src/files/` exige rodar de novo os testes de ataque da Fase 4.
2. **`DefaultBroadDirs` sem diretórios de sistema do Linux (`files/guard.go:170-179`):** `--allow-root /etc`, `/proc`, `/usr` sobem sem aviso. Mudança de código em `src/files/`.
3. **SEC-m-08 (FIFO TOCTOU, Linux):** `files/guard.go:394` usa `rt.Open(rel)` sem `O_NONBLOCK`; hipótese da Fase 4 nunca testada no Linux.
4. **Ordem das validações do OCR (`mcpollama-mcp/ocr.go:32-77` antes de `ocr.NewPool`):** com `--ocr-lib` de caminho Windows o aviso no Linux é "deve ser absoluto", não "OCR só no Windows". Ajuste opcional de código.
5. **Texto do `--help` (`config.go:144`) e comentário `config.go:59`:** citam `;` na primeira linha; o ":" no Linux está só na linha seguinte.
6. **Documentação desatualizada:** `docs/INSTALACAO.md:764` ("ficam só as 4 base", agora 5 com `reportar_erro`); `docs/INSTALACAO.md` e `CLAUDE.md:84` sem comando de build Linux completo; `CHANGELOG.md:81` ("não executado em Linux").
7. **`deploy/servidor-ia/nvidia-powercap.service` e `CLAUDE.md:186-199`:** ainda em +1500/+150 (o plano diz que o servidor passou a 0/0 em 12/11 às 18:12; não verificável).
8. **Planos movidos para `.workspace/plans/concluidos/`:** a mudança está só na árvore de trabalho (outra sessão, sem commit); documentos e relatórios que citam os caminhos antigos (`deploy/gateway/README.md`, `.workspace/reports/*`) ficam com caminhos desatualizados.
9. **Um segundo grupo de itens da árvore de trabalho** (`AGENTS.md`, `CLAUDE.md`, `README.md` modificados por outra sessão, `.playwright-mcp/` não rastreado): não tocados.

---

## Verificação adversarial (opus, 2026-11-13 11:58)

**Modelo:** Opus 5.5 · **Hora real:** início 11:52:20, fim 11:58 (`date`). **Método:** reli cada fonte primária citada (código em `E:\MCPOllam\src`, plano, `git log/blame/diff/tag`, `D:\MCP` por `ls`/`go version -m`, `recuperado/evidence-exec.txt` e `timeline-fase15.md`). Rodei de novo, por conta própria: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` (rc=0), `go test -count=1 ./files ./mcpollama-mcp` (ok 1,2 s e 6,3 s) e um programa descartável no scratchpad (`v03check`, apagado ao fim) que chama `files.Denied` e `Guard.ReadContext` com conteúdo **falso**. `git status --short src` vazio antes e depois. Nada de ssh/scp/curl, `mcp__mcpOllama__*` ou git que escreva. Nenhum valor de credencial foi lido: de `.cursor/mcp.json`, `.vscode/mcp.json` e `.mcp.json` só contei ocorrências da string `--api-key` (`grep -c`).

**Segredos no texto do analista:** varredura (sequências longas, `Bearer`, `--api-key` com valor): **0 ocorrências**. Os hashes citados são de binário (`d61f5e82…`) e de commit, e não são segredo. Nada foi redigido.

### Veredito por achado

| Achado | Veredito | Evidência conferida | Ajuste do reparo |
| --- | --- | --- | --- |
| F-1 | **CONFIRMADO** | `guard.go:120,200,340` (`runtime.GOOS`), `:207` (`filepath.Dir(r) == r`), comentário `:224-226` e `strings.Split(..., os.PathListSeparator)` em `:229`. Arquivos de plataforma fora de gateway/cmd: só os pares citados, mais testes (`ls */*_windows*.go */*_other*.go`). `hasReparseDir` (`guard.go:513`) não ramifica por SO. | A explicação "o uso está em 229" **não** entra no corpo da entrada 22:45 (moldura, regra 4). Ali vai só `**[errata E-n]**` logo após "`guard.go:223`"; o texto fica na §9.1. Na §2 item 1 (fora do changelog) a correção pode ser direta, com errata. |
| F-2 | **CONFIRMADO** (a frase do plano é falsa) | `NewGuard` não recusa `/`; `BroadRoots` só marca (`guard.go:184-221`); WARNING em `server.go:180-182`; `DefaultBroadDirs` só lê variáveis do Windows (`guard.go:170-179`). | A citação da decisão está imprecisa. O `report.md:119-127` da Fase 4 só lista as opções; a decisão "SEC-M-02 = aceitar com aviso" está em `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md:517` (entrada de 2026-10-23 22:10) e no comentário de `guard.go:181-183`. Usar esses dois. |
| F-3 | **CONFIRMADO, com uma precisão** | `policy.go:105` compara `.mcp.json` por igualdade. Reexecutado: `Denied(".mcp.json.bak-windows-20261112")=false`; `ReadContext` lê por caminho absoluto **e por relativo** (raiz única), 41 bytes falsos. O `.bak` foi criado às **22:02:11** reais (`evidence-exec.txt` bloco #21, linhas 639-647; saída `-rw------- root 1523 … .mcp.json.bak-windows-20261112`). O MCP roda como root e o arquivo é `0600 root`, portanto legível. `docs/INSTALACAO.md:764` e `D:\MCP\Documentation\Ollama\03-parametros.md:51` dizem ao usuário que `.mcp.json` é recusado, o que induz a achar que o backup também está protegido. | **Precisão:** o texto que **volta ao Claude** passa por `redact` (`server.go:441-456`, `cfg.secrets()` = `--api-key`/`OLLAMA_API_KEY`, `config.go:458-470`). Se o token do `.bak` for o mesmo em uso (o changelog 22:15 diz "o token do Ollama será o mesmo"), ele chega mascarado ao contexto do cliente. O **prompt** enviado ao Ollama/gateway **não** é redigido (`redact` só se aplica a log, erro e resultado: `ollama/errors.go:91`, `server.go:448-456`), então o token vai ao modelo e ao `content.db`. Trocar "o token iria ao modelo, possivelmente na resposta e no `content.db`" por "o token vai no prompt ao gateway/modelo e é gravado no `content.db` (prompt e resposta, 7 dias); o resultado devolvido ao Claude sai mascarado só se o token for idêntico ao configurado". **Moldura:** o item 3 do reparo ("marca superado no changelog 22:20") viola a regra 4: no corpo da entrada 22:20 só entra `**[errata E-n]**`, e o texto vai para a §9.1 e para o risco novo da §7. A gravidade **crítica** se mantém. |
| F-4 | **CONFIRMADO, com correção de contexto** | Reexecutado: `Denied` falso para `.bashrc`, `.claude.json`, `.mcp.json.bak`. | A lista **não** ficou "como decisão pendente do owner". Em 2026-10-23 22:10 o owner **decidiu** "SEC-M-03 = manter a denylist como está […] as lacunas ficam **aceitas pelo owner** e documentadas" (`…implementacao.plan.md:518`). O item 1 de "Fora do escopo" (ampliar a lista) **reabre uma decisão explícita do owner** e só pode entrar como questão nova, citando essa decisão, nunca como recomendação de fato. Em F-4, trocar "Por isso a raiz **nunca** deve ser `/root`" por "recomendação: raiz nunca `/root`; a decisão é da questão 9". |
| F-5 | **CONFIRMADO** | `guard_test.go:122-130`, `Fatalf` em `:125`; FAIL no Linux em `evidence-exec.txt:1029,1040`. | A frase "`TestSplitRoots` com `os.PathListSeparator`" está no **corpo** da entrada 22:45 C(1), não na célula da S2. Não se troca: entra `**[errata E-n]**` no changelog, e a orientação (literais por plataforma + separador errado + mutação) é **acrescentada** à célula S2. A proposta técnica do analista é correta e não é tautológica. |
| F-6 | **CONFIRMADO** | `ocr_test.go:164-178` (exige `SHA-256` em `:173`); `ocr.go:103-112` (`ocrConfig` antes de `newPoolFn`); `ocr/pool.go:64-66`; `ocr/ocr.go:81`; FAIL no Linux em `evidence-exec.txt:1032,1046,1080`. A ordem das validações (`ocr.go:37` "must be an absolute path" antes da plataforma) confere. | Mesmo ajuste de moldura do F-5: "`t.Skip` fora do Windows ou outra expectativa" é corpo da entrada 22:45. Recebe só o marcador; a orientação "não usar `t.Skip`, ramificar a expectativa" é acrescentada à S2 com `**superado em 2026-11-13 (revisão):**`. Nota: `TestProbeFailureTurnsOCROff` já usa `t.Skip` (`ocr_test.go:184-186`). O argumento do analista continua válido, porque esse teste exercita o worker real, que não existe fora do Windows, e o `TestWrongHashServerStarts` exercita a partida, que existe. |
| F-7 | **CONFIRMADO** | `config_test.go:193-197`, `tools_wiring_test.go:409`, `tools/registry_test.go:169`, `ollama/security_test.go:216-218`. A execução no Linux copiou `TOOLS.md` (`evidence-exec.txt:1004,1013`) e depois `src/mcp` (`:1077`), e o terceiro FAIL sumiu (`:1080`). | Texto da errata vai para a §9.1, não para o corpo da entrada 22:45. Os 7 pacotes com testes do binário (`doctext, files, mcpollama-mcp, ocr, ollama, redact, tools`; `mcp` não tem testes) **foram todos rodados** no Linux (`evidence-exec.txt:1021-1027`). O "parcialmente" de 3.B se refere só a `-race`, `gateway/*` e `cmd/*`. |
| F-8 | **CONFIRMADO, com ampliação** | `config.go:144-145`: a linha 145 diz "(: on Linux)", em `9ef7935` 2026-10-23 22:08 (`git blame`); `config.go:59` cita só `;`. | **O analista errou em parte:** "a documentação confirma só `;`" não vale para toda a documentação. `D:\MCP\Documentation\Ollama\03-parametros.md:47` já diz "separadas por `;` (`:` no Linux)". Só `02-mcp-json-do-zero.md:14,24` e `docs/INSTALACAO.md:764` citam apenas `;`. Corrigir a errata E-n e a S4b nesse ponto. Marcador só no corpo da 22:45; o texto vai para a §9.1. |
| F-9 | **CONFIRMADO** | `tools/allowlist.go:15`; `config.go:134` ("separated by ; or ,"); `03-parametros.md:29`. | Nenhum. |
| F-10 | **CONFIRMADO** | `tools/registry.go:99-109` (comentário `:105`); `tools/filetools.go:56-58`; 8 tools no `tools/list` do changelog 21:55. | O §1 item 3 não é changelog: a correção pode ser direta, com E-n. |
| F-11 | **CONFIRMADO e reforçado** | `go.mod` (diretas e `// indirect` conferem); `go list -deps ./mcpollama-mcp` (GOOS=linux) inclui `pdfium_wasm` e `implementation_webassembly` e não inclui `modernc.org/*` nem `x/image`; `x/image` só em `*/ocr_real_test.go`. | Prova estática mais forte: o módulo tem `internal/pdfium_wasm/embed.go:11` `//go:embed pdfium.wasm` (5.828.543 bytes, em `%GOMODCACHE%\github.com\klippa-app\go-pdfium@v1.21.4`). Acrescentar à marca "superado" da §2.1, linha 47. |
| F-12 | **CONFIRMADO** | `CHANGELOG.md:53` (`## Não lançado`, seção **global** do repositório, hoje com notas das Fases 11 e 12), `:73` (0.10.3), `:81` ("não executado em Linux"); tags até `mcpollama-v0.10.3` → `b1faec3ecacb`; `git diff --stat b1faec3 HEAD` nos pacotes do MCP e `go.mod/go.sum`: vazio; em `src/` só `gateway/monitor/*`. | Nenhum de mérito. Precisão: "Não lançado" não é só do mcpollama. A nota da Fase 15 vai como parágrafo próprio, sem versão. |
| F-13 | **CONFIRMADO** | `CLAUDE.md:84` (HEAD e árvore), `README.md:314`, `AGENTS.md:84`; `go version -m D:\MCP\mcpollama-mcp`: `-ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `vcs.revision=4ae3a87…`, sem `-trimpath`; `.exe` com `vcs.revision=b1faec3…`; `sha256` começa com `d61f5e82` e termina com `40bd0333`. | A questão nova (script de build) está correta como questão. Precisão: o §2 item 2 diz "gerado por `GOOS=linux go build` (CLAUDE.md)", mas o binário tem `-ldflags` que o comando do `CLAUDE.md:84` não tem. Logo, não foi gerado pelo comando documentado, e isso vai na errata. |
| F-14 | **CONFIRMADO** | `ls -la /d/MCP`, `Documentation`, `Documentation/Ollama` (14 arquivos, 6 `.bak-<data>`), `cache/pdfium/wazero-v1.12.0-amd64-windows`: idênticos à tabela do analista. | Nenhum. |
| F-15 | **CONFIRMADO** | `git ls-files` com os nomes dos 7 documentos: vazio (rc=1); `git ls-files docs`: 5 arquivos, nenhum de cliente Linux. | Nenhum. A questão 7 continua do owner. |
| F-16 | **CONFIRMADO, com ampliação** | Fase 12 §11 na `:515`, 11.1 na `:517`, "Push e tags só com autorização" na `:535` (não ~539); Fase 14 existe em `plans/concluidos/` (árvore) e ainda está rastreada no caminho antigo (`git ls-files`); relatório da Fase 4 e `.testes/` existem; `Mkfifo`: 0 ocorrências. | Os testes de ataque da Fase 4 **não** são todos Windows-only: têm ramos por `runtime.GOOS` (`files_attack_test.go:68,274,503,577`) e `cmd /c mklink` (`:244`), mas nunca rodaram em Linux. "Rodá-los no Linux" exige adaptar, além do FIFO, os casos de symlink/hardlink. Para tocar em `src/doctext`, o `CLAUDE.md` exige também os testes da revisão da Fase 7, que o plano não cita (só reportar). |
| F-17 | **CONFIRMADO** | `ocr/ocr_other.go:15,21`; `ocr/ocr.go:81`; `mcpollama-mcp/ocr.go:108-110`; `main.go:61-70` (`syncWriter` sobre `stderr`). | Nenhum. |
| F-18 | **CONFIRMADO** (repositório); servidor **NÃO VERIFICÁVEL SEM O SERVIDOR** | `nvidia-powercap.service:2,14-17` (+1500/+150), último commit `c4fa349` 2026-11-08; `CLAUDE.md:186` igual no HEAD e na árvore. | Nenhum. |
| F-19 | **CONFIRMADO** | Reexecutado às 11:5x: vet rc=0; `ok files 1.222s`, `ok mcpollama-mcp 6.306s`. | Nenhum. |
| F-20 | **CONFIRMADO** | `station.go:147,159,167,188`; `config.go:518-533` (`defaultLabel`, cai em `<host>/<pid>`). | Nenhum. |

**Totais:** 20 achados; **20 confirmados** (F-2 confirma que a frase do plano é falsa; F-8 confirma a refutação parcial e a amplia); **0 refutados**; o F-18, no que diz respeito ao servidor, e o estado atual do `.bak` (F-3) são **não verificáveis sem o servidor**. O analista errou dois detalhes, que viraram ajustes: a fonte da decisão SEC-M-02 e "documentação só `;`". Os reparos de F-1, F-3(3), F-5, F-6, F-7 e F-8 propunham **texto no corpo de entradas antigas do changelog**, o que contraria a regra 4 da moldura. Corrigido acima: só o marcador `**[errata E-n]**` vai no corpo; o texto vai para a §9.1 e para as seções vivas (§2, §7, S2, S4b).

### Achados novos (F-V-n)

#### F-V-1 O procedimento da S4b cria novas cópias do token **dentro** do `--allow-root` que a lista fixa não cobre
- **Onde no plano:** S4b, linha 119: "**backup `.bak-<data>` de cada arquivo tocado**", com alvo "arquivos de configuração e instrução (`.cursor`, `.claude`, `.agents`, comandos, `.mcp.json`)" em `/root/treino`. A S5 (linha 120) e a S7 (linha 122) repetem a convenção `.bak-…`.
- **Evidência:** executado (`files.Denied`, scratchpad): `Denied(".mcp.json.bak-20261113")=false`, `Denied(".claude/settings.local.json.bak-20261113")=false` e `Denied("mcpollama.env.bak-20261113")=false`. O sufixo `.env` deixa de ser o fim do nome, por isso o backup do arquivo de ambiente **escapa** da regra `HasSuffix(".env")` (`policy.go:107`). O `.mcp.json` de `/root/treino` contém o token em `env.OLLAMA_API_KEY` (`evidence-exec.txt` bloco #21, saída mascarada). Seguir a S4b ao pé da letra repete o F-3.
- **Classificação:** confirmado (código + execução com nomes; nada foi criado no servidor).
- **Gravidade:** **crítico** (mesma classe do F-3, e o plano manda fazer).
- **Reparo proposto:** acrescentar à célula S4b, com marca `**superado em 2026-11-13 (revisão):**`: "backup de arquivo com segredo (`.mcp.json`, `mcpollama.env`, `settings.local.json`) **nunca** dentro de uma raiz do `--allow-root` nem com sufixo após o nome (a lista fixa compara `.mcp.json` por igualdade e `.env` por sufixo, `files/policy.go:105-107`). Guardar fora da raiz (ex.: `/root/MCP/backup/`, `0700`); não confiar em renomear para caber na lista fixa. Gate da S4b: `find /root/treino -name '*.bak*'` sem arquivo de segredo." A escolha do local é de execução, não decisão nova. Vale também para a S5 (`mcpollama.env`).

#### F-V-2 `mcp.json` sem ponto (`.cursor/mcp.json`, `.vscode/mcp.json`) não é negado, e no repositório Windows os dois têm `--api-key`
- **Onde no plano:** S4b(b), linha 119 (`.cursor` está entre os alvos em `/root/treino`); §7 "Rodar como root".
- **Evidência:** `Denied(".cursor/mcp.json")=false` e `Denied(".vscode/mcp.json")=false` (executado). Em `E:\MCPOllam`, `.cursor/mcp.json` (1.013 B) e `.vscode/mcp.json` (1.025 B) existem, não são rastreados pelo git e têm **1 ocorrência de `--api-key`** cada (`grep -c`; valor **não** lido). O `.mcp.json` deste projeto usa `--allow-root E:\MCPOllam` (índice, linha 32). Se o argumento for o token real (o `CLAUDE.md` diz que os clientes Cursor e VS Code "estão configurados" com o token como `--api-key`), `summarize_file` de `E:\MCPOllam\.cursor\mcp.json` hoje o envia ao modelo e ao `content.db`. Não testado: chamar a tool real é proibido nesta revisão.
- **Classificação:** confirmado no código e na existência dos arquivos; o valor ser o token real **não foi verificado** (regra: não ler credencial).
- **Gravidade:** importante para o plano (em `/root/treino` só se houver `.cursor/mcp.json` com token: não verificável sem o servidor); a exposição no Windows é **fora do escopo** (reportar).
- **Reparo proposto:** na S4b(b), acrescentar: "inventariar em `/root/treino` todo `mcp.json`, `*.json` de cliente MCP e `*.bak*` que contenham `api-key`/`OLLAMA_API_KEY` (contagem por `grep -c`, sem imprimir valor); nenhum deles pode ficar dentro da raiz com nome fora da lista fixa". Nova linha em §7 (junto com a do F-3): "a lista fixa só nega `.mcp.json` (igualdade); `mcp.json` do Cursor/VS Code e qualquer backup com sufixo são lidos".

#### F-V-3 O token passou por `argv` no servidor ao montar o `.mcp.json` (22:02:11), o que contradiz a motivação da D15-6
- **Onde no plano:** changelog 22:20 ("token […] copiado de `/root/MCP/mcpollama.env` dentro do servidor, sem imprimir"); D15-6 (linha 105: "para não aparecer em `ps`").
- **Evidência:** `evidence-exec.txt:642` `python3 - "$TOK" <<EOF` e `:648` `sed "s/$TOK/<TOKEN>/" .mcp.json`: o valor foi argumento de processo (`/proc/<pid>/cmdline`) durante a execução. Ele não foi impresso, como o plano diz, mas ficou visível em `ps` por um instante.
- **Classificação:** confirmado (comando real registrado).
- **Gravidade:** informativo (servidor root, janela de milissegundos; entra na etapa 04).
- **Reparo proposto:** errata **E-n**: marcador no corpo da entrada 22:20 após "sem imprimir" e, na §9.1, "o valor foi passado como argumento a `python3` e `sed` (`evidence-exec.txt:642,648`): não impresso, mas visível em `ps` durante a execução". Na S4/S4b (documentação Linux), regra: "token só por stdin ou arquivo, nunca em `argv`".

#### F-V-4 A decisão do separador do `--allow-root` foi registrada como "`;`" e o `:` no Linux veio no commit seguinte
- **Evidência:** `…implementacao.plan.md:519` (decisão de 2026-10-23 22:10: "separador `;` (aceita vírgula em nome de pasta)"); `9ef7935` (22:08:30, `guard.go:224-229` e `config.go:145`) implementou `os.PathListSeparator` e documentou "(: on Linux)". O código e o `--help` estão coerentes entre si; a decisão escrita no plano de implementação é que fala só em `;`.
- **Classificação:** confirmado.
- **Gravidade:** informativo (não muda a S2; o teste novo do F-5 fixa `:` no Linux, e isso é coerente com o código e o `--help`).
- **Reparo proposto:** em S2, ao corrigir o `TestSplitRoots`, citar `9ef7935` e `config.go:145` como base do `:` no Linux. Se o owner preferir `;` em todos os SO, isso é mudança de código e vira questão nova. Não proponho a mudança.

#### F-V-5 Linha errada no F-16
- `…fase12-erros-treino.plan.md`: "Push e tags só com autorização do owner" está na linha **535**, não "~539". Informativo.

### Reparos: conformidade com a moldura do índice

- **Conformes:** F-2, F-4 (com o ajuste acima), F-9, F-10, F-11, F-12, F-13, F-14, F-15, F-16, F-17, F-18, F-19, F-20 e as questões novas de F-3 e F-13 (opções, recomendada primeiro, impacto, rollback, não fazer).
- **Não conformes, corrigidos nesta verificação:** F-1, F-3 item 3, F-5, F-6, F-7 e F-8 punham texto no corpo de entradas antigas (regra 4). Regra para a etapa 06: no corpo vai **só** `**[errata E-n]**`; o texto vai para a §9.1 (E-n) e, quando houver ação, é **acrescentado** à célula S2/S4b ou à §7, com `**superado em 2026-11-13 (revisão):**`.
- **Risco de decisão tomada pela revisão:** a ampliação da lista fixa (F-4 e item 1 de "Fora do escopo") reabre a decisão SEC-M-03 do owner (2026-10-23 22:10). Isso só entra como questão.

### Perguntas fechadas para o owner (geradas por esta etapa; numeração final na etapa 06, a partir da 13)

1. **`.bak` com token em `/root/treino` (F-3):** (a) mover para fora da raiz (`/root/MCP/backup/`, `0700`) (Recomendado); (b) apagar (o token está em `mcpollama.env` e no `.mcp.json`); (c) manter, aceitando que `summarize_file`/`extract`/`translate` o enviem ao modelo e ao `content.db`. Rollback de (a): `mv` de volta. Impacto: nenhum serviço lê o arquivo.
2. **Backups da S4b/S5 (F-V-1):** autorizar a regra "backup de arquivo com segredo sempre fora de qualquer raiz do `--allow-root`" como parte do procedimento? (Recomendado: sim.) Alternativa: backups ao lado do original, o que repete o F-3.
3. **`mcp.json` do Cursor/VS Code com `--api-key` dentro de `E:\MCPOllam` (F-V-2, fora do escopo da Fase 15):** (a) só registrar como achado e decidir depois (Recomendado nesta revisão); (b) abrir item próprio para tirar o token desses arquivos (passar a `env`/`OLLAMA_API_KEY`) ou retirar `E:\MCPOllam` do `--allow-root`; (c) aceitar o risco.
4. **Lista fixa (F-4, reabre a SEC-M-03 de 2026-10-23 22:10):** (a) manter a decisão de 2026-10-23 e mitigar só por raiz estreita e backups fora da raiz (Recomendado para esta fase); (b) abrir mudança em `src/files/policy.go` (ex.: `.mcp.json*`, `mcp.json`, `*.env.*`, `.bashrc`, `.claude.json`), com os testes de ataque da Fase 4 de novo e versão nova (afeta D15-1).
5. **Script de build (F-13):** (a) comando documentado e manual na §4.2/`CLAUDE.md` (Recomendado, sem código novo); (b) `tools/Build-Release.ps1` no repositório, com `SHA256SUMS` (arquivo novo, MCPO-REQ-BUILD-004).
6. **Fonte única da documentação (F-15, questão 7 existente):** confirmar se a importação dos 7 documentos + README de `D:\MCP\Documentation\Ollama` para `docs/` entra nesta fase (passo novo antes da S4) ou se a §4.4 volta a ser só proposta.

### Não verificado nesta verificação

- Estado do servidor: se o `.bak-windows-20261112` ainda existe, se há `.cursor/mcp.json` em `/root/treino`, offsets 0/0 da `nvidia-powercap`. A revisão não acessou o servidor.
- Se o argumento `--api-key` de `.cursor/mcp.json`/`.vscode/mcp.json` é o token real (valor não lido por regra).
- Gravação efetiva do prompt no `content.db`: aceito pelo `CLAUDE.md` (Fase 10.L); o código de `src/gateway/` não foi relido nesta etapa.
- `Guard.ReadContext` em Linux: executado só no Windows; a política (`Denied`) não depende do SO.
