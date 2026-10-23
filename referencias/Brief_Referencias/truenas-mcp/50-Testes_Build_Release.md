# truenas-mcp — Testes, build e release

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## Padrão de testes

Medido (cópia no scratchpad com `go.mod` mínimo; ver `README.md` §Medições):

| Pacote | Arquivos de teste | Linhas | Testes (raiz) | Cobertura |
| --- | --- | ---: | ---: | --- |
| `truenas/` | `client_test.go` (822), `tls_test.go` (128) | 950 | 27 (22 + 5) | **70,6%** (`-cover`) |
| `tools/` | `apps_test.go` (1.006), `bootenvironment_test.go` (386), `dataset_test.go` (260), `nfs_test.go` (238), `reporting_exporters_test.go` (199), `scrub_test.go` (221), `smb_test.go` (547), `snapshot_test.go` (454) | 3.311 | 37 | **não medida**: `-cover` falha por BOM em `tools/kmip.go` |
| `cmd/`, `mcp/`, `proxy/`, `tasks/` | nenhum | 0 | 0 | — |

`go test -count=1 -v ./truenas ./tools`: 64 `--- PASS`, 0 `--- FAIL`. 10 arquivos de teste e 4.261 linhas para 109.126 linhas de código (≈ 4%).

Convenção declarada: "só funções puras têm teste unitário; handlers que chamam `client.Call(...)` não têm teste dedicado" (`.wolf/STATUS.md`, seção *Active architecture*, linha "Testes"). Consequências medidas: **o loop stdio (`main.go`) e o `tasks/` não têm nenhum teste**; todo o protocolo MCP foi validado só ao vivo e pelos bugs reportados por clientes (bug-016/021). O `ollama-mcp-go` tem testes de protocolo e de round-trip (ver o brief dele); o mcpollama deve ter ambos.

### Estilo dos testes

- **Tabela de casos** com `name/input/expected` e `t.Run` (`tools/scrub_test.go:8-40`).
- **Testes de redação** com segredos sentinela (`hunter2`, `1-verysecretapikey`) e asserção de que a saída serializada **não contém** o segredo e **contém** o dado não sensível (`truenas/client_test.go:48-80`, constante `testAPIKey` em `:19`).
- **Lista exaustiva para detectar regressão de refatoração:** `authNamespaceMethods` (`client_test.go:27-32`) cobre os 14 métodos `auth.*` em dois testes, para que um `strings.HasPrefix("auth.")` estreitado não passe despercebido.
- **Captura de log:** `log.SetOutput(&buf)` + `defer log.SetOutput(os.Stderr)` (`client_test.go:703-707`), usado por `TestLoggingDefaultModeOmitsBodies` (`:724`), `TestLoggingDebugModeRedactsSecrets` (`:749`), `TestServerEchoedKeyIsScrubbed` (`:772`), `TestTruncateForLog` (`:804`).
- **Cada bug corrigido ganha teste de regressão** com nome explícito (`TestRedactParamsUPSMonpwd`, `:544`; `TestRedactResponseForLogTnConnectClaimToken`, `:604`).

### Mocks

- **Servidor falso TLS+WebSocket:** `startFakeTrueNAS(t, echoKeyInError)` com `httptest.NewTLSServer` e `websocket.Upgrader` (`client_test.go:652-697`): responde `connected`, `auth.login_with_api_key` (sucesso ou erro que ecoa a chave) e qualquer outra chamada com um campo secreto (`bindpw`) no resultado. `t.Cleanup(srv.Close)`.
- **Certificados de teste gerados na hora:** ECDSA P-256 autoassinado com `x509.CreateCertificate`, gravado em `t.TempDir()` com permissão `0o600` (`truenas/tls_test.go:83-117`, helper `serverCertPEM` `:32-40`). O certificado do `httptest` é exportado em PEM (`srv.Certificate().Raw`) para testar `--tls-ca`.
- Para o mcpollama: trocar WebSocket por `httptest.NewTLSServer(http.HandlerFunc(...))` simulando `/api/chat`, `/api/generate`, `/api/embed`, `/api/tags`, inclusive resposta em *stream* NDJSON, `401` e `403`.

