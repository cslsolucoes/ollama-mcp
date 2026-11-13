# Etapa 05 — Etapas, gates, rollback e Definição de Pronto (revisão do plano da Fase 15)

**Gravado:** 2026-11-13, entre 11:50 e 12:10 (hora real: `date` às 11:50:43 durante a análise; o arquivo foi gravado logo depois) · **Analista:** Claude Code (Sonnet 5.5) · **Prefixo dos achados:** D-n
**Alvo:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (177 linhas, igual ao HEAD `7fbe5cc`; não editado por esta etapa).
**Acesso a servidor nesta etapa:** nenhum. Todo fato sobre o `lx-ai01` vem das saídas gravadas em 2026-11-12 e **não foi reverificado em 2026-11-13**.

Siglas: **EV** = `recuperado/evidence-exec.txt`; **TL** = `recuperado/timeline.txt`; **AC** = `recuperado/achados-complemento.md`; **PLANO** = o arquivo acima. Linhas de EV/TL citadas como `EV:n` / `TL:n`.

## Fontes lidas

- `E:\MCPOllam\.workspace\reports\MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas\00-indice-e-metodo.md` (inteiro)
- `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (inteiro, 177 linhas)
- `D:\Users\claiton.linhares\AppData\Local\Temp\claude\e--MCPOllam\6bc04315-e87d-4573-bdfc-8bea884542c3\scratchpad\recuperado\timeline.txt` (inteiro)
- `…\recuperado\achados-complemento.md` (inteiro)
- `…\recuperado\evidence-exec.txt` (inteiro, 1.430 linhas; as 56 saídas)
- Código e documentos do repositório lidos: `src/mcpollama-mcp/config.go` (linhas 120-170, 500-540), `src/mcpollama-mcp/main.go:21-22`, `src/mcpollama-mcp/ocr_test.go:140-190`, `src/mcpollama-mcp/config_test.go:194`, `src/mcpollama-mcp/tools_wiring_test.go:405-414`, `src/tools/registry_test.go:169`, `src/files/guard_test.go:112-130,265-266`, `src/files/policy.go:70-144`, `src/doctext/doctext.go:212-235`, `src/ocr/ocr.go:81`, `src/gateway/queue/queue.go:1130-1131`, `src/gateway/store/store.go:654-688`
- `E:\MCPOllam\CLAUDE.md` (linhas 76-91; `git diff` pendente), `E:\MCPOllam\CHANGELOG.md` (linhas 1-90), `E:\MCPOllam\docs\RELATAR-ERROS-IA.md` (inteiro), `E:\MCPOllam\TOOLS.md` (seção `reportar_erro`), `E:\MCPOllam\docs\CONTRATO-DADOS-TREINO.md` (grep), `E:\MCPOllam\deploy\gateway\Instalar-Gateway.ps1` (55-90)
- `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (§11, linhas 515-596) e `…\plans\concluidos\MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` (grep)
- `E:\MCPOllam\referencias\Brief_Referencias\60-Requisitos_mcpollama.md:272` (MCPO-REQ-BUILD-004), `E:\MCPOllam\.cursor\rules\workspace-plans-persist_V1.4.0.mdc` (nomes das seções)
- `D:\MCP` (listagem, hash do `mcpollama-mcp`) e `D:\MCP\Documentation\Ollama\*.md` (contagens)
- Medições locais desta etapa (somente leitura, sem gravar no repositório nem em `D:\MCP`): `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` em `src`, `go list ./...`, `find src -name testdata`, `ls src/mcp/*_test.go`, `sha256sum D:\MCP\mcpollama-mcp`.

## Achados

### D-1 — Estado real de cada etapa (a tabela da §6 não tem a coluna "Estado")

- **Onde no plano:** §6, linhas 112-123 (tabela sem estado); banner da linha 13 ("Nada foi executado"); changelog, linhas 157-177.
- **O que o plano diz:** a §6 apresenta S0 a S8 como futuras ("Quem / Gate"); o texto "ao fim de cada uma este plano é atualizado" (linha 110) não foi cumprido por etapa, só em prosa no changelog.
- **O que a evidência mostra:** a linha do tempo e as saídas gravadas (47 invocações de `ssh` em 69 minutos, 21:51 a 23:00, `grep -c "ssh -o BatchMode" EV`) mostram que S1(a), S1(b), parte de S1(c), S4 (só o `mcp.json.exemplo`), S4b (só o `.mcp.json` de `/root/treino`), S5 e boa parte da S6 foram executadas **fora de ordem**, por ordens diretas do owner (`TL:10`, 21:50:56; `TL:29`, 21:58:32; `TL:42`, 22:01:51; `TL:58`, 22:14:26; `TL:86`, 22:27:38). A aprovação formal do plano (S0) não existe: o owner disse "ok, 1 e 2, sem execução do plano" (`TL:19`, 21:54:48) e o frontmatter segue `status: draft`.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a nova coluna "Estado" e para a nota abaixo da tabela, sem apagar o texto das células):**

  Nota abaixo do título da §6: `**Estado lido em 2026-11-12 23:00 (última leitura no servidor) e conferido contra os arquivos em 2026-11-13; o servidor não foi reverificado na revisão de 2026-11-13.**`

  | Etapa | Estado (coluna nova) |
  | --- | --- |
  | **S0** | **parcial.** Respondidas **de fato** (sem a pergunta formal): questão 1 (máquina = `lx-ai01`, `TL:26`, 21:57:20 "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1"), questão 8 (token em `OLLAMA_API_KEY`, `TL:19`, 21:54:48 "ok, 1 e 2", lido como a 1ª opção) e questão 10 (URL pública + `--tls-ca`, mesma resposta). Questão 2 respondida pelo pedido original (`TL:2`, 21:44:20); questões 9 (root), 11 (execução por Claude/sonnet) e 12 (commits direto no `master` com push, `TL:96`, 22:32:37) decididas **por ação**, a 12 contra a opção recomendada. **Aprovação formal do plano: não existe** (ver D-15). |
  | **S1** | **parcial, fora de ordem.** (a) feita 21:51:10 (EV:288-299: `ia.cslsolucoes.com.br`, x86_64, CentOS Stream 10, glibc 2.39, 88 GB livres, `/root/MCP` não existia). (b) feita 21:51:20-21:51:33 **dentro de `/root/MCP`, não em diretório temporário** como o gate pedia (EV:301-395). (c) **parcial**: 7 de 25 pacotes executados como binários de teste compilados no Windows (EV:980-1033), 2 FAIL conhecidos; `go vet` no Linux e `-race` não feitos na época (o `go vet` Linux foi medido em 2026-11-13, ver D-3). (d) saídas registradas só em prosa; tabela comando → código de saída → resultado reconstruída em 6.1. |
  | **S2** | **pendente.** Nenhum código alterado (`git log b1faec3..HEAD` sem mudança em `src/mcpollama-mcp`, `files`, `tools`, `ollama`, `mcp`, `doctext`, `redact`, `ocr`). Escopo conhecido: 2 testes + o texto do `--help` (ver D-4). |
  | **S3** | **pendente.** O binário em uso foi **copiado** de `D:\MCP\mcpollama-mcp` (`GOOS=linux CGO_ENABLED=0`, `-ldflags=-X main.Version=0.10.3-b1faec3`, `vcs.modified=false`, **sem `-trimpath` e sem `-s -w`**, EV:304-314), não gerado pelo comando da §4.2. Não existe `SHA256SUMS` (só `VERSION` com o hash). Hash de `D:\MCP\mcpollama-mcp` reconferido em 2026-11-13 11:50: `d61f5e82…0333`, igual ao do servidor em 2026-11-12 (EV:316). |
  | **S4** | **pendente, com um item feito fora de ordem:** `mcp.json.exemplo` sem token (EV:507-525). Documentação Linux **não feita**: `/root/MCP/Documentation/Ollama` é cópia byte a byte da Windows (EV:305, 319-338), ver D-7/D-8. Varredura de segredos não feita. |
  | **S4b** | **pendente, com um item feito fora de ordem:** o `.mcp.json` de `/root/treino` foi refeito para Linux às 22:02:11 (EV:635-674), antes do pedido de S4b (`TL:48`, 22:03:14). O backup ficou como `.mcp.json.bak-windows-20261112`, **fora da convenção `.bak-<data>` da própria S4b** e **dentro do `--allow-root`** (ver D-9). Nenhum outro arquivo de `/root/treino` alterado. |
  | **S5** | **concluída, fora de ordem.** Feita 21:51 por ordem do owner (`TL:10`, 21:50:56: "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP"), antes de S1(c), S2, S3 e S4, o que contraria D15-5 (linha 104) **com autorização expressa do owner**. Sem backup prévio porque não havia binário anterior (EV:299 "/root/MCP nao existe"). Árvore da §4.1 criada (EV:384-396, 527-537); extras não previstos: `certs/` e `mcpollama.env`. Hash local = remoto (EV:307, 316). |
  | **S6** | **parcial.** Feito: `list_models` pelo Claude Code 2.1.321 **com `--mcp-config --strict-mcp-config` e `env -u CLAUDE_CODE_OAUTH_TOKEN`** (EV:1151-1154, 22:25:47, rc=0); por stdio: `summarize_file` de `.md` na raiz (EV:707-714), 2 recusas fora da raiz (EV:951-954), `embed` 768 dim (EV:955-958), `extract` de PDF com texto (EV:959-972), `generate` x10 com `seed` (EV:1275-1287, 10/10 ok). **Não feito:** `chat`, `translate`, `reportar_erro`, `.docx`/`.pptx`, recusa de PDF digitalizado e de imagem, rótulo no `/monitor`, aprovação interativa do servidor MCP (segue "Pending approval", EV:1385), chamada normal sem `--mcp-config`. Itens e gates em D-10/D-11. |
  | **S7** | **pendente.** Ver D-17. |
  | **S8** | **pendente e condicional** (só existe se a S2 gerar versão). |

  Commits que tocaram o plano: `f6bc6ab` (2026-11-12 21:53:31) e `7fbe5cc` (22:32:51, push 22:32:54). **Nenhum commit de código.**

### D-2 — Gate da S1(c) impreciso ("`go test` num Linux" sem procedimento nem critério)

- **Onde no plano:** S1(c), linha 115; risco da linha 130; changelog 22:45 (linha 163).
- **O que o plano diz:** "rodar `go test -count=1 ./...` e `go vet ./...` num Linux (no alvo, se houver Go, ou num contêiner/WSL/outra máquina Linux de teste)"; gate "lista de falhas do `go test` no Linux (se houver)".
- **O que a evidência mostra:**
  1. O servidor **não tem Go** (EV:797-798, `command -v go` vazio); o método real foi compilar os binários de teste no Windows e executá-los no servidor (EV:980-1033). O gate da §6 nunca descreveu esse método.
  2. **Não existe nenhum diretório `testdata`** em `src` (`find src -type d -name testdata` vazio, 2026-11-13). As dependências de arquivo dos testes são: `../../TOOLS.md` (`src/tools/registry_test.go:169` e `src/mcpollama-mcp/tools_wiring_test.go:409`), `../mcp/types.go` (`src/mcpollama-mcp/config_test.go:194`) e os `.go` do próprio `mcpollama-mcp` (`config.go`, `server.go`, `main.go`, `logger.go`, mesma linha). A falha "terceiro FAIL" de 22:17 (`TestNoLiteralContextLimits`, EV:1044-1045) foi exatamente `../mcp/types.go` ausente; o plano chama isso de "erro meu do envio" sem dizer a regra para não repetir.
  3. `src/mcpollama-mcp` contém um `mcpollama-mcp.exe` de build local (ignorado pelo git, `.gitignore:2 *.exe`), de ~23 MB, que o `tar` da época também levou (EV:1066).
  4. Os testes rodaram como **root** (sessão `lx-ai01-root`). Testes de guard que dependem de permissão (arquivo ilegível, diretório sem execução) se comportam diferente como root: o "verde" como root é mais fraco que o verde como usuário comum. Não verificado se há testes assim; recomendar 1 repetição de `files` e `tools` como usuário sem privilégio.
  5. Pacotes: `go list ./...` = **25**. Executados no Linux em 2026-11-12: **7** (`tools`, `files`, `doctext`, `ollama`, `redact`, `mcpollama-mcp`, `ocr`). Dos **18** restantes, `mcp` **não tem arquivos de teste** (`ls src/mcp/*_test.go` falha, 2026-11-13); os outros 17 são `cmd/bakeoff`, `cmd/dumptools`, `cmd/mcpollama-gateway` e os 14 `gateway/*` (`content`, `cpu`, `gpu`, `hmackey`, `joblog`, `melhora`, `melhora/feed`, `models`, `monitor`, `pool`, `proxy`, `queue`, `store`, `treino`). O AC (item 7) e o pedido desta etapa falam em "18 não testados"; a leitura correta é "1 sem testes + 17 não executados".
  6. `-race` precisa de cgo e do gcc de `E:\MCPOllam\deps\gcc-mingw-w64\bin` (CLAUDE.md:89-90): só roda no Windows e **não cobre** os arquivos `_other`/`!windows` (`files/nlink_other.go`, `ocr/ocr_other.go`, `station_wmi_other.go`).
  7. Testes com tag (`live`, `stress`, `ocr && windows`) não entram no `go test` sem `-tags`; não faz parte do gate.
