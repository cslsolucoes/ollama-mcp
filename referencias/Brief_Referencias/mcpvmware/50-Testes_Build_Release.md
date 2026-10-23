# MCPVMWare — testes, build, release e geração de catálogo

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Por restrição do coordenador, **nada foi compilado, testado nem executado dentro de `referencias/`**; as execuções abaixo usaram uma cópia de `MCPVMWare/src` na pasta temporária da sessão.

## 1. Padrão de testes

**Números (medidos com `grep`/`wc`):** 86 arquivos `_test.go`, 29.380 linhas, 485 funções `Test*`, ~489 chamadas `t.Run`. 70 arquivos de `tools/` usam o simulador `vcsim`; 16 arquivos usam `httptest`.

**Duas famílias de dublê:**

| Backend | Dublê | Onde |
| --- | --- | --- |
| vSphere (SOAP) | `vcsim` em processo (`simulator.ESX()` / `simulator.VPX()`), cliente real passando por `vmware.NewClient` (login + keepalive de verdade) | `tools/testhelpers_test.go:96-172` |
| REST/VAPI | `vapi/simulator` (import em branco, `RegisterEndpoints=true`) ou `httptest` puro | `testhelpers_test.go:13`, `cerebrum.md:333-350, 653-662` |
| Workstation, VMC, VAMI legado | `httptest.Server` com fixtures | `workstation/client_test.go`, `cloudaws/client_test.go`, `tools/appliance_test.go` |

**Regras do padrão:**

1. **Chamar pelo `Registry.CallTool`, nunca pelo handler** (`testhelpers_test.go:90-95`), então o teste exercita registro, gate, filtro de modo e `recover`.
2. **Um arquivo de teste por domínio**, com um simulador compartilhado por arquivo e `t.Run` por tool (`cerebrum.md:484-499`).
3. **Três camadas para as centenas de tools** (plano de codegen): smoke universal (registra, schema válido, argumento obrigatório ausente dá erro limpo, sem panic), smoke funcional para leituras contra `VPX()`, e teste dedicado para toda tool destrutiva e as mais usadas (`plano-cobertura...plan.md:515-527`).
4. **Teste de conjunto exato por modo** (`tools/mode_test.go:1165-1193`, 1.289 linhas): `assertExactToolSet` compara os dois conjuntos e a contagem. Adicionar uma tool obriga a atualizar a lista (`cerebrum.md:89-94`). As tools meta são somadas num só lugar (`:1185-1188`).
5. **Teste de gate sem backend** (`destructive_test.go:39-117`): registra uma tool fictícia destrutiva e prova que o handler **não rodou** quando o gate está fechado ou o `confirm` não é booleano exato (`{"confirm":"true"}` e `{"confirm":1}` negados).
6. **Teste do que se omite, em corpus real** (`discovery_test.go:49` `TestFirstSentence_CorpusReal`: as 1.034 descrições reais; `:150` `TestListTools_CompactPreservaEstrutura`: `type/required/enum` e nº de propriedades iguais em >100 tools; `:139` `TestListTools_OmissaoEIgualAFull`; `:285` `describe_tool` completo mesmo em compact).
7. **Teste de regressão que prova a si mesmo:** a primeira versão do teste de `null` vs `[]` passava **com o defeito presente** (o `vcsim` devolve slice vazio não-nil); foi trocada por um teste que chama o helper com `nil` direto (`buglog.json`, bug-014).

**Executado nesta onda (em cópia, `-count=1`):** `go test ./tools -run '<discovery|destructive|modo|profile|describe>' -count=1` → `ok ... 1.440s`; `go vet ./mcp ./tools` limpo; `gofmt -l` do projeto limpo (o único arquivo listado foi o `cmd/measure/main.go` criado por mim na cópia). **Não** rodei a suíte completa: o próprio projeto documenta que ela leva ~90 s só em `tools` e é intermitente no Windows (§2).

**O que a suíte não cobre:** o loop stdio (`mcpvmware-mcp` não tem teste; o smoke é por script), e tudo que o `vcsim` não simula. Das 1.047 tools, 42 foram provadas contra ESXi 8.0.3 (leitura) e 61 contra ESXi 7.0.3 (leitura e escrita autorizada); contra vCenter, 0 (`README.md:37-48`).

## 2. Intermitência da suíte no Windows (e o que se fez)

Diagnosticada em 2026-08-22 (`.wolf/buglog.json`, bug-013; plano de ambiente D1). Sintoma: `failed to connect to simulator: dial tcp 127.0.0.1:15xx` em testes **diferentes a cada corrida** (interseção vazia entre duas corridas), contradizendo a hipótese de "6 testes com bug".