### Testes de TLS (4+1)

`tls_test.go`: `TestTLSDefaultRejectsUntrustedCert` (`:43`), `TestTLSInsecureOptInAccepts` (`:59`), `TestTLSCAPinnedCertAccepts` (`:71`), `TestTLSCAPinDoesNotDisableVerification` (`:83`), `TestNewTLSConfigErrors` (`:119`, CA inexistente e PEM sem certificado).

## Build

- Go 1.25+ (`go.mod` pedia 1.25.12 segundo `.wolf/STATUS.md`, seção *Active architecture*; o `go.mod` não está na cópia). Medido aqui com Go 1.26.5.
- Gate por fase, documentado: `go build ./... && go vet ./... && go test ./... -count=1` (`.wolf/STATUS.md`, seção *Useful commands*).
- Medido: `go build ./...` leva ~9 s com cache frio de módulos; `go vet` limpo.
- Binário Windows na raiz da pasta (`TruenasMcp.exe`, 15.545.344 bytes), reconstruído manualmente a cada correção. Problema registrado: rebuild falha quando o `.exe` está em uso por um cliente MCP; "os processos que bloqueavam o ficheiro tinham terminado sozinhos" (`.wolf/STATUS.md:153`), e um processo antigo mantido pelo Cursor mostrava 52 de 622 tools (`.wolf/memory.md:88`).
- Dependências externas mínimas: `github.com/gorilla/websocket` e `github.com/google/uuid` (resolvidas por `go mod tidy` na cópia).

## Cross-compile

Sem receita no repositório copiado. Medido: `GOOS=linux GOARCH=amd64 go build -o <saida> ./cmd/truenas-mcp` gera binário de 15.416.372 bytes sem CGO nem alteração de código. O CLAUDE.md do mcpollama já prevê o mesmo comando em PowerShell (`$env:GOOS='linux'`).

## Versão e release

- `var Version = "dev"` em `main`, sobrescrito por `-ldflags "-X main.Version=<valor>"` (`cmd/truenas-mcp/main.go:29-31`); `--version` imprime `truenas-mcp version <Version>` (`:36-39`) e a mesma string vai em `serverInfo.version` (`:200-203`). Medido com `probe-1`.
- Convenção citada: `VERSION=$(git describe --tags --always --dirty)` num `Makefile` (bug-021, `.wolf/buglog.json`); o `Makefile` não está na cópia (GAP).
- **Não há** pipeline de release, assinatura, `CHANGELOG` do produto nem geração de `TOOLS.md` (o modelo MCPVMWare tem `tools/Build-Release.ps1` e gerador, ver o brief dele). O "histórico" é o plano de 110 fases (`.workspace/plans/truenas-mcp2026-08-01-204513-cobertura-api-truenas-mcp.plan.md`, 3.927 linhas) e `.wolf/STATUS.md`.

## Higiene do repositório que gerou bugs

- **CRLF:** todos os `.go` ficaram em CRLF; `gofmt -w` direto reescreve para LF e `git diff` com `core.autocrlf=true` mascara. Receita: `tr -d '\r' < arq.go > /tmp/check.go && gofmt -l /tmp/check.go`. Arquivos novos criados pela ferramenta de escrita saíram em LF 4-5 vezes (bug-003/006/011/012). Falso negativo conhecido: `grep -c $'\r$'` do Git Bash dá 0 em arquivo CRLF; conferir com `od`/leitura de bytes (`.wolf/cerebrum.md:65`).
- **BOM:** `tools/kmip.go` (medido) impede `go test -cover`.
- Para o mcpollama: `.gitattributes` (`*.go text eol=lf`), `gofmt -l` no gate e `go test -count=1 -cover ./...` no gate (a cobertura detecta BOM cedo).
