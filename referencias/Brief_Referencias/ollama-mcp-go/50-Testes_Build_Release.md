# ollama-mcp-go — testes, build e release

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Execuções feitas em 2026-10-23 com Go 1.26.5 na pasta da referência, **cache de teste desligado** (`-count=1`); `git status --short` vazio antes e depois (nada foi gravado na referência).

## Resultado medido

| Comando | Resultado lido |
| --- | --- |
| `go test -count=1 -v ./...` | 26 linhas `--- PASS`, `PASS`, `ok github.com/KTCrisis/ollama-mcp-go 0.110s` |
| `go test -count=1 -cover ./...` | `coverage: 75.2% of statements` |
| `go vet ./...` | sem saída |
| `grep -c "^func Test"` | 21 em `ollama-mcp-go/main_test.go` (inclui `TestMain`), 6 em `ollama-mcp-go/protocol_test.go` |

Observação de método: o "verde" acima é de cache limpo para o **resultado de teste**, mas o build do `go test` usa o cache de compilação do Go (que o próprio Go invalida por conteúdo). Nenhum artefato do projeto foi limpo, porque não há nenhum versionado.

## Padrão de testes

1. **Pacote único, testes internos** (`package main`), então testam funções não exportadas (`toolGenerate`, `dispatch`, `handleToolsCall`) diretamente, sem subprocesso (`ollama-mcp-go/main_test.go:1`, `ollama-mcp-go/protocol_test.go:1`).
2. **Mock do Ollama com `httptest.NewServer`** (`mockOllama`, `ollama-mcp-go/main_test.go:106-159`): um `http.HandlerFunc` com `switch r.URL.Path` para `/api/tags`, `/api/generate`, `/api/chat`, `/api/embed`; demais caminhos devolvem 404. Os handlers decodificam o request com os **mesmos structs de produção** (`ollamaGenerateReq` etc.), então o teste também protege o formato do corpo enviado. O mock exige `model` não vazio (400 senão), mas as tools validam antes, então esse ramo nunca é exercitado.
3. **Mock que fala NDJSON de verdade** (`writeNDJSON`, `ollama-mcp-go/main_test.go:83-104`): quebra o texto em tokens por espaço (`strings.SplitAfter(text, " ")`), emite um objeto por linha com `json.NewEncoder(w).Encode`, e fecha com um chunk `done:true` com `prompt_eval_count:7` e `eval_count:11`. Serve a `generate` (`response`) e a `chat` (`message.content`).
4. **Injeção do servidor de teste:** `testServer` monta `&server{ollamaURL: mock.URL, client: mock.Client()}` e devolve `cleanup` (`ollama-mcp-go/main_test.go:161-169`). Um mock próprio e menor é criado no teste de raciocínio (`ollama-mcp-go/main_test.go:437-444`).
5. **Isolamento de efeito colateral:** `TestMain` define `OLLAMA_MCP_TRACE=off` (`ollama-mcp-go/main_test.go:73-76`); testes de trace usam `t.Setenv` e `t.TempDir()` (`ollama-mcp-go/main_test.go:401-402`, `446-447`).
6. **Helper de protocolo:** `call(t, s, rawJSON)` monta `rpcRequest`, chama `dispatch`, remarshala e devolve `map[string]any` (`ollama-mcp-go/protocol_test.go:8-18`), então asserta sobre o JSON real que sairia.
7. **Asserções simples:** `strings.Contains` e comparações diretas com `t.Errorf`/`t.Fatalf`; sem biblioteca de asserção.

## Casos cobertos (26 testes)

| Área | Testes (linha) |
| --- | --- |
| Handshake legado | `TestInitialize` (`ollama-mcp-go/main_test.go:17`), `TestLegacyHandshake` (`ollama-mcp-go/protocol_test.go:64`) |
| `tools/list` | `TestToolsList` (4 tools, nomes) (`ollama-mcp-go/main_test.go:34`); `TestModernToolsList` (campos 2026, `title`, anotações) (`ollama-mcp-go/protocol_test.go:37`) |
| Tool desconhecida | `TestToolsCallUnknown` (`ollama-mcp-go/main_test.go:58`) |
| `list_models` | `TestListModels` (`ollama-mcp-go/main_test.go:171`) |
| `generate` | `TestGenerate`, `TestGenerateWithSystem`, `TestGenerateMissingParams` (`ollama-mcp-go/main_test.go:188`, `204`, `222`) |
| `chat` | `TestChat`, `TestChatMultiTurn`, `TestChatMissingParams`, `TestChatInvalidMessages`, `TestChatEmptyMessages` (`ollama-mcp-go/main_test.go:237`, `255`, `276`, `293`, `473`) |
| `embed` | `TestEmbed`, `TestEmbedMissingParams` (`ollama-mcp-go/main_test.go:322`, `343`) |
| Streaming | `TestStreamReassembly` (reconstrói texto + contadores 7/11), `TestChatStreamCounters` (`ollama-mcp-go/main_test.go:362`, `381`) |
| Trace | `TestTraceFile`, `TestTraceOff` (`ollama-mcp-go/main_test.go:397`, `424`) |
| Raciocínio fora da resposta | `TestThinkingStaysOutOfReply` (`ollama-mcp-go/main_test.go:436`) |
| Round-trip de linhas | `TestStdioRoundTrip` (`ollama-mcp-go/main_test.go:488`) |
| Protocolo moderno | `TestDiscover`, `TestUnsupportedModernVersion`, `TestPingOnlyForLegacy`, `TestToolsCallCarriesResultType` (`ollama-mcp-go/protocol_test.go:22`, `52`, `74`, `85`) |