- **1ª causa investigada e refutada:** race de arranque do listener. **Evidência decisiva:** `netstat` no instante da falha mostra a porta em `LISTENING` no PID do próprio teste, e o `connect()` falha em ~1,000 s exatos (WSAETIMEDOUT); retentativa imediata funciona; em isolamento 60 iterações = 0 falhas.
- **Causa real:** artefato de ambiente (Windows Defender/firewall descartando SYNs de loopback sob rajada de handshakes **TLS**). Experimento com variável isolada: com TLS, 1 de 2 corridas falhou; sem TLS, 2 de 2 passaram.
- **Mitigação na camada certa:** o simulador sobe em **HTTP puro por padrão** (`MCPVMWARE_SIM_TLS=1` religa), mais `connectSimWithRetry` (6 tentativas, só para erro de ligação via `errors.As(*net.OpError)`, `testhelpers_test.go:53-88`), e um teste dedicado que força TLS (`:174`). **Não** se usou `t.Skip` nem `-p 1` para esconder. Ferramenta de diagnóstico atrás da *build tag* `diag` (`zz_diag_harness_test.go`).
- **Lição de método** que o próprio projeto registra: declarar resolvido com 3 corridas verdes foi evidência insuficiente; só a experiência com variável isolada deu a causa (`cerebrum.md:48-54`).

## 3. Build

- **Módulo:** `src/go.mod` (`module github.com/cslsoftwares/mcpvmware`, `go 1.25.0`). Entrypoint em `src/mcpvmware-mcp/`.
- **Comandos** (`README.md:197-214`, `ci.yml:47-64`): `go build ./...`, `go vet ./...`, `gofmt -l .` (CI falha se listar algo), `go test ./...`.
- **Binário:** `CGO_ENABLED=0`, `go build -trimpath -ldflags "-s -w -X main.Version=<v>" -o <saida> ./mcpvmware-mcp` (`tools/Build-Release.ps1:78, 93-96`). Medido: o binário de `dev` sem `-s -w` tem 28.289.024 bytes (cópia); o release v0.1.0 registrado tem 20,6 MB (`plano-ambiente...plan.md`, §D-Release).
- **Cross-compile:** não faz. **Windows-only por decisão do owner (DEC-14, 2026-08-23)**: o parâmetro `-Targets` aceita só `windows-amd64` (`Build-Release.ps1:40`), e o CI só tem `windows-latest` (`ci.yml:31-35`). Nota: o `CLAUDE.md` do mcpollama prevê binário Linux; essa decisão é do mcpollama.
- **Cacheado e versionado indevidamente:** `src/gen.exe` e `src/mcpvmware-mcp.exe` (29 MB) estiveram *tracked* apesar do `.gitignore` (plano de ambiente D4); ainda existem na pasta (`ls src` mostra os dois).

## 4. Versão e release

- **Variável:** `var Version = "dev"` (`mcpvmware-mcp/main.go:65`); `--version` imprime `mcpvmware-mcp version <Version>` (`:128-131`) e `initialize` devolve a mesma em `serverInfo.version` (`:374`).
- **Derivação** (`Build-Release.ps1:52-66`): sem `-Version`, usa `git describe --tags --always --dirty`; se o resultado for só um hash (sem tag), usa `0.0.0-dev+<sha>` com sufixo `.dirty` se sujo. **O `-dirty` aparece de fato:** `git describe` na referência devolveu `v0.1.0-11-g26490a2-dirty` (executado antes da restrição de `git` do coordenador; ver nota no fim). A causa que o projeto registrou: arquivos de estado do OpenWolf (`.wolf/hooks/_session.json`, `_precompact-snapshot.json`, `token-ledger.json`) são versionados mas regeneram a cada sessão, então a árvore **nunca** está limpa; contornado com `git stash` desses três na hora do build; **causa não resolvida (P-11)**.
- **Saída:** `dist/<versao>/mcpvmware-mcp-<versao>-windows-amd64.exe` + `SHA256SUMS.txt` (formato de `sha256sum -c`, `:118-126`).
- **O script se verifica:** executa o binário com `--version` e falha se não contiver a versão injetada (`:131-142`).
- **Não cria nem publica tags** (`:17-18`); a tag `v0.1.0` foi decisão do owner/assistente documentada (anotada, escolhida em vez de `1.0.0` porque só ~15% das tools servíveis por ESXi estavam provadas, `plano-ambiente...plan.md` §D-Release).
- **Regra de `.ps1` ASCII puro** (`tools/README.md:126-129`): o PowerShell 5.1 lê arquivo sem BOM como ANSI; 3 travessões num comentário geraram 10 erros de sintaxe enganosos. `-Encoding utf8` grava BOM e o `go.mod` rejeita BOM; usar `ascii` (`:130-131`).