- **Classificação:** confirmado (itens 1-3, 5 por leitura de EV e do código; item 4 é hipótese a verificar; item 6 por CLAUDE.md).
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a S1(c) e para a nova subseção 6.1):**

  > **S1(c), procedimento exato (um `ssh` para copiar, um para executar, um para limpar):**
  > 1. *Estação (bash ou PowerShell com os mesmos comandos), a partir de `E:\MCPOllam\src`, sem gravar no repositório:* para cada `p` em `tools files doctext ollama redact mcpollama-mcp ocr`: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o <scratch>/lt/<p>.test ./<p>`. Registrar qualquer `COMPILE-FAIL`.
  > 2. *Empacotar:* `tar cf - --exclude='*.exe' -C src tools files doctext ollama redact mcpollama-mcp ocr mcp -C .. TOOLS.md` mais os `<p>.test`. Os testes leem `../../TOOLS.md`, `../mcp/types.go` e os `.go` do próprio pacote; **não há `testdata`**. Layout no destino: `/tmp/fase15-lt/src/<p>/`, `/tmp/fase15-lt/src/mcp/`, `/tmp/fase15-lt/TOOLS.md`, `/tmp/fase15-lt/<p>.test`.
  > 3. *Executar no alvo:* `cd /tmp/fase15-lt/src/<p> && ../../<p>.test -test.v -test.count=1 -test.timeout=10m > /tmp/fase15-lt/<p>.out 2>&1; echo "rc=$?"`. Resumir por pacote: `rc`, tempo, `grep -c '^--- PASS'`, `grep -c '^--- FAIL'`, `grep -c '^--- SKIP'`. **Listar os `SKIP`** (cada skip no Linux é cobertura perdida; ex.: `TestProbeFailureTurnsOCROff`, `ocr_test.go:184`).
  > 4. *Limpar:* `rm -rf /tmp/fase15-lt` e provar com `ls /tmp/fase15-lt` (deve falhar).
  > 5. *Repetir `files` e `tools` como usuário sem privilégio* (`runuser -u <usuário> -- …` ou o alias `lx-ai01`) e registrar a diferença, se houver.
  > 6. **Pacotes cobertos:** os 7 acima. **Não executados no Linux:** `mcp` (sem testes), `cmd/bakeoff`, `cmd/dumptools`, `cmd/mcpollama-gateway`, `gateway/*` (14): fora do escopo do MCP; a **compilação** deles para Linux é coberta pelo `go vet` do gate D-3.
  > 7. **`-race`:** só no Windows (gcc de `deps\`); **não cobre** os arquivos `!windows`. Limitação aceita e registrada, não "verde".
  >
  > **Gate S1(c):** para cada um dos 7 pacotes, `rc=0` e 0 `--- FAIL`, **salvo, até a S2, exatamente estes 2**: `files/TestSplitRoots` e `mcpollama-mcp/TestWrongHashServerStarts`. Qualquer outro FAIL bloqueia a S2/S3. **Gate S2/S3:** 0 FAIL absoluto nos 7 pacotes e `SKIP` listados.

  Tabela 6.1 a incluir (resultado de 2026-11-12 22:17-22:18, EV:1016-1082; **não reverificado em 2026-11-13**; HEAD de código = `b1faec3`, sem mudança de código depois):

  | Pacote | Resultado (Linux, root, `-test.count=1`) | Nota |
  | --- | --- | --- |
  | `redact` | rc=0, 0 s, PASS | |
  | `files` | rc=1, 0 s, **FAIL** `TestSplitRoots` (`guard_test.go:125`: obteve `["a ; ;b;; c"]`) | teste assume `;`; no Linux o separador é `:` (`guard.go:223`, `os.PathListSeparator`) |
  | `ollama` | rc=0, 1 s, PASS | |
  | `ocr` | rc=0, 0 s, PASS | |
  | `doctext` | rc=0, 16 s, PASS | |
  | `mcpollama-mcp` | rc=1, 3 s, **FAIL** `TestWrongHashServerStarts` (`ocr_test.go:173`) | 1ª rodada teve também `TestNoLiteralContextLimits` por `../mcp/types.go` ausente (erro de envio, EV:1045); repetido com `src/mcp`, passou (EV:1080-1081) |
  | `tools` | rc=0, 8 s, PASS | |

### D-3 — `GOOS=linux go vet ./...` não consta nos gates

- **Onde no plano:** S1(c) cita `go vet ./...` "num Linux", mas o gate da S2 (linha 116) só exige `go test -count=1 ./...` e `-race`; S3 e §6.1 não têm `vet`.
- **O que o plano diz:** gate S2: "`go test -count=1 ./...` verde no Windows **e** no Linux; `-race` nos pacotes tocados".
- **O que a evidência mostra:** o `vet` cruzado compila e tipa **todos os 25 pacotes e os seus testes** para Linux sem precisar de Go no servidor. **Medido em 2026-11-13 ~11:48 na estação, HEAD `7fbe5cc`: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` → `rc=0`, 0 linhas de saída; `go vet -tags live ./ollama ./tools` com os mesmos GOOS/GOARCH → `rc=0`.** Isto cobre a **compilação** dos 18 pacotes não executados. O plano da Fase 12 (§11.2, linhas 549-550) já exige "build Linux (`GOOS=linux go build -o NUL ./...`)" e `gofmt -l .` como gate padrão; o plano da Fase 15 não os herda.
- **Classificação:** confirmado (medido; a medição é do cruzado no Windows, não do `vet` rodando no Linux).
- **Gravidade:** importante.
- **Reparo proposto:** acrescentar aos gates S1(c), S2 e S3: `Gate de compilação (estação): cd src; GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./... ; GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./... ; gofmt -l .` com `rc=0` e saída vazia nos três (valor de partida medido em 2026-11-13 11:48: `vet` rc=0, vazio). Registrar também que é verificação de **compilação/análise estática**, não execução, e que `go vet` rodando no próprio Linux não foi feito nem é necessário (Go ausente no servidor).

### D-4 — S2: reparo dos 2 testes sem tautologia e o texto do `--help` (a premissa do plano está parcialmente errada)

- **Onde no plano:** S2, linha 116; changelog 22:45, itens C.(1) e C.(2) (linha 163); §4.3 (linha 90).
- **O que o plano diz:** corrigir "`TestSplitRoots` com `os.PathListSeparator`; `TestWrongHashServerStarts` com `t.Skip` fora do Windows ou outra expectativa"; "**o `--help` (`config.go:144`) e a documentação dizem só `;`, o que engana**"; "corrigir o texto do `--help` e a documentação Linux (S4b)".
- **O que a evidência mostra:**
  1. **O `--help` já diz `:` no Linux.** `src/mcpollama-mcp/config.go:144-145`: `--allow-root DIR[;DIR]  absolute directories the file tools may read, separated by ;` / `(: on Linux), not nested; without it`. `git blame` das linhas 144-145: commit `9ef7935` de 2026-10-23 22:08 (a própria linha do Linux é de quando a flag nasceu). O comentário da linha 59 também cita `os.PathListSeparator`.
  2. **A documentação também já diz.** `D:\MCP\Documentation\Ollama\03-parametros.md:47`: "separadas por `;` (`:` no Linux)". Logo, a afirmação do changelog 22:45 de que `--help` **e** a documentação "dizem só `;`" é **refutada em parte**: o defeito real é só que a primeira linha do `--help` (`DIR[;DIR]` e "separated by ;") fica enganosa se lida sem a linha de baixo, e que `02-mcp-json-do-zero.md:24` dá o exemplo `E:\Projeto1;E:\Projeto2` sem nota de plataforma (confirmado por `grep` em 2026-11-13).
  3. `TestSplitRoots` (`guard_test.go:122-130`) usa literais `;` na entrada (`" a ; ;b;; c "`); reescrevê-lo com `os.PathListSeparator` **repete a lógica do código de produção** (tautologia: um erro de constante no código passaria no teste).
  4. `TestWrongHashServerStarts` (`ocr_test.go:164-178`) exige o texto "SHA-256" na saída (`ocr_test.go:172`). No Linux, `setupOCR` falha antes, por `ErrUnsupportedPlatform` = "ocr: OCR indisponível nesta plataforma" (`src/ocr/ocr.go:81`); a saída real foi `WARNING: OCR disabled: ocr: OCR indisponível nesta plataforma; the file tools run without OCR (scanned PDFs and images are refused)` (EV:1047-1050). `t.Skip` esconderia o contrato "o servidor sobe e avisa" no Linux; o teste pode **afirmar** o aviso nessa plataforma. O outro teste vizinho, `TestProbeFailureTurnsOCROff` (`ocr_test.go:183-186`), já usa `t.Skip` com o motivo "OCR is Windows only (D-10)": precedente do próprio repositório, mas aqui o ganho de afirmar é maior.
- **Classificação:** confirmado (itens 1-4 por leitura de código e `git blame`; "separador no Linux é `:`" provado em EV:861-867 e EV:930-957).
- **Gravidade:** importante (muda o que a S2 precisa alterar e corrige uma informação do changelog).
- **Reparo proposto (texto pronto para a S2 e para uma errata do changelog 22:45):**

  > **S2, item 1 (`TestSplitRoots`):** o teste passa a ter **literais por plataforma**, sem usar `os.PathListSeparator`: `sep, outro := ":", ";"` e, se `runtime.GOOS == "windows"`, `sep, outro = ";", ":"`. Casos: (i) `SplitRoots(" a " + sep + " " + sep + "b" + sep + sep + " c ")` deve dar `a|b|c`; (ii) `SplitRoots("a" + outro + "b")` deve devolver **um** elemento, `a<outro>b` (o outro separador é caractere comum do nome: no Linux `;` é válido em nome de pasta, SEC-m-02; no Windows `:` é o da unidade); (iii) `SplitRoots("")` vazio. **Prova por mutação** (obrigatória, registrar saída): trocar no código de produção `string(os.PathListSeparator)` por `";"` fixo e compilar o teste para Linux (`go test -c`, executar no servidor): deve **falhar** no caso (i); trocar por `":"` fixo e rodar no Windows: deve falhar; restaurar e ver verde nos dois.
  > **S2, item 2 (`TestWrongHashServerStarts`):** se `runtime.GOOS != "windows"`, em vez de `t.Skip`, afirmar: `code == 0`; 1 linha de resposta contendo `summarize_file`; stderr contém `WARNING: OCR disabled: ` e `indisponível nesta plataforma`; stderr **não** contém o caminho `root` (mantém a checagem da linha 175). No Windows mantém "SHA-256". **Prova por mutação:** fazer `setupOCR` não avisar fora do Windows e ver o teste falhar no Linux.
  > **S2, item 3 (texto do `--help`, opcional, informativo):** reescrever a primeira linha para `--allow-root DIR[SEP DIR]  absolute directories the file tools may read; SEP is ; on Windows and : elsewhere (os.PathListSeparator)` e manter "not nested". Mexe em `config.go` (arquivo não de teste): **dispara a regra de versão da D-5**. Não mexe em `src/files/` (os testes de ataque da Fase 4 só são exigidos ao tocar `src/files/`; `TestSplitRoots` fica em `src/files/guard_test.go`, **arquivo de teste de `src/files/`**: mudar só o `_test.go` não altera o guard, mas por prudência rodar os testes de ataque da Fase 4 de `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.testes` depois do ajuste, como a regra do CLAUDE.md pede para "qualquer mudança em `src/files/`").
  > **S2, item 4 (documentação):** na S4/S4b, os exemplos de `--allow-root` com mais de uma pasta no Linux usam `:` (`/root/treino:/root/outra`); a nota "`;` no Windows, `:` no Linux" já existe em `03-parametros.md:47` e deve ser mantida nas duas versões.
  > **Errata para o changelog 22:45 (item C.(2)):** onde diz "o `--help` (`config.go:144`) e a documentação dizem só `;`", acrescentar `**[errata E-n]**` com: "o `--help` desde 2026-10-23 (commit `9ef7935`) já traz `(: on Linux)` na linha seguinte (`config.go:145`) e `03-parametros.md:47` já diz `:` no Linux; o que engana é a primeira linha do `--help` e o exemplo de `02-mcp-json-do-zero.md:24`".

### D-5 — D15-1 × S2: mudar `config.go` muda o binário; falta a regra de versão

- **Onde no plano:** D15-1 (linha 100), questão 3 (linha 144), S2 (linha 116), S8 (linha 123), §4.3 (linha 90).
- **O que o plano diz:** "Versão: **0.10.4** só se houver mudança de código; se a fase for só empacotamento/validação, **sem nova versão** e sem tag (o binário continua `0.10.3`)".
- **O que a evidência mostra:** (a) nenhum código do MCP mudou desde `b1faec3` (`git log b1faec3..HEAD` sem mudanças em `src/mcpollama-mcp`, `files`, `tools`, `ollama`, `mcp`, `doctext`, `redact`, `ocr`; o commit `0973774` tocou só o monitor, AC:19); (b) **não existe "0.10.4" em nenhum lugar do repositório fora do plano** (grep em `CHANGELOG.md`, `README.md`, `CLAUDE.md`, `docs/`: só o PLANO e o índice desta revisão); o `CHANGELOG.md` só tem a seção "Não lançado" (linha 53) e `[0.10.3]` (linha 73); tags: `mcpollama-v0.10.3` é a última (`git tag`); (c) a S2 prevê editar `files/guard_test.go` e `mcpollama-mcp/ocr_test.go` (só testes, **não mudam o binário**) e, se o owner quiser, `config.go` (**muda o binário**); (d) o plano não distingue as duas coisas, e a questão 3 foi respondida de fato como "sem execução" (`TL:19`).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto; a decisão é do owner, vira questão nova):**

  > **Regra de versão da Fase 15 (proposta; aguarda a questão QD-1):** (i) mudança **só em `*_test.go`**: **sem nova versão, sem tag e sem novo binário**; o `0.10.3-b1faec3` de `/root/MCP` continua válido, e a S5 não se repete. (ii) **Qualquer mudança em arquivo não de teste** de `src/mcpollama-mcp`, `files`, `tools`, `ollama`, `mcp`, `doctext`, `redact` ou `ocr` (inclusive só o texto do `--help`): **0.10.4**, rótulo `0.10.4-<hash curto do commit que contém a mudança>`, tag anotada `mcpollama-v0.10.4` (só com autorização, D15-7), **dois binários gerados do mesmo commit** (Windows e Linux), `D:\MCP` e `/root/MCP` trocados com `.bak-0.10.3` (D-6 e 7.1), e a S8 passa a existir. (iii) O rótulo só pode ser calculado **depois** do commit (o hash faz parte do rótulo); build com árvore suja (`vcs.modified=true`) não é aceito.
  > **QD-1 (nova questão, numeração definitiva no consolidado 06):** "Se a S2 só alterar testes, mantemos o binário `0.10.3-b1faec3` (recomendado) ou publicamos 0.10.4 mesmo assim? Se alterarmos o `--help` (arquivo não de teste), aceita 0.10.4 + tag, ou deixamos o `--help` como está (a linha 145 já diz `: on Linux`) e a Fase 15 fica sem versão?" Impacto: 0.10.4 obriga trocar o binário em duas máquinas e reiniciar as sessões do Claude Code; sem versão, nada muda no binário. Rollback: `.bak-0.10.3` (D-6). Alternativa de não fazer: manter `0.10.3` e deixar o `--help` como está.

### D-6 — S3: `-trimpath -s -w` gera outro hash com o mesmo rótulo; `file` existe; `readelf` não medido; ETXTBSY

- **Onde no plano:** §4.2 (linhas 82-86), S3 (linha 117), S5 (linha 120), risco da linha 138.
- **O que o plano diz:** "`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=<versão>-<hash>"`"; "Conferir com `file`/`readelf` que é ELF estático"; "`SHA256SUMS` do binário gravado ao lado (como no `Build-Release.ps1` do molde, MCPO-REQ-BUILD-004)"; "Se já houver binário, guardar `mcpollama-mcp.bak-<versão>` **antes de sobrescrever**"; risco: "Sobrescrever um binário em uso".
- **O que a evidência mostra:**
  1. O binário em `/root/MCP` e em `D:\MCP` foi gerado **sem** `-trimpath` e **sem** `-s -w`: `go version -m` mostrou `build -ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `GOARCH=amd64`, `GOOS=linux`, `vcs.modified=false` (EV:308-314). O gateway é gerado do mesmo jeito (`deploy/gateway/Instalar-Gateway.ps1:67-68`: `-ldflags "-X main.Version=$Versao"`, sem `-trimpath`). Reconstruir pelo comando da §4.2 produz um binário **menor e com hash diferente** (`-s -w` remove símbolos; `-trimpath` muda os caminhos embutidos) **com o mesmo rótulo** `0.10.3-b1faec3`: dois arquivos diferentes com o mesmo `--version`. O requisito `MCPO-REQ-BUILD-004` (`60-Requisitos_mcpollama.md:272`) pede exatamente `-trimpath -s -w`, mas é `SHOULD` e **a prática do projeto nunca o seguiu** (CLAUDE.md:83-84 e `Instalar-Gateway.ps1` sem eles).
  2. `file` **existe no servidor**: o próprio comando de 21:51:20 o executou lá e imprimiu `mcpollama-mcp: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, Go BuildID=…` (EV:305, 318). A premissa "`file`/`readelf` podem não existir" é **refutada para `file`**; `readelf` não foi testado (binutils). `ldd` já foi medido ("não é um executável dinâmico", EV:789).
  3. `SHA256SUMS` **não existe** em `/root/MCP` (listagem EV:527-537 e EV:1089-1098: `cache certs Documentation mcp.json.exemplo mcpollama.env mcpollama-mcp VERSION`). `VERSION` guarda o hash em texto (EV:392-395), com modo `0600`.
  4. **ETXTBSY / binário em uso:** a tentativa de testar isso em 23:00:50 (EV:1428-1429, "binario em uso troca?") só **copiou** o binário para `/tmp/x.bin` e tentou subir um processo em segundo plano com `</dev/tty`; imprimiu `ok` e **não provou nada** sobre a troca. Semântica POSIX conhecida (não medida aqui): escrever em executável em uso (`cp` por cima, `>`) falha com "Text file busy"; `mv`/`rename` sobre ele é permitido, e o processo em execução mantém o inode antigo até reiniciar (as sessões abertas do Claude Code **não** passam a usar o binário novo).
  5. A convenção do `D:\MCP` é `mcpollama-mcp.bak-<versão>` com a versão do arquivo **guardado** (ex.: `.bak-0.10.2` = o binário 0.10.2, CHANGELOG.md:81 "backup `.bak-0.10.2`").
- **Classificação:** confirmado (itens 1-3, 5); item 4 conhecido por POSIX, **não medido** (testável em `/tmp`, ver abaixo); `readelf` não verificado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a S3 e a S5; a escolha entre (a) e (b) é do owner, vira QD-2):**

  > **S3, duas opções (QD-2):** **(a) manter o binário atual** (recomendado se a regra D-5 (i) valer): registrar no plano que ele **não** foi gerado pelo comando da §4.2 (sem `-trimpath`/`-s -w`), com hash `d61f5e8286f918b34d903216ae5b81dcfe7e3f4f84097e73132bbead40bd0333`, 22.770.050 bytes, e criar o `SHA256SUMS` a partir dele; o comando da §4.2 passa a valer só para o **próximo** binário (0.10.4). **(b) reconstruir só junto com a 0.10.4**, nunca com o rótulo `0.10.3-…` (dois binários diferentes com o mesmo rótulo não são aceitos). **Em nenhum caso** reconstruir "0.10.3" com flags diferentes.
  > **Comando e conferências da S3 (quando houver build):** `cd src; GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=<versão>-<hash>" -o <dir>/mcpollama-mcp ./mcpollama-mcp` com a árvore limpa (`git status --short src` vazio). Gates **mensuráveis**: (1) `go version -m <binário>` na estação mostra `-trimpath=true`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, `vcs.modified=false`; (2) **repetibilidade:** dois builds em pastas diferentes dão o **mesmo** SHA-256; (3) no servidor: `head -c 4 mcpollama-mcp | od -An -tx1` = `7f 45 4c 46`; `file mcpollama-mcp` diz `ELF 64-bit LSB executable, x86-64 … statically linked`; `ldd mcpollama-mcp` diz "não é um executável dinâmico" (os três já medidos para o binário atual; `readelf` é opcional, não medido); `./mcpollama-mcp --version` = rótulo esperado; (4) `sha256sum -c SHA256SUMS` = `OK`. Sem Go no servidor, o build é sempre na estação.
  > **S5, troca de binário (comandos):** `cd /root/MCP && sha256sum -c SHA256SUMS` (do novo, antes); `cp -p mcpollama-mcp mcpollama-mcp.bak-<versão-atual>`; `install -m 0755 <novo> .mcpollama-mcp.new && mv -f .mcpollama-mcp.new mcpollama-mcp` (mesmo diretório, `rename` atômico, sem ETXTBSY); `./mcpollama-mcp --version`; depois **reiniciar as sessões do Claude Code** que usam o MCP (as abertas seguem no binário antigo). Rollback: `cp -p mcpollama-mcp.bak-<versão> .mcpollama-mcp.new && mv -f .mcpollama-mcp.new mcpollama-mcp`.
  > **Prova do ETXTBSY (pendência P-n, só em `/tmp`, sem tocar `/root/MCP`):** `cp /bin/sleep /tmp/etxt && /tmp/etxt 30 & cp /bin/true /tmp/etxt` deve falhar com "Text file busy"; `cp /bin/true /tmp/etxt.new && mv -f /tmp/etxt.new /tmp/etxt` deve passar com o `sleep` seguindo vivo; apagar os temporários.

### D-7 — Premissa falsa em §4.4/questão 7: a documentação **não** tem fonte no repositório

- **Onde no plano:** §4.4 (linha 94: "A fonte **única** é o repositório (`docs/`, ver questão 7); as cópias em `D:\MCP\Documentation` e `/root/MCP/Documentation` são geradas dela, não editadas à mão"), questão 7 (linha 148), S4 (linha 118), S7 (linha 122).
- **O que o plano diz:** a documentação de uso do MCP tem fonte no `docs/` do repositório e as cópias são derivadas.
- **O que a evidência mostra:** `docs/` tem 5 arquivos (`CONTRATO-DADOS-TREINO.md`, `INSTALACAO.md`, `MANUAL-GATEWAY.md`, `README.md`, `RELATAR-ERROS-IA.md`); **os 7 documentos `01` a `07` e o `README` de `Documentation/Ollama` não estão no git** (`git ls-files | grep -i -E "Documentation|03-parametros|pre-requisitos"` só devolve skills; `Glob **/0[1-7]-*.md` só acha um template do `vcpkg` em `referencias/`). A fonte hoje é `D:\MCP\Documentation\Ollama\` editada à mão, com `.bak-<data>` por arquivo (listagem EV:91-106; o plano da Fase 14 diz "fora do repositório; **fora do git: copiar antes para `.bak` com data**", `fase14…plan.md:100`). Os arquivos têm **finais de linha CRLF** (95/95, 264/264, 145/145, 245/245, 83/83, 87/87, 88/88 e 52/52 linhas com `\r`; contagem de 2026-11-13) e foram copiados **byte a byte por `tar`** (EV:305), logo `/root/MCP/Documentation/Ollama` também está em CRLF (inferido da cópia binária; **não reverificado** no servidor).
- **Classificação:** confirmado (repositório e Windows); CRLF no servidor inferido.
- **Gravidade:** importante (a questão 7 está mal formulada: "manter a fonte única no repositório" supõe algo que não existe; e a S7 mandaria "copiar de `docs/`" sem origem).
- **Reparo proposto:** marcar a §4.4 e a questão 7 com `**superado em 2026-11-13 (revisão):** os 7 documentos e o README de uso não estão no repositório; a fonte atual é D:\MCP\Documentation\Ollama (CRLF, editada à mão, sem git).` e criar a **questão QD-3 (fonte da documentação)**, com opções (a recomendada primeiro):
  > **QD-3:** (a) **importar a documentação para o repositório** (ex.: `docs/mcp-ollama/windows/` e `docs/mcp-ollama/linux/`, ou um só conjunto com blocos por plataforma), com `.gitattributes` `eol=lf` para `*.md` dali, e gerar as cópias de `D:\MCP\Documentation` (CRLF) e de `/root/MCP/Documentation` (LF) por script (recomendado: dá versionamento e `git diff` à S4/S4b); (b) **manter `D:\MCP\Documentation\Ollama` como fonte** (fora do git, `.bak-<data>`) e **derivar a versão Linux por script de substituição** documentado, com revisão manual; (c) manter **duas cópias editadas à mão** (a opção atual; risco de divergência). Impacto: (a) cria arquivos no repositório (**não** na área protegida `Documentation/` do pack `.cursor`; `docs/` é livre, mas conferir a rule `documentation-migration-plan-mode_V1.2.0`); (b) e (c) não tocam o git. Rollback: (a) `git revert`; (b)/(c) restaurar os `.bak-<data>`. Alternativa de não fazer: deixar a documentação Linux sem versão e só a Windows existir.

### D-8 — S4/S4b: falta a tabela de mapeamento Windows → Linux, a política de divergência, os gates redigidos e as exclusões

- **Onde no plano:** S4 (linha 118), S4b (linha 119), §4.4.
- **O que o plano diz:** S4b: "`D:\MCP` → `/root/MCP`, `.exe` removido, PowerShell → bash, `--tls-ca` → …, token por `OLLAMA_API_KEY` (…**sem CR**), sem OCR, URL pública resolvendo para `127.0.0.1` pelo `/etc/hosts`"; gate: "0 ocorrências de `D:\`/`E:\`/`qwen3-ia` nos arquivos-alvo (e só nas exclusões declaradas), e o `.mcp.json` continua válido".
- **O que a evidência mostra:**
  1. **Contagem real dos padrões** nos 8 arquivos de `D:\MCP\Documentation\Ollama` (2026-11-13, `grep -cF`): `D:\` em 50 linhas (`01`:10, `02`:24, `03`:1, `06`:3, `07`:9, `README`:3), `E:\` em 1 (`02`, linha 24), `C:\` em 5 (`02`:2, `04`:3), `.exe` em 30 (`01`:9, `02`:10, `03`:1, `06`:3, `07`:5, `README`:2), `powershell` em 13 linhas, `$env:` em 3, `USERPROFILE` em 5.
  2. **O gate "0 `qwen3-ia`" entra em conflito com texto legítimo:** `qwen3-ia` aparece em 4 arquivos como **nota histórica** (`01-pre-requisitos-e-copia.md:68`, `06-solucao-de-problemas.md:26`, `07-checklist-ia.md:86`, `README.md:4`: "antes de 2026-11-05 era o alias `qwen3-ia`"). Na versão Linux essas notas não têm sentido (nenhuma sessão Linux rodou a 0.10.0), mas apagá-las tem de ser decisão consciente, não efeito do gate.
  3. **O gate com `node` não roda no servidor:** `command -v claude node` devolveu só `/root/.local/bin/claude` (EV:608-610); **`python3` existe** (usado em dezenas de comandos do servidor, ex.: EV:542-548, 615-625). A checagem "`.mcp.json` válido" deve usar `python3`.
  4. **O `--allow-root` de `/root/treino`:** a S4b manda `grep`/`sed` nos arquivos de `.cursor`, `.claude`, `.agents` etc.; o projeto do Windows usa **symlinks** de `.claude` para `.cursor` (CLAUDE.md, "Pack `.cursor/`"), mas `/root/treino/.claude` mostrou `agents commands plans` (EV:630-632), que **não é** o layout de symlinks do pack; não foi medido se o corpo copiado para o servidor trouxe os links, cópias duplicadas ou nada.
  5. Os 3 CSV (`20-CATALOGO-DIRETORIOS.csv`, `21-CATALOGO-PDFS.csv` de 1 MB, `22-CATALOGO-CURSOR.csv`) e as transcrições estão listados em `/root/treino` (EV:599-601); a exclusão proposta pela S4b é coerente, mas faltam os padrões exatos.
- **Classificação:** confirmado (contagens, `node` ausente, notas históricas); item 4 **não verificável sem o servidor**.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a S4/S4b):**

  > **Tabela de mapeamento (S4/S4b), a aplicar por arquivo e conferir por `diff`:**
  >
  > | Windows | Linux |
  > | --- | --- |
  > | `D:\MCP\mcpollama-mcp.exe` | `/root/MCP/mcpollama-mcp` (sem `.exe`) |
  > | `D:\MCP\cache\pdfium` | `/root/MCP/cache/pdfium` |
  > | `D:\Users\<u>\.openclaude\certs\cloudflare-origin-ca.pem` / `%USERPROFILE%` | `/root/MCP/certs/cloudflare-origin-ca.pem` / `$HOME` |
  > | `--api-key <TOKEN>` em `args` | `env.OLLAMA_API_KEY` (de `/root/MCP/mcpollama.env`, `0600`, **sem CR**) |
  > | JSON com `\\` nos caminhos | `/` |
  > | `E:\Projeto1;E:\Projeto2` em `--allow-root` | `/root/projeto1:/root/projeto2` (separador `:`) |
  > | `--ocr-lib`, `--ocr-lib-sha256`, `--ocr-tessdata`, `libtesseract64.dll`, `tessdata`, `--ocr-*` | **removidos**; texto "OCR só no Windows (D-10): PDF digitalizado e `.png`/`.jpg` são recusados" (mensagens reais: "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado", `doctext.go:217`; "imagem requer OCR, que não está habilitado no servidor", `doctext.go:230`) |
  > | blocos PowerShell (`$env:`, `Copy-Item`, `Get-FileHash`, `Get-Content`) | bash (`export`, `cp`, `sha256sum`, `cat`); **nunca gerar arquivo de ambiente a partir do Windows sem remover o CR** (incidente de 22:00, EV:581-583; é a armadilha já listada em `fase12…plan.md:584`) |
  > | `mcpollama-mcp.exe --version` | `/root/MCP/mcpollama-mcp --version` |
  > | URL e hosts | 3 cenários, ver D-12 |
  > | finais de linha CRLF | **LF** nos arquivos `.md`/`.json`/`.sh` da versão Linux (gate: 0 `\r`) |
  > | notas históricas de `qwen3-ia` e "sessão aberta antes de 2026-11-05" | **removidas de propósito** na versão Linux (listar as 4 linhas: `01:68`, `06:26`, `07:86`, `README:4`) |
  >
  > **Política de divergência:** depende da QD-3 (D-7). Em qualquer opção, a versão Linux declara no topo `Gerada de <fonte> em <data>`; mudança de conteúdo comum (parâmetros, tools) é feita **na fonte** e regenerada; só o que é específico de plataforma (blocos de instalação, rede, OCR) é escrito à mão e marcado.
  >
  > **Exclusões de `/root/treino` (padrões exatos):** `BRIEFS/**/Brief/*.transcricao.md`, `*.csv`, e qualquer `*.bak-*`.
  >
  > **Checagem prévia de symlinks (leitura):** `find /root/treino -maxdepth 3 -type l -printf '%p -> %l\n'` e `find /root/treino/.claude /root/treino/.agents -maxdepth 1 -printf '%y %p\n'`; editar **só arquivos reais** (alvo dos links), nunca por dois caminhos; `grep -r` (minúsculo) **não segue** symlinks, então fazer o inventário com `grep -rIl` e o das exceções por `find -type l`.
  >
  > **Gate S4b com redação exata:**
  > `grep -rIl -F -e 'D:\' -e 'E:\' -e 'C:\' -e 'D:/' -e 'E:/' -e 'qwen3-ia' -e '.exe' -e 'powershell' -e '$env:' -e 'libtesseract' /root/MCP/Documentation /root/MCP/mcp.json.exemplo <arquivos-alvo-de-/root/treino> | grep -v -E '\.bak-|\.transcricao\.md$|\.csv$'` deve devolver **0 linhas**; `python3 -c 'import json,sys; json.load(open(sys.argv[1]))' /root/treino/.mcp.json` com rc=0 (no servidor **não há `node`**); `grep -rIlc $'\r' /root/MCP/Documentation /root/MCP/mcp.json.exemplo /root/MCP/mcpollama.env` sem arquivo listado (0 CR); `head -c 3 <arquivo> | od -An -tx1` ≠ `ef bb bf` (sem BOM). Usar `-F` evita o escape `\\\|` do `grep` básico.

### D-9 — S4b manda criar backups `.bak-<data>` **dentro** do `--allow-root`; `/root/treino` é um repositório git; o `.bak` com token já está lá

- **Onde no plano:** S4b (linha 119: "**backup `.bak-<data>` de cada arquivo tocado**"), changelog 22:20 (linha 167: backup em `/root/treino/.mcp.json.bak-windows-20261112` "contém o token em texto").
- **O que o plano diz:** os backups ficam ao lado do original; o `.bak` do `.mcp.json` "agora `0600`".
- **O que a evidência mostra:**
  1. A denylist de nomes do guard (`src/files/policy.go:101-119`) recusa por **igualdade** `.mcp.json` (linha 105), prefixo `.env`, sufixos `.pem/.key/.pfx/.p12`, prefixos `id_*` e nomes com `credential`/`secret`. **`.mcp.json.bak-windows-20261112` não casa com nenhuma regra** (não é igual a `.mcp.json`; não contém `credential`/`secret`). `src/files/guard_test.go:265-266` não testa variantes `.bak`. Como `/root/treino` **é** a raiz permitida (`--allow-root /root/treino`, EV:661-662), qualquer sessão que chame `summarize_file`/`extract`/`translate` sobre esse caminho **lê o token em texto** e manda o conteúdo ao modelo; a resposta volta ao Claude. **Confirmado por leitura do código; não executado.**
  2. `/root/treino` **tem `.git`**: `ls -la /root/treino/.git | head -1` imprimiu `total 4076` (EV:633) e `git check-ignore -q .mcp.json` **falhou** (EV:674: "(sem repo git ou nao ignorado)"). Com `.git` presente, a mensagem significa "**não ignorado**" (hipótese a confirmar com `git -C /root/treino check-ignore -v .mcp.json; echo rc=$?`): o `.mcp.json` novo (que tem o token em `env`) e o `.bak` (token) podem entrar num `git add -A` do projeto. O plano nunca diz que `/root/treino` é repositório (o changelog 22:20 e a S4b não mencionam).
  3. Convenção `.bak-<data>` da S4b (`.bak-AAAAMMDD`) × nome real do backup existente (`.bak-windows-20261112`): inconsistência; o gate "0 ocorrências" **precisa excluir** `*.bak-*` (ele contém `D:\` por definição).
- **Classificação:** confirmado (item 1 por código; item 2 por EV:633/674 com a ressalva de hipótese; item 3 por EV:673).
- **Gravidade:** **crítico** (o procedimento previsto multiplica cópias de arquivos com credencial/caminhos dentro da raiz que o MCP lê, e o token do Ollama fica exposto à própria ferramenta; a decisão de mover/apagar o `.bak` existente é do owner e é cruzada com a etapa 04).
- **Reparo proposto (texto pronto; ações no servidor exigem autorização, viram questão e pendência):**

  > **S4b, backup (substitui "`.bak-<data>` de cada arquivo tocado"):** os backups **ficam fora do `--allow-root`**, em `/root/MCP/backups/treino-AAAAMMDD/` (modo `0700`, estrutura de caminhos relativa preservada, `cp -p`), ou, se `/root/treino` estiver limpo (`git -C /root/treino status --short` vazio), o rollback é `git -C /root/treino checkout -- <arquivos>` e **não** se cria cópia. Gate: `find /root/treino -name '*.bak*' | wc -l` = 0 após a S4b (ou só o que o owner decidir manter).
  > **Pré-checagens somente leitura da S4b (P-n):** `git -C /root/treino status --short | head`; `git -C /root/treino check-ignore -v .mcp.json; echo rc=$?`; `git -C /root/treino ls-files .mcp.json .mcp.json.bak-windows-20261112`; `git -C /root/treino log --oneline -3 -- .mcp.json`; `git -C /root/treino remote -v`. Se o `.mcp.json` com token estiver **versionado ou com remoto**, tratar como incidente de credencial (rotação do token, `deploy/servidor-ia/nginx/ollama-key-rotate.sh`) **por decisão do owner**.
  > **Questão QD-4 (nova):** "O `.mcp.json.bak-windows-20261112` (token em texto, dentro do `--allow-root`) deve ser (a) movido para `/root/MCP/backups/` com `0600` (recomendado), (b) apagado depois de conferir que o `.mcp.json` novo funciona, ou (c) mantido?" Impacto: nenhum serviço usa o `.bak`; (a)/(b) eliminam o vetor de leitura pela ferramenta. Rollback de (a): `mv` de volta; de (b): não há (era cópia do arquivo do Windows, que existe na estação). Alternativa de não fazer: o token segue legível por `summarize_file` em `/root/treino`. **Sem alterar o servidor nesta revisão.**
  > **Fora do escopo (reportar):** a denylist por nome **não cobre variantes de backup** (`.mcp.json.bak*`, `.env.bak` já cobre por prefixo `.env`, mas `settings.local.json.bak` e `.mcp.json~` não). É decisão de produto sobre `src/files/policy.go`; mexer exige os testes de ataque da Fase 4.

### D-10 — S6 incompleta: itens que faltam, com gate mensurável por item

- **Onde no plano:** S6 (linha 121), §1 item 3 (linha 25: "`list_models`, `generate`, `chat`, `embed` e, com `--allow-root`, `summarize_file`, `extract`, `translate`, `reportar_erro`"), changelog 22:35 (linha 159).
- **O que o plano diz:** gate "cada tool responde; jobs aparecem no `/monitor` com o rótulo esperado"; a §1 promete 8 tools.
- **O que a evidência mostra:** das 8 tools do `tools/list` (EV:378), foram exercitadas **5** (`list_models`, `summarize_file`, `extract`, `embed`, `generate`). **Faltam `chat`, `translate`, `reportar_erro`.** Também faltam `.docx`/`.pptx`, a recusa de PDF digitalizado e de imagem, e a conferência de rótulo. O `summarize_file` foi de `.md`, não de `.txt`, e o PDF foi por `extract`, não por `summarize_file` (a S6 pede os dois). A chamada pelo Claude Code só passou com `--mcp-config --strict-mcp-config` (que dispensa a aprovação) e com `env -u CLAUDE_CODE_OAUTH_TOKEN`; a aprovação do servidor de projeto segue pendente (EV:1385 "Pending approval").
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (tabela pronta para a S6; cada item é uma chamada, com a "uma chamada por vez" de `fase12…plan.md:562`; `reportar_erro` e a criação de arquivos de teste exigem autorização, ver abaixo):**

  | # | Item | Como | Gate mensurável |
  | --- | --- | --- | --- |
  | 1 | `chat` | 2 mensagens curtas, `seed` fixo | `isError` falso; linha `[uso]` com `modelo=qwen3.8:27b` e `job=<id>` |
  | 2 | `translate` | `.md` pequeno dentro da raiz, `target_language` `en` | `isError` falso; `[arquivo] … formato=…`; identificadores e blocos de código preservados (conferir 3 por amostragem) |
  | 3 | `summarize_file` de `.txt` e de PDF com texto | arquivo na raiz permitida | `isError` falso; `[arquivo]` com `formato=txt` e `formato=pdf paginas=<n> paginas_sem_texto=0` |
  | 4 | `.docx` e `.pptx` | 1 de cada, pequeno, em raiz de teste (2ª raiz com `:` e apagada depois, como o `/tmp/mcptest` de 22:15) | `isError` falso; `formato=docx` e `formato=pptx` |
  | 5 | **PDF digitalizado deve ser RECUSADO** | PDF sem camada de texto | `isError` **verdadeiro** com a mensagem exata "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado" (`doctext.go:217`) e **sem** chamada ao modelo (tempo de ~0,2 ms como as recusas de EV:951-954) |
  | 6 | `.png`/`.jpg` recusado | 1 imagem | `isError` verdadeiro com "imagem requer OCR, que não está habilitado no servidor" (`doctext.go:230`) |
  | 7 | `reportar_erro` | ver QD-5 | rc sem erro e **número do registro** devolvido; registro marcado como teste |
  | 8 | Rótulo no `/monitor` | ver D-11 | `client_label` = `ia.cslsolucoes.com.br/root` |
  | 9 | Aprovação interativa | o **owner** roda `cd /root/treino && claude` e aprova | `claude mcp list` em `/root/treino` mostra o servidor `mcpOllama` conectado, **não** "Pending approval" (EV:1385 é o estado de partida) |
  | 10 | Chamada normal | após 9, em shell de login, **sem** `--mcp-config`, `--strict-mcp-config` nem `env -u`: `cd /root/treino && claude -p "Chame a ferramenta list_models do servidor mcpOllama …" --allowedTools mcp__mcpOllama__list_models` | rc=0 e saída com `qwen3.8:27b, nomic-embed-text:latest` |
  | 11 | Sessão reiniciada após troca de binário | só se a S2 gerar 0.10.4 | `--version` visto pelo cliente = o esperado (sessão aberta não troca de binário) |

  **`reportar_erro` (item 7) — cuidado com a base de treino:** `docs/RELATAR-ERROS-IA.md` **não prevê** procedimento de teste (a busca por "teste/test/sandbox" só acha o texto de `jobs`), ao contrário do que a tarefa supunha. A tool grava um **candidato** no `treino.db` do servidor (`RELATAR-ERROS-IA.md:44`), com pedido e resposta do job, e a lista de transições (`CONTRATO-DADOS-TREINO.md`, C-1.12) só permite sair da estatística por **descartar** (ação exclusiva do admin, C-1.13). Procedimento proposto, **mínimo e reversível**: usar um job do próprio teste (`chat` do item 1, com texto fictício), categoria `outro`, `erro` iniciando por `[TESTE Fase 15, descartar]`, **`resposta_correta` vazia** (o doc, §4, diz que registro sem resposta correta "serve só para estatística" e não entra no treino; C-1.18: nada entra no treino sem confirmação do admin), `projeto` = `mcpollama-fase15`, e pedir ao admin que **descarte** o registro (C-1.12: sai também da estatística). Isto **escreve na base de produção**: depende de autorização do owner.
  **QD-5 (nova questão):** "Autoriza 1 chamada de `reportar_erro` de teste na S6, com o procedimento acima, e o admin descarta o registro? Ou prefere não testar `reportar_erro` no Linux (a tool está presente no `tools/list`, EV:378, e o contrato é o mesmo do Windows)?" Impacto: 1 linha na base de erros até ser descartada; sem outros efeitos. Rollback: descartar/apagar o registro no `/monitor` (admin). Alternativa de não fazer: S6 conclui com `reportar_erro` como "presente, não exercitada", o que deve constar no gate.

### D-11 — Rótulo da estação no `/monitor`: valor esperado e a chave certa (a conferência de 22:30 foi cega)

- **Onde no plano:** fato 4 da §2 (linha 35: "o rótulo padrão `<máquina>/<usuário>` no `/monitor` será `<hostname>/root`"), S6 gate (linha 121), changelog 22:45 "Não verificado: o rótulo da estação no `/monitor` para esses jobs (não achei nos logs…)" (linha 157).
- **O que o plano diz:** o rótulo é `<hostname>/root`; não foi conferido.
- **O que a evidência mostra:**
  1. O `hostname` do servidor é o **FQDN**: `hostname=ia.cslsolucoes.com.br | hostname -s=ia` (EV:1415). `clientLabel()` usa `os.Hostname()` (`config.go:512`) e `defaultLabel` devolve `host + "/" + usr` (`config.go:528`): o rótulo esperado é **`ia.cslsolucoes.com.br/root`**, não `ia/root` (sem `--client-label`). `station_computer` = o mesmo hostname; `station_user` = `root`; `station_domain` vazio (no Linux `USERDOMAIN` não existe); `station_mac` = MAC da interface ativa com IPv4 privado (`station.go:23-24`, `166-170`).
  2. A conferência de 22:30 (EV:1291-1306) procurou, em `recent[]`, as chaves `label`/`client`/`cliente`/`rotulo`/`tool`/`model`/`status`/`state` e imprimiu só `tool`, `model`, `status`; **a chave real é `client_label`** (`queue.go:1131`, `store.go:655`, ambas `json:"client_label"`; os campos da estação são `station_computer`, `station_user`, `station_domain`, `station_mac`, `store.go:685-688`). O `grep` no `journalctl` também não acha rótulo (o gateway não o loga). Ou seja, "não achei" **não prova** que o rótulo não aparece; a leitura estava cega à chave.
  3. As sessões do mesmo usuário na mesma máquina compartilham **um** cliente na fila (`config.go:506-507`), então todo MCP rodando como root no servidor aparece como o mesmo cliente.
- **Classificação:** confirmado (itens 1-3 por leitura de código e EV); o valor efetivo no `/monitor` **não verificável sem o servidor** (uma leitura `curl` somente leitura resolve, sem gerar job novo se o `recent` ainda tiver os 10 jobs).
- **Gravidade:** importante.
- **Reparo proposto:**
  > **S6, item 8 (gate):** `curl -s --cacert /root/MCP/certs/cloudflare-origin-ca.pem https://ia.cslsolucoes.com.br:8443/monitor/api/state | python3 -c '…'` percorrendo `recent[]` (e `queue.running[]`) e imprimindo `client_label`, `station_computer`, `station_user`, `station_domain`, `station_mac`: **pelo menos 10 itens** com `client_label == "ia.cslsolucoes.com.br/root"` (os 10 `generate` de 22:30) e `station_user == "root"`. A leitura do estado é livre (sem token, EV:1175, 1304) e vem pelo nginx via loopback. Se o gateway tiver reiniciado, gerar 1 `generate` curto e ler de novo.
  > Corrigir no plano o valor esperado do rótulo: `**superado em 2026-11-13 (revisão):** o rótulo esperado é <FQDN>/root = ia.cslsolucoes.com.br/root (o hostname do servidor é o FQDN, EV:1415).`

### D-12 — Documentação de rede: separar 3 cenários e a CA

- **Onde no plano:** fato E (linha 63), questão 10 (linha 151), changelog 21:58 e 22:05 (linhas 173, 171), S4b "(URL pública resolvendo para `127.0.0.1` pelo `/etc/hosts`)".
- **O que o plano diz:** o MCP no servidor usa `https://ia.cslsolucoes.com.br:8443`, `--tls-ca` e o `/etc/hosts` apontando para `127.0.0.1`; a documentação Linux "resolve para `127.0.0.1` pelo `/etc/hosts`".
- **O que a evidência mostra:** esse é **um** de três cenários, e só vale **no próprio servidor**. (i) **No próprio `lx-ai01`**: sem a linha em `/etc/hosts` o nome resolve para 2 endereços IPv6 (EV:441-442, `2804:2904:715:5c01::` e `fe80::…`) e o `curl` ainda passava (HTTP 401, EV:443); com a linha, `getent ahostsv4` dá `127.0.0.1` (EV:495) e o TLS valida pelo nome, **não pelo IP** (`https://127.0.0.1:8443` falha com `ssl_verify=1`, EV:459-461). Depois do reinício a linha persistiu (`grep -c` = 1, EV:1224). **Hipótese não verificada:** `getent hosts` continuou listando os 2 IPv6 depois da edição (EV:496-497); o cliente Go pode tentar IPv6 primeiro e cair para `127.0.0.1` (o nginx escuta `0.0.0.0:8443` e `[::1]:8443`, EV:453-456, não no IPv6 global); funcionou 10/10 em 22:30, mas a origem desses 2 IPv6 (hosts ou DNS) e o comportamento de fallback não foram medidos. (ii) **Outra máquina da LAN**: `https://ia.cslsolucoes.com.br:8443` pelo **DNS interno** (split-horizon, `10.100.2.2`), sem `/etc/hosts`, com o PEM da CA local copiado (CLAUDE.md, "Na LAN"). (iii) **Internet**: o mesmo host **sem porta** pelo túnel Cloudflare (CLAUDE.md); de dentro da LAN esse caminho dá timeout (split-horizon). O CLAUDE.md diz que o SO não confia no Origin CA e pede `--tls-ca`; isso vale para o `:8443` (nginx com certificado de origem); **se o caminho do túnel precisa de `--tls-ca` não foi medido** (o certificado da borda é público; hipótese).
- **Classificação:** confirmado para o cenário (i) (medido); (ii) e (iii) por CLAUDE.md, **não medidos nesta fase**; IPv6 **não verificável sem o servidor**.
- **Gravidade:** importante (a documentação Linux que a S4b descreve só serve ao cenário (i) e induziria uma estação Linux da LAN a editar `/etc/hosts` sem necessidade).
- **Reparo proposto (seção da documentação Linux e do plano):**

  | Cenário | Quem | URL | `/etc/hosts` | `--tls-ca` | Status |
  | --- | --- | --- | --- | --- | --- |
  | 1. No próprio servidor `lx-ai01` | MCP em `/root/MCP` | `https://ia.cslsolucoes.com.br:8443` | **sim**: `127.0.0.1   ia.cslsolucoes.com.br` (backup `/etc/hosts.bak-20261112`) | `/root/MCP/certs/cloudflare-origin-ca.pem` (sha256 `01d12a7c938de82b3b264458a24dd2cda139814393a920d8b5b9d0aa6a54d8f3`) | medido em 2026-11-12; não reverificado |
  | 2. Outra máquina Linux da LAN | Claude Code/MCP em outra estação | `https://ia.cslsolucoes.com.br:8443` | **não** (DNS interno 10.100.2.2) | mesmo PEM (copiar a CA **pública**, conferir o sha256) | **não medido** |
  | 3. Internet | fora da LAN | `https://ia.cslsolucoes.com.br` (443, sem porta, túnel) | não | a confirmar | **não medido** |

  Mais a nota: `https://127.0.0.1:8443` nunca (TLS valida o nome); o gateway local `127.0.0.1:11500` fica fora (decisão da questão 10).

### D-13 — §4.5 Inventário de artefatos: o que já existe, o que falta, e uma ação do servidor que o plano não registra

- **Onde no plano:** §4.1 (árvore, linhas 68-80), changelog.
- **O que o plano diz:** a árvore da §4.1 (`mcpollama-mcp`, `.bak-<versão>`, `cache/pdfium`, `Documentation/Ollama`, `mcp.json.exemplo`, `VERSION`); nada sobre `certs/`, `mcpollama.env`, `/etc/hosts`, `/root/treino`.
- **O que a evidência mostra:** o servidor tem mais coisas do que a §4.1 e o plano não tem tabela única; além disso, o comando de 23:00:50 (1ª revisão, EV:1387-1430; TL:107) fez no servidor, **sem registro no plano**: `cp /root/MCP/mcpollama-mcp /tmp/x.bin` e `rm -f /tmp/x.bin` no mesmo comando, e `( /root/MCP/mcpollama-mcp --ollama-url https://127.0.0.1:1 </dev/tty >/dev/null 2>&1 & )`; esse último, sem terminal de controle (ssh em lote), provavelmente **falhou ao abrir `/dev/tty` e não subiu**, mas isso **não foi verificado**; se subiu, haveria um processo órfão.
- **Classificação:** confirmado (a ação existe e não está no plano); o processo órfão, **não verificável sem o servidor**.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a subseção 4.5; estados lidos em 2026-11-12, não reverificados em 2026-11-13):**

  | Caminho | Operação | Modo / dono | Nota e evidência |
  | --- | --- | --- | --- |
  | `/root/MCP/` (servidor `lx-ai01`) | criar | `0700` root | 21:51 (EV:319-324, 1089-1098); não existia (EV:299) |
  | `/root/MCP/mcpollama-mcp` | criar (cópia de `D:\MCP\mcpollama-mcp`) | `0755` root | `0.10.3-b1faec3`, 22.770.050 B, sha256 `d61f5e82…0333` (EV:307, 316); **sem `-trimpath`/`-s -w`** (EV:308-314) |
  | `/root/MCP/cache/pdfium/` | criar | `0700` | 1ª leitura de PDF criou `wazero-v1.12.0-amd64-linux` (22:15, EV:975-978) |
  | `/root/MCP/Documentation/Ollama/` (7 + README) | criar (cópia) | `0644`, dir `0755` | **cópia Windows em CRLF**, sem `.bak` (EV:305, 327-338); a refazer na S4 |
  | `/root/MCP/VERSION` | criar | `0600` | hash e versão (EV:392-395) |
  | `/root/MCP/mcp.json.exemplo` | criar e editar | `0600` | sem token (EV:507-525) |
  | `/root/MCP/certs/cloudflare-origin-ca.pem` | criar | dir `0700`, arquivo `0644` | CA **pública**, 2 certificados, 0 chaves, válida até 15/08/2029 (EV:422-434) |
  | `/root/MCP/mcpollama.env` | criar | `0600` | `OLLAMA_API_KEY` de 64 caracteres; CR removido às 22:00 (EV:581-583); **contém o token** |
  | `/etc/hosts` (**fora de `/root/MCP`**) | editar | — | +3 linhas (linha em branco, comentário e `127.0.0.1   ia.cslsolucoes.com.br`), backup `/etc/hosts.bak-20261112` (`cp -p`), EV:483-499 |
  | `/root/treino/.mcp.json` | refazer (Linux) | `0600` | 486 B, `command` `/root/MCP/mcpollama-mcp`, token em `env` (EV:638-674); **`/root/treino` tem `.git`** (EV:633) |
  | `/root/treino/.mcp.json.bak-windows-20261112` | criar | `0600` | **token em texto**, **dentro do `--allow-root`** (ver D-9) |
  | `/tmp/mcptest/…` | criar e apagar | — | apagado 22:18 (EV:1088) |
  | `/tmp/x.bin` | criar e apagar no mesmo comando | — | 23:00:50 (EV:1391); **ação não registrada no plano** |
  | processo `mcpollama-mcp` em segundo plano (23:00:50) | possível | — | **não verificado**; conferir com `pgrep -a mcpollama-mcp` |
  | reinício do servidor | executar | — | 22:28, por ordem do owner (`TL:86`); estado final lido (EV:1195-1226, 1311-1314) |
  | `/root/.bashrc` | **alterado pelo owner** (22:25:13 e 22:47:12), não pelo Claude | — | `CLAUDE_CODE_OAUTH_TOKEN` válido a partir de 22:47:12 (AC §1.3); **não é ação do plano**; não registrar como tal |
  | `/root/.claude/.credentials.json`, `/root/.claude.json` | **só lidos** (`ls`) | `0600` | preexistentes, de 21:31 (EV:1119-1120) |
  | Estação: `D:\MCP\*` | **nada alterado** | — | hash de `mcpollama-mcp` reconferido em 2026-11-13 11:50 |
  | Estação: binários de teste `lt\*.test` | criar (scratchpad da sessão) | — | temporários locais; apagar |
  | Repositório: o PLANO | criar e editar | — | commits `f6bc6ab` e `7fbe5cc` (push 22:32:54); **nenhum código** |

  **Faltam (não criados, previstos):** `SHA256SUMS`; `mcpollama-mcp.bak-<versão>` (só quando houver troca); documentação Linux (S4); script ou documento de build repetível (§1 objetivo 4); fonte da documentação no repositório (QD-3); entradas do CHANGELOG/README/CLAUDE.md (S7); `/root/MCP/backups/` (se QD-4 for (a)).

### D-14 — §7.1 Rollback: por ação já executada e por etapa futura

- **Onde no plano:** §7 (riscos, linhas 125-138) tem mitigações, não tem rollback; só `/etc/hosts` menciona "reverter = apagar as 2 linhas ou restaurar o backup" (linha 171, e a diferença real é de **3** linhas, EV:490-493, uma em branco).
- **O que o plano diz:** "Sobrescrever um binário em uso → backup `.bak-<versão>` antes; reiniciar a sessão"; nada para as ações já feitas.
- **O que a evidência mostra:** 8 ações no servidor/repositório já executadas sem rollback por escrito (D-13).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a subseção 7.1):**

  **Por ação já executada** (estado lido em 2026-11-12; os comandos **não foram executados** na revisão):

  | Ação | Reverter | Impacto de reverter | Conferência |
  | --- | --- | --- | --- |
  | `/root/MCP` criada | primeiro `mv /root/MCP /root/MCP.removido-AAAAMMDD` (reversível); só depois, com ordem do owner, `rm -rf` | o `.mcp.json` de `/root/treino` aponta para `/root/MCP/…`: reverter a árvore **sem** restaurar o `.mcp.json` quebra o servidor MCP de `/root/treino` | `ls /root/MCP` falha; `claude mcp list` em `/root/treino` |
  | `/root/MCP/mcpollama.env` (token) | apagar o arquivo (faz parte da árvore); **rotacionar o token** só se houver suspeita de vazamento (`deploy/servidor-ia/nginx/ollama-key-rotate.sh`, decisão do owner) | quem usa o token do Ollama passa a ter outro | — |
  | `/etc/hosts` (+3 linhas) | `diff /etc/hosts.bak-20261112 /etc/hosts` (para ver se mais alguém editou) e depois `cp -p /etc/hosts.bak-20261112 /etc/hosts` | o nome volta a resolver para os 2 IPv6 (EV:441-442): o MCP local pode passar a falhar ou contornar o `:8443` por loopback; o baseline de 21:58 mostra que só o `server_name` do nginx e o `ollama-install-cert` citam o nome (EV:466-477) | `getent ahostsv4 ia.cslsolucoes.com.br` |
  | `/root/treino/.mcp.json` (Linux) | `cp -p /root/treino/.mcp.json.bak-windows-20261112 /root/treino/.mcp.json` | restaura o arquivo do Windows (`D:\MCP\mcpollama-mcp.exe`, `E:\Treinamento`, OCR), **inútil no Linux**, e volta a ter token em texto; o servidor MCP de `/root/treino` deixa de funcionar | `python3 -c 'import json;json.load(open(…))'` |
  | `/root/treino/.mcp.json.bak-windows-20261112` | ver QD-4 (D-9) | — | `find /root/treino -name '*.bak*'` |
  | `/tmp/mcptest`, `/tmp/x.bin` | já removidos / `rm -f /tmp/x.bin` | nenhum | `ls /tmp/mcptest /tmp/x.bin` |
  | Reinício de 22:28 | **irreversível**; sem ação | — | estado final já lido (todos os serviços `active`, 0 Xid) |
  | Commits `f6bc6ab`, `7fbe5cc` (push feito) | `git revert <hash>` (não reescrever histórico publicado) | só documentação | `git log` |
  | Nada no código | — | — | `git log b1faec3..HEAD` sem mudança em `src/` do MCP |

  **Por etapa futura:** **S2** (testes/`--help`): `git revert` do commit da S2 (um commit por correção); nenhum binário muda se só testes; se 0.10.4, ver S5. **S3**: nada é instalado; descartar o artefato de build e o `SHA256SUMS`. **S4** (documentação Linux): backups fora do `--allow-root` e `git revert` se a fonte for o repositório (QD-3). **S4b**: backup em `/root/MCP/backups/treino-AAAAMMDD/` ou `git checkout -- <arquivos>`; **gate de rollback**: após restaurar, o `diff -r` contra o backup deve ser vazio. **S5 (troca de binário)**: ver comandos em D-6 (`cp -p …bak-<versão>` + `mv -f` atômico); **nunca** `cp` por cima do binário em uso; reiniciar sessões. **S7**: `git revert` por arquivo (CHANGELOG/README/CLAUDE.md), e restaurar `.bak-<data>` das cópias de documentação fora do git. **S8**: tag só com autorização; remover tag local antes do push, `git push --delete origin <tag>` só por ordem do owner.

### D-15 — 1.2 Definição de Pronto e registro de aprovação formal (S0): o plano não tem

- **Onde no plano:** não existe seção de critérios de conclusão; o §1 tem só 4 objetivos sem verificação; a S0 (linha 114) só diz "respostas registradas na §8".
- **O que o plano diz:** gate da S0: "respostas registradas na §8"; frontmatter `status: draft`.
- **O que a evidência mostra:** a rule `workspace-plans-persist_V1.4.0` exige "Critérios de conclusão" (linha 151 da rule) e "Rollback" (linha 155); o plano executou 6 das 9 etapas sem que o owner tenha aprovado formalmente; o owner pediu "sem execução do plano" (`TL:19`, 21:54:48) e, ao mesmo tempo, mandou fazer deploy e ajustes em ordens avulsas (`TL:10`, `TL:29`, `TL:42`, `TL:58`, `TL:86`).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto):**

  > **1.2 Critérios de conclusão (Definição de Pronto da Fase 15).** A fase só é "concluída" quando **todos** os itens abaixo têm evidência lida e registrada (número, comando e data), ou uma exceção aceita por escrito pelo owner:
  > 1. **Binário no Linux:** `/root/MCP/mcpollama-mcp --version` = o rótulo da regra D-5 (hoje `0.10.3-b1faec3`); `initialize` e `tools/list` por stdio listam as 8 tools com `seed`; sha256 = `SHA256SUMS`. *(Estado: atendido para o rótulo atual, EV:316-317, 377-379; falta `SHA256SUMS`.)*
  > 2. **Testes:** `go test` dos 7 pacotes executado no Linux com **0 FAIL** e `SKIP` listados; `go test -count=1 ./...` no Windows com 0 FAIL; `GOOS=linux go vet ./...` rc=0 e `gofmt -l .` vazio (D-2, D-3).
  > 3. **S2:** os 2 testes corrigidos **com prova por mutação**; se tocou arquivo não de teste, regra de versão (D-5) cumprida.
  > 4. **Árvore:** `/root/MCP` conforme a §4.5 (modos `0700`/`0600`/`0755`), sem arquivos fora do inventário.
  > 5. **Build repetível documentado** num único lugar e conferido (dois builds, mesmo hash), ou registro de que o binário atual não o segue (D-6).
  > 6. **Documentação Linux:** 7 documentos + README, com gate de D-8 (0 ocorrências dos padrões, 0 CR, JSON válido por `python3`) e **cada comando documentado executado uma vez**.
  > 7. **S6:** os 11 itens da tabela de D-10 com gate cumprido ou exceção aceita; rótulo `ia.cslsolucoes.com.br/root` visto no `/monitor`.
  > 8. **Segurança:** nenhum token em arquivo dentro de `--allow-root` fora do `.mcp.json` ativo; `.bak` com token tratado (QD-4); `/root/treino` com `.mcp.json` **ignorado** pelo git ou fora do repositório; varredura de segredos nas linhas adicionadas = 0.
  > 9. **Rollback documentado** (7.1) e, se houver troca de binário, **ensaiado** uma vez (trocar para o `.bak` e voltar; `--version` confere nas duas pontas).
  > 10. **Pendências P-n** (6.2) zeradas ou aceitas por escrito.
  > 11. **Documentos atualizados** (S7) e commits por caminho, sem arrastar o trabalho de outra sessão (D-17); push e tag só com autorização.
  > 12. **Aprovação do owner** registrada na 0.3 (ver abaixo).
  >
  > **Registro de aprovação formal (S0), para preencher:** `Data/hora real: ____ · Escopo aprovado: (a) regularização do que foi executado fora de ordem (S5, parte da S1/S4/S4b/S6) · (b) execução das etapas restantes (S1(c) completa, S2, S3, S4, S4b, S6, S7, S8) · Citação literal do owner: "____" · Questões fechadas: 1 a 12 e QD-n · Exceções aceitas: ____`. **Enquanto este registro estiver vazio, `status: draft` permanece** e nenhuma etapa pendente é executada além do que o owner ordenar uma a uma.

### D-16 — Referências sem caminho, S8 com "release", e a linha de build Linux do CLAUDE.md

- **Onde no plano:** linha 15 ("a §11 do plano da Fase 12", "Fase 14"), D15-7 (linha 106, "§11.1 da Fase 12"), S8 (linha 123, "tag e release criadas"), §4.2 linha 86 ("`Build-Release.ps1` do molde").
- **O que a evidência mostra:**
  1. A §11 da Fase 12 **existe** (`.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`, §11 na linha 515, §11.1 na linha 517; a regra "Push e tags só com autorização do owner" está na linha 535), mas o plano da Fase 15 **não dá o caminho**; e o plano da Fase 15 diz que **não repete** essa metodologia, enquanto repete justamente uma armadilha da §11.4 (linha 584: `here-strings` enviadas por `ssh` ganham `\r` na última linha), com o incidente do token com CR (EV:581-583).
  2. A **Fase 14**: o plano foi movido de `.workspace/plans/` para `.workspace/plans/concluidos/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` pela **outra sessão**, ainda **não commitada** (`git status`: ` D .workspace/plans/MCPOllam2026-11-07-…` e `?? .workspace/plans/concluidos/`). O plano da Fase 15 cita "Fase 14" sem caminho; ao commitar, o caminho certo será `plans/concluidos/`.
  3. **S8 diz "tag e release criadas"**, mas o `CHANGELOG.md:4-5` afirma que "as versões não têm release formal no GitHub, só **tags anotadas**". A S8 deve pedir só a tag anotada.
  4. **CLAUDE.md, linha 84** (`$env:GOOS='linux'; go build -o D:\MCP\mcpollama-mcp ./mcpollama-mcp; Remove-Item Env:GOOS`): sem `-ldflags "-X main.Version=…"` (o rótulo cairia em `dev`, `main.go:22`), sem `CGO_ENABLED=0` (o cross-build já desliga o cgo por padrão, mas o objetivo "ELF estático" depende disso e não está escrito), sem `GOARCH`. O binário atual foi gerado com as flags corretas, portanto **por outro comando**, que **não está documentado em lugar nenhum** (o objetivo 4 do plano).
- **Classificação:** confirmado.
- **Gravidade:** informativo (itens 1-3 corrigíveis no plano); o item 4 é **fora do escopo** do plano (CLAUDE.md está sendo editado por outra sessão, ver D-17): só reportar.
- **Reparo proposto:** na linha 15 e em D15-7, acrescentar os caminhos: `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (§11, §11.1 e §11.4) e `.workspace/plans/concluidos/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` (`.workspace/plans/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` no HEAD `7fbe5cc`, antes da movimentação feita pela outra sessão); no §6 S8: trocar "tag e release criadas" por "tag anotada criada e conferida (`git tag -l`)", com `**superado em 2026-11-13 (revisão)**`; na §4.2: citar `referencias/MCPVMWare/tools/Build-Release.ps1` e o requisito `MCPO-REQ-BUILD-004` (`referencias/Brief_Referencias/60-Requisitos_mcpollama.md:272`, `SHOULD`) e a prática real (`deploy/gateway/Instalar-Gateway.ps1:67-68`).

### D-17 — S7: o que entra em CHANGELOG, README, CLAUDE.md e documentos de servidor; CLAUDE.md está sendo editado por outra sessão

- **Onde no plano:** S7 (linha 122).
- **O que o plano diz:** "CHANGELOG (seção 'Não lançado' ou versão, conforme D15-1), `README.md` (nota 'Linux'), `CLAUDE.md` (estado), `docs/` (fonte única); cópia da documentação…"; gate: "`git diff` só com o previsto; varredura 0".
- **O que a evidência mostra:** (a) `git diff -- CLAUDE.md AGENTS.md README.md` tem **8 inserções e 8 remoções** de **outra sessão**, hoje (troca de caminhos para `plans/concluidos/`); o CLAUDE.md está em edição fora desta revisão (índice 00, linha 14). (b) `docs/` **não** é fonte da documentação de uso (D-7). (c) O `CHANGELOG.md` já tem o precedente de entrada de infraestrutura **sem versão** (`## Servidor de IA (infraestrutura), 2026-11-08`, linhas 11-13: "sem mudança de código e sem número de versão"). (d) O Fase 12 §11.3 (linhas 575-577) lista outros documentos a atualizar em cada implantação: `docs/MANUAL-GATEWAY.md` (n/a aqui), e, fora do repositório, `CONEXAO-IA.md` e a memória do projeto. (e) A alteração de `/etc/hosts` e a árvore `/root/MCP` são **configuração do servidor**, que o repositório inventaria em `deploy/servidor-ia/` (`inventario.txt`, `README.md`).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto (texto pronto para a S7, sem editar nada agora):**

  > **S7, conteúdo por arquivo (somente na execução da S7, nunca agora):**
  > - **`CHANGELOG.md`:** na seção "Não lançado" (linha 53), uma entrada **"Fase 15: `mcpollama-mcp` no Linux (`/root/MCP`, `lx-ai01`), 2026-11-12/13"** no formato da entrada de infraestrutura de 2026-11-08: **sem mudança de código e sem número de versão** (se a regra D-5 (i) valer), com binário `0.10.3-b1faec3`, hash, o que foi validado (7 de 25 pacotes de teste, 10 jobs, PDF, cache) e o que **não** foi (OCR, 18 pacotes), os 2 testes corrigidos; se houver 0.10.4, seção `### [0.10.4]` com data/hora real, hash, tag.
  > - **`README.md`:** na tabela de versões (linha 72), acrescentar que existe uma instalação **Linux** (`/root/MCP/mcpollama-mcp`, `lx-ai01`) e uma nota "Linux" (OCR só no Windows; separador `:` no `--allow-root`; token por `OLLAMA_API_KEY`).
  > - **`CLAUDE.md`:** (i) "Comandos": a linha de build Linux **completa** (D-16 item 4); (ii) "Estado": uma frase da Fase 15; (iii) "Arquitetura alvo": `/root/MCP`. **Só depois** de a outra sessão commitar as suas mudanças (ou com ordem do owner), e sempre com `git diff -- CLAUDE.md` lido **antes** para não arrastar o trabalho alheio no commit (regra global de commit por caminho e de não arrastar frentes alheias).
  > - **Documentação de uso:** conforme QD-3 (cópias com `.bak-<data>` **fora** do `--allow-root`).
  > - **`deploy/servidor-ia/`** (`inventario.txt`/`README.md`): registrar a linha de `/etc/hosts` e a árvore `/root/MCP` como configuração do servidor (com o `Reverter` da 7.1). **Fora do repositório:** `CONEXAO-IA.md` (conferir MD5) e a memória do projeto (`MEMORY.md`).
  > - **O plano da Fase 15 e o plano mestre** (`MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`, que o CLAUDE.md manda atualizar "a cada fase"; hoje ele **não** menciona as Fases 11 a 15, `grep -c "Fase 1[1-5]"` = 0).
  > **Nota de coordenação (valor de gate):** `git status --short CLAUDE.md AGENTS.md README.md` e `git diff --stat` **antes** de qualquer `git add`; `git add <caminho>` por arquivo; `git diff --cached` conferido; 0 arquivos de outra frente no commit.

### D-18 — Regra de SSH "uma conexão por vez, com pausa" não foi observada (informativo)

- **Onde no plano:** S1 (linha 115: "Uma conexão SSH por vez, com pausa, por causa do fail2ban"), §7 (linha 134), fato 7 (linha 38).
- **O que o plano diz:** a regra vale para a S1.
- **O que a evidência mostra:** `grep -c "ssh -o BatchMode" EV` = 47 invocações em 69 minutos (21:51:10 a 23:00:50), várias em sequência de segundos (21:51:10, 21:51:20, 21:51:32, 21:55:56, 21:56:02, 21:57:30, 21:58:38 …); houve ainda um laço de até 40 tentativas de reconexão após o reinício (EV:1182). **Nenhum banimento foi registrado.** O risco do `fail2ban` segue válido; o plano não registra que a regra foi descumprida sem consequência.
- **Classificação:** confirmado (contagem por `grep`; "sem banimento" é ausência de registro, não prova).
- **Gravidade:** informativo.
- **Reparo proposto:** na entrada de erros de processo (errata/0.3): "a regra de uma conexão por vez com pausa não foi seguida (≥ 45 conexões em 69 min, sem banimento registrado); as próximas etapas agrupam comandos em **um** `ssh` por passo (padrão já usado em `Instalar-Gateway.ps1`) e usam `ControlMaster` ou um script único."

## Pendências numeradas sugeridas para a nova subseção 6.2 (cada uma com a conferência somente leitura, a executar só na etapa de execução)

| P-n | Pendência | Conferência / ação | Quem |
| --- | --- | --- | --- |
| P-1 | Aprovação formal do plano (S0) | preencher o registro da D-15 | owner |
| P-2 | Aprovação interativa do servidor MCP em `/root/treino` | `cd /root/treino && claude` (owner); depois `claude mcp list` não mostra "Pending approval" | owner |
| P-3 | Chamada normal sem `--mcp-config` | item 10 da tabela de D-10, após P-2 | sonnet |
| P-4 | Rótulo da estação no `/monitor` | leitura de `/monitor/api/state` com a chave `client_label` (D-11) | sonnet |
| P-5 | S6: `chat`, `translate`, `.docx`/`.pptx`, PDF digitalizado e imagem recusados | itens 1-6 de D-10 | sonnet |
| P-6 | `reportar_erro` de teste | QD-5 (autorização do owner) | owner + sonnet |
| P-7 | Corrigir `TestSplitRoots` e `TestWrongHashServerStarts` (e o `--help`, se QD-1 permitir) | D-4, com mutação | sonnet + opus |
| P-8 | Registrar `GOOS=linux go vet ./...` (rc=0 em 2026-11-13) como gate e repetir `go test` no Linux depois da S2 | D-2, D-3 | sonnet |
| P-9 | S3: decidir (a) manter o binário atual ou (b) reconstruir só com 0.10.4; criar `SHA256SUMS` | QD-2, D-6 | owner + sonnet |
| P-10 | S4/S4b: documentação Linux e arquivos de `/root/treino` | D-7, D-8, QD-3 | sonnet |
| P-11 | `.mcp.json.bak-windows-20261112` com token dentro do `--allow-root` | QD-4 | owner |
| P-12 | `/root/treino` é repositório git? `.mcp.json` versionado/ignorado? remoto? | comandos de D-9 | sonnet |
| P-13 | Processo órfão e `/tmp/x.bin` do comando de 23:00:50 | `pgrep -a mcpollama-mcp; ls /tmp/x.bin` | sonnet |
| P-14 | CRLF nos documentos copiados para `/root/MCP/Documentation` | `grep -rIlc $'\r' /root/MCP/Documentation` | sonnet |
| P-15 | Resolução de `ia.cslsolucoes.com.br` no servidor (origem dos 2 IPv6, ordem) | `getent ahosts ia.cslsolucoes.com.br; grep -n ia.cslsolucoes /etc/hosts /etc/nsswitch.conf` | sonnet |
| P-16 | Prova do ETXTBSY e de `readelf`/`od` no servidor | procedimento em `/tmp` (D-6); `command -v readelf od` | sonnet |
| P-17 | Cenários 2 (LAN) e 3 (internet) da documentação de rede | só se o owner quiser documentar com prova (D-12) | owner |
| P-18 | S7 e S8 | D-17, D-16 | sonnet / owner |

## Questões novas para o owner (numeração definitiva no consolidado 06; a partir da 13 no plano)

- **QD-1:** regra de versão (0.10.4 só com mudança de arquivo não de teste; D-5).
- **QD-2:** S3 (a) manter o binário `0.10.3-b1faec3` atual, ou (b) reconstruir só com a 0.10.4 (D-6).
- **QD-3:** fonte da documentação de uso (D-7).
- **QD-4:** destino do `.mcp.json.bak-windows-20261112` e backups da S4b fora do `--allow-root` (D-9).
- **QD-5:** 1 chamada de `reportar_erro` de teste na S6 (D-10).
Cada uma com opções (a recomendada primeiro), impacto, rollback e a alternativa de não fazer, como escrito nos achados correspondentes.

## Não verificado nesta etapa

- Todo estado do `lx-ai01` (a revisão não acessou o servidor): conteúdo atual de `/root/MCP`, `/etc/hosts`, `/root/treino`, processos, serviços, `/monitor`.
- Se `/root/MCP/Documentation/Ollama` está de fato em CRLF; se `/root/treino/.mcp.json` está ignorado/versionado; se `/root/treino` tem remoto.
- Se há testes de `files`/`tools` que mudam de comportamento como root; contagem de `SKIP` no Linux.
- A origem dos 2 endereços IPv6 de `ia.cslsolucoes.com.br` no servidor e se o cliente Go tenta IPv6 primeiro; o cenário 3 (internet) e se pede `--tls-ca`.
- O comportamento de troca de binário em uso (ETXTBSY) e a disponibilidade de `readelf` no servidor.
- Os 17 pacotes (`cmd/*`, `gateway/*`) **executados** no Linux (só compilação coberta pelo `vet`); `-race` em código `!windows`.
- Se o rótulo `ia.cslsolucoes.com.br/root` aparece de fato em `client_label` no `/monitor` (inferido do código e do hostname).
- A reprodutibilidade de dois builds com `-trimpath` (não executada; só `go vet` e `go list` rodaram).

## Fora do escopo do plano (só reportar)

1. **Denylist do guard não cobre variantes de backup** de arquivos sensíveis (`.mcp.json.bak*`, `settings.local.json.bak`, `.mcp.json~`): `src/files/policy.go:101-119`. Decisão de produto; mexer em `src/files/` exige os testes de ataque da Fase 4. Reportado, não corrigido.
2. **CLAUDE.md, linha 84:** build Linux sem `-ldflags -X main.Version`, sem `CGO_ENABLED=0`/`GOARCH`; o binário atual foi gerado por comando não documentado. O CLAUDE.md está em edição por outra sessão: **não tocar**.
3. **`nvidia-powercap` do servidor** foi alterado em 2026-11-12 18:12 para clock de fábrica e offsets 0/0 (EV:1232-1241), enquanto `CLAUDE.md`, `CHANGELOG.md` e `deploy/servidor-ia/nvidia-powercap.service` ainda descrevem +1500/+150 (já anotado pelo plano na entrada 22:45; é do `deploy/servidor-ia/`, não da Fase 15).
4. **`/root/.bashrc` do servidor** exporta `CLAUDE_CODE_OAUTH_TOKEN` (alterado pelo owner às 22:25:13 e 22:47:12): é credencial em arquivo de shell, decisão do owner (etapa 04).
5. **O plano mestre** (`MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`) não registra as Fases 11 a 15, contra a regra "atualizar o plano a cada fase" do CLAUDE.md.
6. **Documentos de uso do Windows** (`D:\MCP\Documentation\Ollama`): `01-pre-requisitos-e-copia.md:68` ainda cita `0.10.2-09e2936` como "esperado" (diz "ou superior", mas a versão atual é a 0.10.3); fora do repositório e fora do escopo desta fase.
7. **Ação de 23:00:50 da 1ª revisão** fez uma conexão ao servidor e criou/apagou `/tmp/x.bin` sem registro no plano (D-13), **depois** de o owner pedir apenas "faça uma revisão do plano" (`TL:106`, 22:59:55): a leitura do `cloudflared`/nginx cabia no pedido, a cópia do binário e a tentativa de subir um processo, não. Reportado como erro de processo, sem correção.

## Verificação adversarial (opus, 2026-11-13 12:03:08)

**Verificador:** Claude Code (Opus 5.5). **Início:** 11:56:40 (`date`). **Sem acesso ao servidor**, sem `mcp__mcpOllama__*`, sem git que escreva. Fontes primárias relidas: o PLANO inteiro; `TL` inteiro; `EV:280-709`, `EV:780-1429`; `src/files/policy.go:60-147`, `src/files/guard.go` (uso de `Denied`, linhas 349 e 368), `src/files/guard_test.go:120-131` e todos os `t.Skip` dos 7 pacotes, `src/mcpollama-mcp/config.go:138-149,498-537`, `ocr_test.go:160-190`, `config_test.go:190-198`, `src/ocr/ocr.go:79-82`, `src/doctext/doctext.go:215-231`, `src/gateway/queue/queue.go:1125-1135`, `src/gateway/store/store.go:650-690`, `src/ollama/security_test.go:212-235`, `src/tools/registry_test.go:169,222-237`; `CHANGELOG.md:1-14,53,73,81`; `README.md:70-74`; `CLAUDE.md:80-91`; rule `workspace-plans-persist_V1.4.0.mdc:145-158`; plano da Fase 12 (linhas 515-518, 535, 549-550, 562, 575-577, 584); `docs/RELATAR-ERROS-IA.md:40-46`; `docs/CONTRATO-DADOS-TREINO.md:48-53`; `D:\MCP\Documentation\Ollama\*.md` (recontagem); `git blame`, `git tag`, `git log b1faec3..HEAD -- <pacotes do MCP>`, `git status`. Medições repetidas na estação (somente leitura): `go list ./...` = 25; `find -name testdata` vazio; `ls src/mcp/*_test.go` falha; `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` **rc=0** (~11:58); `gofmt -l .` **0 arquivos**; CRLF 8/8 arquivos em todas as linhas; contagens de `D:\` 50, `E:\` 1, `C:\` 5, `.exe` 30, `powershell` 13 (sem distinguir caixa).

**Segredo no `.md` desta etapa:** varrido (o início conhecido do token OAuth inválido, sem reproduzi-lo aqui; `sk-ant`; `Bearer <valor>`; sequências hex ≥ 40, sequências `[A-Za-z0-9_-]` ≥ 30). **Nenhum token nem trecho de token.** Só aparecem o SHA-256 do binário (`d61f5e82…0333`, público) e o SHA-256 da CA **pública** Cloudflare Origin (`01d12a7c…d8f3`); nada redigido.

### Tabela achado → veredito → evidência → ajuste do reparo

| Achado | Veredito | Evidência conferida | Ajuste do reparo |
| --- | --- | --- | --- |
| D-1 Estado por etapa | **CONFIRMADO, com 5 correções** | S1(a) `EV:291-299`; S1(b) extraído **em** `/root/MCP` (`EV:305`); 7 de 25 pacotes (`EV:1019-1033`); S6 `EV:1151-1154` | (1) **S5 "concluída" exagera:** o gate da S5 pede "hash igual ao da S3" e a S3 não existiu; o hash conferido foi contra `D:\MCP` (`EV:307,316`) e não há `SHA256SUMS`. Estado: **"fora de ordem; gate cumprido em parte"**. (2) **Questões 4, 5 e 6 também foram decididas de fato** (alvo `x86_64`, `EV:295`; OCR e `tessdata` não copiados; gateway não copiado, `EV:1225`): listar na célula da S0 como "decididas por ação, a confirmar". (3) **Questão 12:** "ok, commit tudo e push" (`TL:96`) autorizou 1 commit de **documentação** no `master`; não decide o ramo do **código** (S2). Trocar "decidida por ação, contra a recomendada" por "precedente: commit de documentação no `master` com push autorizado; o ramo para código segue aberto". (4) **`TL:48` (22:03:14) é a pergunta (`ASK`) do Claude**, não o pedido do owner; a resposta do owner está entre 22:03:14 e 22:04:18 e não aparece na linha do tempo. (5) **S6, "chamada normal sem `--mcp-config`"**: foi **tentada** às 22:25:28 e falhou por autenticação (401, `EV:1136-1146`); depois do token válido (22:47) só se testou `claude -p "ok"` (`EV:1383`) e o servidor seguia `Pending approval` (`EV:1385`). Escrever "tentada, falhou por autenticação; não repetida". |
| D-2 Gate da S1(c) | **CONFIRMADO** nos itens 1, 2, 3, 5, 6, 7; **item 4 REFUTADO** | `EV:797-798`, `EV:980-1033`, `EV:1045`, `EV:1066`; `go list` 25; sem `testdata`; `src/mcp` sem testes. Item 4: `grep Chmod\|Getuid\|Geteuid` nos testes de `files`, `tools`, `doctext`, `mcpollama-mcp` = **0**; os `t.Skip` são de hardlink, symlink, junction, 8.3, Windows e `live` | (a) **Retirar o passo 5** ("repetir como usuário sem privilégio"): não há teste dependente de permissão, e a memória do projeto registra **só** o alias `lx-ai01-root` (não existe "alias `lx-ai01`"); rodar como outro usuário exigiria criar usuário no servidor de produção (fora do escopo). (b) **Defeito no passo 2** (ver D-V-1). (c) Acrescentar ao gate: `ls src/<p>/*.go | wc -l` > 0 em cada pacote antes de executar (ver D-V-5). (d) Usar `-test.v` (a rodada de 2026-11-12 **não** usou, `EV:1019`, por isso os `SKIP` de então são desconhecidos; dizer isso na 6.1). |
| D-3 `go vet` cruzado | **CONFIRMADO** (remedido às ~11:58: rc=0, saída vazia; `gofmt -l .` vazio) | Fase 12 linhas 549-550 | Nenhum. |
| D-4 S2, testes e `--help` | **CONFIRMADO** | `config.go:144-145` (`git blame` → `9ef7935`, 2026-10-23 22:08:30); `03-parametros.md:47`; `02-mcp-json-do-zero.md:24`; `guard_test.go:122-130`; `ocr_test.go:164-178,184-185`; `ocr.go:81`; `EV:1041,1047` | **Moldura:** a errata do changelog 22:45 não pode "acrescentar `[errata E-n]` **com** o texto": na entrada antiga entra **só** o marcador `**[errata E-n]**`; a explicação vai para a **9.1 Errata** (moldura, item 4). |
| D-5 D15-1 × S2 | **CONFIRMADO** em (a), (b), (c); **(d) REFUTADO** | `git log b1faec3..HEAD -- src/<pacotes do MCP>` vazio; `git tag`: última `mcpollama-v0.10.3`; `CHANGELOG.md:53,73`; "0.10.4" só no PLANO e nesta revisão | (d) diz que "a questão 3 foi respondida de fato como 'sem execução' (`TL:19`)". **Errado:** "ok, 1 e 2" respondeu as duas perguntas daquele `AskUserQuestion`, que o próprio plano mapeia para as **questões 10 e 8** (changelog 21:58, linha 173). A questão 3 segue **aberta**. Retirar (d) do texto do reparo. |
| D-6 S3, hash, `file`, ETXTBSY | **CONFIRMADO**; ETXTBSY **NÃO VERIFICÁVEL SEM O SERVIDOR** (semântica POSIX, não medida) | `EV:309-314` (sem `-trimpath`), `EV:318` (`file` existe e rodou no servidor), `EV:1089-1098` (sem `SHA256SUMS`), `EV:1428-1429` (teste de 23:00:50 não prova nada); `D:\MCP` tem `mcpollama-mcp.bak-0.10.1` e `.bak-0.10.2` (convenção confirmada) | Nenhum. |
| D-7 Documentação fora do repositório | **CONFIRMADO** (recontado: 95/95, 264/264, 145/145, 245/245, 83/83, 87/87, 88/88, 52/52 linhas com `\r`; `docs/` com 5 arquivos) | `git ls-files` sem `Documentation/Ollama`; Fase 14 (concluidos) linha 100 | CRLF em `/root/MCP` segue **inferido** (cópia por `tar`, `EV:305`). |
| D-8 Mapeamento e gate da S4/S4b | **CONFIRMADO** (contagens idênticas às minhas; `node` ausente, `EV:608-610`; `python3` presente, `EV:615`; `.claude` = `agents commands plans`, `EV:630-632`) | — | **O gate redigido tem 2 defeitos** (D-V-2): `-F '.exe'` casa com `mcp.json.exemplo` (falso FAIL) e `powershell` sem `-i` não pega as 3 linhas `PowerShell` (falso OK). Usar a redação de D-V-2. `grep -rIlc` → `grep -rIl`. |
| D-9 `.bak` com token dentro do `--allow-root` | **CONFIRMADO** (item 1 e 3) — **crítico mantido**; item 2 **NÃO VERIFICÁVEL SEM O SERVIDOR** | `policy.go:101-119` (igualdade `.mcp.json`; `.mcp.json.bak-windows-20261112` não casa com nenhuma regra, mesmo após `normComponent`, `policy.go:77-79`); `guard.go:349,368` aplicam a lista a todos os segmentos; `EV:639,673` (`.bak` criado, `0600`); `EV:633` (`.git` é diretório: `ls -la` imprimiu `total 4076`) | (a) Item 2: o comando de `EV:649` usou `2>/dev/null`, então a mensagem "(sem repo git ou nao ignorado)" também cobre **`git` não instalado no servidor**, que nunca foi medido. Pré-checagem a acrescentar: `command -v git; echo rc=$?`. (b) **Local dos backups:** `/root/MCP/backups/` é ruim, porque o smoke test da S1(b) rodou com `--allow-root /root/MCP` (`EV:368`); backup de `.mcp.json` ali fica legível pela ferramenta. Usar `/root/backups/fase15-AAAAMMDD/` (`0700`) e proibir `--allow-root /root/MCP` (ver D-V-3). (c) **Moldura:** "substitui" vira marca `**superado em 2026-11-13 (revisão):**` na célula da S4b, sem apagar o texto. |
| D-10 S6 incompleta | **CONFIRMADO** (8 tools em `EV:378`; 5 exercitadas) | `doctext.go:217,230`; `RELATAR-ERROS-IA.md:42-44` (sem procedimento de teste); `CONTRATO-DADOS-TREINO.md:48-53` | Item 10 da tabela: estado de partida = "tentado às 22:25:28, 401" (`EV:1136-1146`), não "não feito". |
| D-11 Rótulo no `/monitor` | **CONFIRMADO** (valor no `/monitor` **NÃO VERIFICÁVEL SEM O SERVIDOR**) | `config.go:512,528`; `queue.go` `Item.ClientLabel json:"client_label"`; `store.go` idem; `EV:1300` (chaves procuradas sem `client_label`); `EV:1415` (hostname = FQDN) | `recent` tem 50 itens e **sobreviveu ao reinício** (o exemplo de `EV:1306` é um `extract`, de antes do boot de 22:28); um dia depois, os 10 `generate` podem ter saído da lista. Gate: "≥ 1 item com `client_label == ia.cslsolucoes.com.br/root`; se os 10 jobs não estiverem mais em `recent`, gerar 1 `generate` curto e ler de novo". |
| D-12 3 cenários de rede | **CONFIRMADO** para o cenário 1; 2 e 3 não medidos | `EV:441-443`, `EV:450-461`, `EV:490-499`, `EV:1224` | Origem dos IPv6: ver D-V-4 (quase certamente `nss-myhostname`, não DNS). |
| D-13 Inventário 4.5 | **CONFIRMADO**; processo órfão **NÃO VERIFICÁVEL SEM O SERVIDOR**, mas improvável | modos em `EV:321-338,386-392,432-434,528-536,672-673`; `EV:1355,1380` (`.bashrc` 22:25:13 e 22:47:12) | Nota: com `ssh -o BatchMode` não há terminal de controle, então `</dev/tty` falha **antes** do `exec` (o binário nem sobe). Manter a pendência P-13 como conferência barata. Acrescentar ao inventário: `VERSION` contém o texto `D:\MCP\mcpollama-mcp` (`EV:347`, ver D-V-6). |
| D-14 Rollback 7.1 | **CONFIRMADO**, com 1 correção | `EV:490-493` (3 linhas) | **Impacto de reverter o `/etc/hosts` exagerado:** antes da edição o `curl` pelo nome **já passava** (HTTP 401, `ssl_verify=0`, `EV:443`). Reescrever: "o nome volta a resolver pelos endereços locais da máquina (provável `nss-myhostname`, D-V-4); em 2026-11-12 21:56 isso funcionou; perde-se só a garantia de loopback IPv4". |
| D-15 Definição de Pronto | **CONFIRMADO**, com 2 correções | rule `workspace-plans-persist_V1.4.0.mdc:151` (Critérios de conclusão, obrigatória) | (1) A rule lista **Rollback** como seção **opcional** (linhas 153-155), não obrigatória; a exigência de rollback vem da regra global do owner (backup e reversão antes de tocar). (2) "executou 6 das 9 etapas": são **10** (S0–S8 + S4b); **tocou** 6 (S0, S1, S4, S4b, S5, S6), e só a S5 foi feita por inteiro (com o gate parcial de D-1). O registro de aprovação fica melhor na **0.2 Situação atual** (a 0.3 é das ações já executadas). |
| D-16 Referências, S8, CLAUDE.md | **CONFIRMADO** (itens 1 a 4) | Fase 12: §11 linha 515, §11.1 linha 517, push/tags linha 535, armadilha CR linha 584; `CHANGELOG.md:4-5`; `CLAUDE.md:84` | (a) O CR do token **não** veio de here-string: veio do `print` do Python do **Windows** (`\r\n`) num pipe para o `ssh` (`EV:542-550`). Mesma família da armadilha da linha 584, mecanismo diferente: escrever "da mesma família". (b) **Moldura:** "trocar 'tag e release criadas'" vira marca `superado em 2026-11-13 (revisão): só tag anotada (CHANGELOG.md:4-5)`, sem apagar. |
| D-17 S7 e CLAUDE.md de outra sessão | **CONFIRMADO** (`git diff --stat -- CLAUDE.md AGENTS.md README.md`: 3 arquivos, 8+/8−; `CHANGELOG.md:11-13`; `deploy/servidor-ia/inventario.txt` existe; plano mestre: 0 menções a "Fase 1[1-5]") | — | Acrescentar o precedente da **tag anotada de infraestrutura `servidor-ia-2026-11-08`** (`git cat-file -t` = `tag`): vira questão (QV-4), não decisão da revisão. |
| D-18 Regra de SSH | **CONFIRMADO** (47 linhas com `ssh -o BatchMode`; a `#46` foi bloqueada e não rodou, `EV:1193`; o laço do reinício, `EV:1182`, abriu várias conexões numa linha) | — | Nenhum. |

**Contagem:** 18 achados conferidos; **18 confirmados no essencial**, nenhum refutado por inteiro; **2 subitens refutados** (D-2 item 4; D-5 (d)); **4 subitens não verificáveis sem o servidor** (D-6 ETXTBSY, D-9 item 2, D-11 valor no `/monitor`, D-13 processo órfão), mais a origem dos IPv6 (D-V-4). **Crítico mantido:** D-9.

### Achados novos (D-V-n)

- **D-V-1 — O `tar` único do procedimento de D-2 não produz o layout que ele mesmo descreve (importante).** `tar cf - --exclude='*.exe' -C src tools … mcp -C .. TOOLS.md` grava `tools/…` e `TOOLS.md` no **mesmo** nível do arquivo; extraído num só diretório, ou o `TOOLS.md` fica em `src/` (e `../../TOOLS.md` a partir de `src/tools` não acha), ou os pacotes ficam fora de `src/`. A rodada de 2026-11-12 contornou isso com **dois** `tar` e uma cópia separada (`EV:1003-1004`). Redação correta, a partir da raiz do repositório: `tar cf - --exclude='*.exe' -C E:/MCPOllam src/tools src/files src/doctext src/ollama src/redact src/mcpollama-mcp src/ocr src/mcp TOOLS.md -C <scratch>/lt .` e, no destino, `mkdir -p /tmp/fase15-lt && tar xf - -C /tmp/fase15-lt` (dá `src/<p>/`, `src/mcp/`, `TOOLS.md` e `<p>.test` no mesmo `/tmp/fase15-lt`). Conferir com `ls /tmp/fase15-lt/TOOLS.md /tmp/fase15-lt/src/mcp/types.go` antes de executar.
- **D-V-2 — O gate redigido da S4b (D-8) dá falso FAIL e falso OK (importante).** `-F -e '.exe'` casa com `mcp.json.exemplo`, que a documentação Linux vai citar; `-e 'powershell'` sem `-i` não pega `PowerShell` (3 das 13 linhas nos documentos atuais). Redação proposta: `grep -rIl -i -E 'D:\\|E:\\|C:\\|[CDE]:/|qwen3-ia|\.exe\b|powershell|\$env:|libtesseract|USERPROFILE' /root/MCP/Documentation /root/MCP/mcp.json.exemplo <alvos de /root/treino> | grep -v -E '\.bak-|\.transcricao\.md$|\.csv$'` = 0 linhas; CR: `grep -rIl $'\r' <alvos>` = 0 linhas. Conferir o gate **contra um arquivo de controle** (um `.md` com `D:\` e outro com `mcp.json.exemplo`) antes de confiar nele.
- **D-V-3 — O lugar proposto para os backups e um precedente perigoso de `--allow-root` (importante).** O smoke test da S1(b) rodou com `--allow-root /root/MCP` (`EV:368`). Hoje isso é inofensivo porque `mcpollama.env` (sufixo `.env`) e `certs/*.pem` estão na lista fixa (`policy.go:107-111`), mas **não** estaria inofensivo se os backups de D-9 (`/root/MCP/backups/…/.mcp.json.bak-*`, com token) fossem para lá. Texto pronto: "backups da S4b em `/root/backups/fase15-AAAAMMDD/` (`0700`), fora de `/root/MCP` e de `/root/treino`; nenhuma etapa usa `--allow-root /root/MCP` nem `--allow-root /root`".
- **D-V-4 — Os 2 IPv6 do nome no servidor quase certamente vêm do `nss-myhostname`, não do DNS (informativo; NÃO VERIFICÁVEL SEM O SERVIDOR).** O `hostname` do servidor é o próprio FQDN (`EV:1415`), o `/etc/hosts` não tinha o nome antes da edição (`EV:450-451`) e um dos endereços é **`fe80::…` (link-local)**, que DNS público não publica (`EV:442`). Consequência: sem a linha do `/etc/hosts`, o nome já resolve para a própria máquina (e o `curl` passava, `EV:443`); a linha só força o loopback IPv4. Conferência: `grep ^hosts /etc/nsswitch.conf`. Afeta D-12 (cenário 1), D-14 (impacto do rollback) e a documentação Linux. A decisão de manter a linha é do owner (QV-3).
- **D-V-5 — Um teste passa "no vazio" se as fontes do pacote não forem copiadas (informativo).** `tools/registry_test.go:224-233` (`TestArgumentHelpersLiveInOneFile`) ignora o erro de `os.ReadDir(".")`: sem os `.go` no diretório, passa sem checar nada. (`ollama/security_test.go:231-233` se protege com `t.Fatal("no source files found")`.) O gate da S1(c) precisa conferir que as fontes estão lá (D-2, ajuste c).
- **D-V-6 — `VERSION` em `/root/MCP` contém o texto `D:\MCP\mcpollama-mcp` (`EV:347`) (informativo).** É proveniência legítima; o gate de D-V-2 não o inclui, e não deve incluir sem exceção declarada.
- **D-V-7 — O insumo `recuperado/evidence-exec.txt` contém, sem redação, um trecho de 13 caracteres do token OAuth inválido (linha 1370, rótulo `prefixo=`) (importante; fora do plano).** O índice (moldura, item 11) proíbe reproduzi-lo; ele **não** foi reproduzido neste `.md`. Sugestão ao coordenador: redigir essa linha no scratchpad antes de passar o arquivo a outros agentes. Não alterei o insumo (não pedido).
- **D-V-8 — Os reparos D-4, D-9 e D-16 violam a moldura em pontos de redação (importante para a etapa 06).** D-4 põe texto dentro de entrada antiga do changelog; D-9 "substitui" e D-16 "troca" texto vigente. Já ajustados na tabela acima; a etapa 06 deve aplicar só com marcas `superado em 2026-11-13 (revisão)` e erratas na 9.1.
- **D-V-9 — Precedente de tag de infraestrutura sem versão (informativo).** Existe a tag anotada `servidor-ia-2026-11-08`. Se a Fase 15 ficar sem versão (QD-1), as mudanças de servidor (`/etc/hosts`, `/root/MCP`) podem ganhar uma tag `servidor-ia-2026-11-12`, ou nenhuma: decisão do owner (QV-4).

### Perguntas fechadas para o owner geradas por esta etapa (a recomendada primeiro; numeração definitiva no 06)

1. **QD-1 Versão:** (a) **só testes mudam → sem nova versão, binário `0.10.3-b1faec3` fica** (recomendado); (b) publicar 0.10.4 mesmo só com testes. Impacto de (b): trocar binário em `D:\MCP` e `/root/MCP` e reiniciar sessões. Rollback: `.bak-0.10.3`.
2. **QD-1b `--help` do `--allow-root`:** (a) **não mexer** (a linha 145 já diz `: on Linux`; corrigir só a documentação) (recomendado); (b) reescrever a 1ª linha e lançar 0.10.4 + tag.
3. **QD-2 S3:** (a) **manter o binário atual, registrar que não segue a §4.2 e criar `SHA256SUMS` dele** (recomendado); (b) reconstruir com `-trimpath -s -w` só junto com a 0.10.4. Nunca reconstruir "0.10.3".
4. **QD-3 Fonte da documentação:** (a) importar para o repositório (`docs/…`) e gerar as cópias (recomendado pelo analista); (b) `D:\MCP\Documentation\Ollama` como fonte + script de conversão; (c) duas cópias à mão.
5. **QD-4 `.mcp.json.bak-windows-20261112` (token, dentro do `--allow-root`):** (a) **mover para `/root/backups/fase15-20261112/` (`0600`, fora de qualquer raiz)** (recomendado, ajustado por D-V-3); (b) apagar após conferir o `.mcp.json` novo; (c) manter. Rollback de (a): `mv` de volta.
6. **QD-5 `reportar_erro` no Linux:** (a) 1 registro de teste marcado `[TESTE Fase 15, descartar]`, sem `resposta_correta`, descartado pelo admin (recomendado pelo analista); (b) não testar e registrar "presente, não exercitada".
7. **QV-1 Regularização (S0):** aceita registrar a S5 e os itens da S1/S4/S4b/S6 como "feitos fora de ordem por ordem sua", com o gate parcial (sem S3, sem `SHA256SUMS`), na 0.3? (a) **sim** (recomendado); (b) não, refazer pela ordem.
8. **QV-2 Pré-checagens somente leitura no servidor antes da S4b** (`command -v git`, `git -C /root/treino status/check-ignore/ls-files/remote -v`, `grep ^hosts /etc/nsswitch.conf`, `pgrep -a mcpollama-mcp`, `ls /tmp/x.bin`): (a) **autorizar 1 conexão SSH com esses comandos** (recomendado); (b) não acessar o servidor agora.
9. **QV-3 Linha do `/etc/hosts`:** (a) **manter** (força loopback IPv4; já funciona 10/10) (recomendado); (b) medir a resolução sem ela (D-V-4) e decidir depois.
10. **QV-4 Tag de infraestrutura** para as mudanças do servidor na Fase 15: (a) **nenhuma** (recomendado, se não houver versão); (b) `servidor-ia-2026-11-12`, como o precedente de 2026-11-08.