## Casos NÃO cobertos (lacunas medidas por leitura; cobertura global 75,2%)

| # | Lacuna | Onde está o código sem teste |
| --- | --- | --- |
| 1 | **`main()` nunca é executado**: `TestStdioRoundTrip` copia o loop dentro do teste (`ollama-mcp-go/main_test.go:515-534`) e ainda deixa variáveis mortas (`writer`, `_ = writer`, `ollama-mcp-go/main_test.go:509`, `537`) | `ollama-mcp-go/main.go:627-662` |
| 2 | JSON inválido em stdin, linha vazia, `id:null`, requisição sem `id` | `ollama-mcp-go/main.go:640-654` |
| 3 | Linha de stdin acima de 4 MB | `ollama-mcp-go/main.go:635`, `659-661` |
| 4 | HTTP ≠ 200 do Ollama (400/401/403/500) em `list_models`, `embed` e `postStream` | `ollama-mcp-go/main.go:389-392`, `432-435`, `565-568` |
| 5 | Corpo malformado: `failed to parse response`, linha NDJSON inválida | `ollama-mcp-go/main.go:402-404`, `437-440`, `570-573` |
| 6 | Erro no meio do stream (`{"error":...}` com `200`); stream sem `done:true` | `ollama-mcp-go/main.go:118-124`, `413-415` (o struct nem tem `error`) |
| 7 | Falha de rede e **timeout** de 5 min | `ollama-mcp-go/main.go:383-386`, `426-429`, `167` |
| 8 | `embed` com lista vazia; `embed` com mais de 16 valores (corte do preview) | `ollama-mcp-go/main.go:576-583` (o mock devolve 5 valores) |
| 9 | `list_models` sem modelos | `ollama-mcp-go/main.go:447-449` |
| 10 | `params` ausente/inválido em `tools/call` (o executado nesta onda deu `-32602`, mas não há teste) | `ollama-mcp-go/main.go:267-269` |
| 11 | `newServer` com `/` final em `OLLAMA_HOST`; valor sem esquema | `ollama-mcp-go/main.go:159-165` |
| 12 | Falha ao abrir o trace (caminho inválido) e seu efeito "não fatal" | `ollama-mcp-go/main.go:329-333` |
| 13 | Concorrência, cancelamento, e `ping` moderno em fluxo real | (ausentes no código) |
| 14 | `tools/call` com tipo errado em `arguments` (por exemplo, `prompt` numérico) | `ollama-mcp-go/main.go:454-456` |
| 15 | Teste de conformidade do `tools/list` contra o esquema MCP (`inputSchema` válido) | só se checam contagem e nomes (`ollama-mcp-go/main_test.go:43-55`) |

`TestTraceOff` depende de `TestMain` ter definido a variável antes (ele afirma `traceFile() == ""` sem setar, `ollama-mcp-go/main_test.go:425-427`): acoplamento implícito entre testes.

Para o mcpollama: manter o desenho (mock `httptest` com NDJSON de verdade, `testServer`, `call`), **chamar o `main`/loop por uma função testável** (por exemplo, `Run(in io.Reader, out io.Writer)`), e cobrir as linhas 2 a 8 e 10 a 12 da tabela, mais testes de **leitura de arquivo** (fora de raiz, symlink, arquivo grande, binário), que esta referência não tem.

## Build

- **Comando documentado:** `go build -o ollama-mcp-go .` (`ollama-mcp-go/README.md:36-42`); requisito declarado "Go 1.22+ e uma instância do Ollama" (`ollama-mcp-go/README.md:44`), mas `go.mod` exige `go 1.24.2` (`ollama-mcp-go/go.mod:3`): inconsistência.
- Sem `Makefile`, `tools/Build-Release.ps1`, `Dockerfile`, `.github/`, GoReleaser ou scripts (listagem de `git ls-files`: 7 arquivos).
- `.gitignore` ignora o binário (`ollama-mcp-go`), `*.exe` e `CLAUDE.md` (`ollama-mcp-go/.gitignore:1-3`).
- Cross-compile não documentado (Go o faz por `GOOS`/`GOARCH`; o `CLAUDE.md` do mcpollama já traz os comandos).

## Versão

- `const version = "0.2.0"` hard-coded (`ollama-mcp-go/main.go:59`), usada em `serverInfo` (`ollama-mcp-go/protocol.go:34`) e testada indiretamente (`TestInitialize` só checa o `name`, `ollama-mcp-go/main_test.go:28-31`). **Sem `--version`**, sem `-ldflags -X`, sem tag git (`git log` não foi cruzado com `git tag`; não verificado).
- Para o mcpollama: `--version` obrigatório (requisito do `CLAUDE.md`), com a versão injetada por `-ldflags` ou constante única, e `serverInfo.version` lendo a mesma fonte.

## Release

- O README diz "Pre-built binaries: Coming soon" (`ollama-mcp-go/README.md:46-48`): **não há processo de release** nesta referência.
- Instalação por `git clone` + `go build` (`ollama-mcp-go/README.md:36-42`); registro do cliente por caminho do binário (`ollama-mcp-go/README.md:50-62`).
- Para o mcpollama: o molde de release é do `MCPVMWare` (`tools/Build-Release.ps1`), a ser coberto na onda B2. Esta referência não contribui com nada nesse ponto.