## 5. CI

`.github/workflows/ci.yml` (93 linhas): job **bloqueante** `padrao` (`build`, `vet`, `gofmt`, `go clean -testcache` + `go test ./...` "a frio, sem serializar", `:25-71`); job **informativo** `race` com `continue-on-error: true` (`:79-93`). `concurrency` cancela execuções antigas (`:19-21`), `permissions: contents: read`.

**O CI foi provado por um teste negativo** (STATUS 2026-08-23): um bug deliberado (retirar `normalizeNilSlices`) fez o job falhar **exatamente no passo `go test`**, com `build`/`vet`/`gofmt` passando antes: "nesta classe, build e vet não chegam". Revertido, 3/3 verde.

## 6. Mecanismo `src/gen` → `generated_*` → `TOOLS.md` (em detalhe)

> **Resumo honesto: não existe um pipeline único `gen → generated_* → TOOLS.md`.** Existem **três coisas separadas**, e confundi-las é o risco ao copiar o desenho.

### 6.1 `src/gen` é um classificador de uso único (Fase 0)

- **Entrada:** o código do `govmomi` em `referencia/govmomi` (checkout local), pastas `object/` e `vapi/*` (recursivo, pulando `internal` e `simulator`). Argumento opcional = raiz; padrão `../referencia/govmomi` (`gen/main.go:217-221`). Executado com `go run ./gen` dentro de `src/` (`:13`).
- **O que faz:** percorre os `.go` com `go/parser`, pega todo método **exportado, com receptor, cujo 1º parâmetro é `context.Context`** (`parsePackageDir`, `:365-421`; `firstParamIsContext`, `:423-434`), e para cada um calcula: tier por regex sobre o **nome** do método (`tier1Patterns` > `tier2Patterns` > `readOnlyPatterns` > `tier1VerbAnywhere` > padrão `tier2` "fail-safe", `classify`, `:523-542`), modo (`vcenter-only` se o arquivo está em `vcenterOnlyFiles` ou o pacote é `vapi/*`; senão `vsphere-general`, `:410-416`) e **nome proposto** `vmware_<domínio>_<ação>` (`:544-555`).
- **Saída:** `gen/classification.json` (527 métodos) e `gen/CLASSIFICATION_REPORT.md` (`writeJSON`/`writeMarkdown`, `:558-611`). Como o próprio cabeçalho diz: **"It does NOT generate any tools/*.go file"** (`:9-11`); a geração era a Fase 1+, "gated on a human reviewing this report first".
- **Salvaguardas:** exclui nomes já escritos à mão e **anuncia** a exclusão em stdout (`existingHandWrittenTools`, `:160-190`, `:252-271`); avisa colisão de nome proposto (`:273-285`).
- **Limites reconhecidos:** (a) só vê métodos com *wrapper* `object.*`; a superfície SOAP crua (iSCSI, FT, KMIP...) ficou invisível e teve de ser escrita à mão em ondas (`cerebrum.md:74-88`); (b) uma nota do plano dizia que o gerador "pula nomes já registrados"; era falso, nunca implementado, achado por `grep` vazio (`cerebrum.md:571-581`), e só depois implementado (hoje existe, `:252-271`); (c) `vcenterOnlyFiles` e o tier erraram em ambas as direções (`:112-139`); (d) **não formata a saída** (`ci.yml:61-62`).

### 6.2 Os `generated_*` foram escritos por subagentes, não por template

- **Prova:** (i) `grep -l "DO NOT EDIT\|Code generated" src/tools/*.go` não retorna nada; (ii) o relatório da Fase 2 fala em "3 subagents de geração" por grupo e em "revisão humana pré-geração dos 53 métodos candidatos" (`reports/...fase2-vm-codegen.report.md`); (iii) os cabeçalhos dos arquivos citam evidência que só uma leitura humana/LLM produz (ex.: `generated_alarm.go:18-60`).
- **Fluxo real** (plano de codegen): `gen` → relatório revisado pelo humano → grupos de métodos delegados a subagentes em paralelo → cada um escreve `generated_<grupo>.go` + `_test.go` → o orquestrador **liga** o `registerXTools` em `registry.go` (à mão) e atualiza `mode_test.go` → gate `go build` + `go vet` + `gofmt` + testes por pacote → commit por fase.
- **Como evita divergir do código?** **Não evita**, porque não há regeneração: depois de escritos, os arquivos são código mantido à mão. A rede de proteção é o **teste de conjunto exato por modo** (`mode_test.go`) e o *gate* de `go build` limpo por fase (que pegou `DeleteSolutionOnly`, método que existia no checkout `referencia/` mas não no `govmomi` fixado em `go.mod`, bug-012).
- **Técnica sem fonte Go** (VAMI legado, Workstation, VMC): parse estruturado (Python) de uma *collection* Postman versionada em `.workspace/` + classificação de tier manual; decode genérico `interface{}` (`cerebrum.md:405-413`).

### 6.3 `TOOLS.md` sai do registro em execução

- **Entrada:** `src/cmd/dumptools` (47 linhas): constrói um `Registry` por modo com **cliente nulo** e imprime `name`, `description`, `inputSchema` em JSON, com 4 chaves (`vcenter`, `vmware`, `workstation`, `cloudaws`) (`cmd/dumptools/main.go:23-47`). Seguro porque `NewRegistry` não desreferencia o cliente.
- **Transformação:** `scripts/gen_tools_md.py` (165 linhas), em Python: lê o JSON, monta a união dos nomes, classifica produto (`workstation`/`cloud-aws`/`vsphere`), deriva o **domínio** do nome (2º segmento; `host`, `appliance`, `cloudaws` descem mais um nível, `:69-81`), agrupa por produto e domínio com títulos num dicionário (`DOMAIN_TITLE`, `:52-68`), e para cada tool escreve: nome, `**Modo:**` (+ `*(destrutiva)*` se o schema tem `confirm`, `:150`), descrição, `**Sintaxe:** nome(obrigatórios, [opcionais])` (`sig`, `:105-114`) e a tabela de parâmetros. Corrige *mojibake* (UTF-8 lido como CP1252, `fix`, `:17-25`) e escapa `|`.
- **Disparo:** **manual**, em duas etapas (`README.md:220-226`; cabeçalho do script `:7-9`): `cd src && go run ./cmd/dumptools > catalog.json`, depois `python scripts/gen_tools_md.py [catalog.json] [TOOLS.md]`. Nenhum hook, CI nem `go generate`. O cabeçalho do `TOOLS.md` avisa: "Nao editar a mao — regenerar apos mudar tools" (`TOOLS.md:5`).
- **Como evita divergir?** Só pela fonte: sai do **mesmo** `ListTools()` que o `tools/list` real, então não depende de ninguém lembrar de escrever documentação. **Mas não há verificação automática de que o arquivo esteja atualizado:** `grep -rn "TOOLS.md"` em `*.go`, `*.yml`, `*.ps1` só acha o próprio script. **Medido hoje:** os 1.047 nomes do registro e os 1.047 títulos `#### \`nome\`` do `TOOLS.md` são **idênticos** (diff vazio), ou seja, está em dia, por disciplina.
- **Lacunas:** o script lê só 4 modos e usa a união; as 2 tools meta aparecem sob "Workstation Pro" no índice (`TOOLS.md:111-114`: "Describe — 1", "Inventario - Listagens — 1"), efeito do `product()` com valor padrão (`gen_tools_md.py:91-96`); os títulos de domínio estão sem acento ("Autenticacao") por uma regra de encoding do projeto.
- **Tamanho:** 1.027.395 bytes, 14.738 linhas. Inútil como contexto de modelo inteiro; útil como referência humana e para busca.

### 6.4 O que o mcpollama aproveita

1. Copiar `dumptools` + gerador de Markdown, **em Go** (sem Python, para um só toolchain), disparado por `go generate` ou por um comando `mcpollama-mcp --dump-tools` e escrito em `TOOLS.md`.
2. Acrescentar o que falta: **teste que regenera em memória e compara com o `TOOLS.md` versionado** (falha se divergir). Com ~8 tools o arquivo é pequeno e o teste é barato.
3. Não precisa de `gen`/classificador: as tools do mcpollama são poucas e escritas à mão; o equivalente do tier (leitura/escrita) é "lê arquivo?" e fica na própria declaração da tool.
4. Escrever o `.md` com acentos corretos (o projeto desta referência abriu mão disso).

## 7. Nota sobre comandos `git` nesta onda

Antes de o coordenador proibir `git` dentro de `referencias/`, rodei **quatro comandos de leitura** na árvore de `MCPVMWare` (`git log --oneline -3 -- src/gen`, `git log --oneline -- src/gen/main.go | wc -l`, `git log --oneline | head -5` e `git describe --tags --always --dirty`). Depois da restrição, **nenhum**. O `.git/index` de `MCPVMWare` tem `mtime` 2026-10-23 16:02:18 (medido com `ls`, sem git), compatível com a atualização de *stat cache* que esses comandos podem fazer. Não rodei `git status`, `add`, `commit` nem comando que altere o histórico.
