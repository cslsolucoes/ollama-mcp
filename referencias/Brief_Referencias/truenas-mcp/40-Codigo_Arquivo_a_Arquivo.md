# truenas-mcp — Código arquivo a arquivo

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Caminhos relativos a `referencias/MCPNas/truenas-mcp/`. Linhas lidas diretamente nos arquivos (CRLF; `wc -l` e numeração do leitor coincidem).

## `cmd/truenas-mcp/main.go` (308 linhas)

| Elemento | Linhas | Papel |
| --- | --- | --- |
| flags globais | `:20-27` | `truenas-url`, `api-key`, `insecure`, `tls-ca`, `version`, `debug` |
| `var Version = "dev"` | `:29-31` | injetado por `-ldflags "-X main.Version=..."` |
| `main()` | `:33-117` | parse → env → TLS → cliente → `tasks.Manager` → `tools.NewRegistry` → `StdioHandler.Run()` |
| `tasks.PollerConfig` | `:100-104` | `PollInterval` 5 s, `MaxPollAttempts` 0 (ilimitado), `CleanupInterval` 1 min |
| `StdioHandler` | `:119-133` | `registry mcp.ToolRegistry`, `*bufio.Scanner`, `stdoutMutex`, `debug` |
| `Run` | `:135-173` | laço `Scan()` → `Unmarshal` → `handleRequest` → `sendResponse` |
| `handleRequest` | `:175-195` | 4 métodos; desconhecido com `id` → `-32601`; sem `id` → ignora |
| `handleInitialize` | `:197-216` | `protocolVersion` fixo `2024-11-05`, `capabilities.tools.listChanged=false` |
| `handleToolsList` / `handleToolsCall` | `:218-229` / `:231-273` | lista e chamada; erro de handler → `isError:true` |
| `createErrorResponse` / `sendResponse` / `sendError` | `:275-284` / `:286-301` / `:303-308` | resposta com mutex e `Printf("%s\n")` |

## `mcp/types.go` (71 linhas)

Tipos `Request` (`:5-10`), `Response` (`:12-17`), `Error` (`:19-23`), `InitializeResult` (`:27-31`), `ServerInfo` (`:33-36`), `Capabilities` (`:38-40`), `Tool` (`:42-46`), `ToolsListResult` (`:48-50`), `ToolCallParams` (`:52-55`), `ToolCallResult` (`:57-60`), `ContentBlock` (`:62-65`) e interface `ToolRegistry{ListTools; CallTool}` (`:68-71`). `Params` é `map[string]interface{}`: parâmetros que não sejam objeto (array) não decodificam (limitação, **inferência**).

## `proxy/` (152 linhas, não usado)

- `proxy/stdio.go` (86): `StdioHandler` (`:17-21`), `NewStdioHandler` (`:24-29`), `ReadRequest` (`:32-51`), `WriteResponse` (`:54-73`), `WriteError` (`:76-86`). Duplica a lógica de `main.go`.
- `proxy/config.go` (66): `Config{ServerURL, APIKey, Timeout, Debug, Insecure}` (`:13-19`) e `LoadConfig` (`:22-66`). `--version` devolve `errors.New("version requested")` (`:36-38`), uma forma estranha de sinalizar.

## `tasks/` (568 linhas)

| Arquivo | Linhas | Conteúdo |
| --- | ---: | --- |
| `types.go` | 51 | `TaskStatus` (`:8-16`), `OperationType` (`:19-24`), `Task` (`:27-44`), `PollerConfig` (`:47-51`) |
| `store.go` | 160 | `TaskStore` (`:11-15`), `Add` (`:26`), `Get` (`:41-56`), `Update` (`:59-71`), `List` (`:74-123`, cursor por `TaskID`), `GetActive` (`:126-146`), `CleanExpired` (`:149-160`) |
| `manager.go` | 150 | `Manager` (`:13-20`), `NewManager` (`:23-37`), `Start` (`:40-46`), `Shutdown` (`:49-51`), `cleanupLoop` (`:54-66`), `CreateJobTask` (`:69-88`), `CreateStatusTask` (`:91-110`), `Get`/`List` (`:113-120`), `Cancel` (`:123-150`) |
| `poller.go` | 207 | `Run` (`:29-41`), `pollAllTasks` (`:44-55`), `pollJobTask` (`:58-82`), `pollStatusTask` (`:85-102`), `updateTaskFromJob` (`:105-155`), `updateTaskFromStatus` (`:158-207`) |

Dependência externa: `github.com/google/uuid` (`manager.go:8`). IDs de task: UUID v4.

## `truenas/client.go` (878 linhas) — cliente do backend

| Elemento | Linhas | Papel |
| --- | --- | --- |
| `debugLogging atomic.Bool`, `init`, `SetDebugLogging`, `DebugLogging` | `:22-36` | liga/desliga o log de corpos |
| `Client` | `:38-67` | `connMu`, `writeMu`, `pending` (id→canal), `pendingMethods`, `requestID atomic.Uint64` |
| `NewClient` | `:105-119` | valida endpoint e chave não vazios; **não** conecta |
| `connect` | `:123-199` | dialer com `HandshakeTimeout` 10 s e buffers de 64 KiB (`:133-138`); dica de TLS (`:145-147`); `SetReadLimit` 10 MiB (`:154`); mensagem `connect`/`connected` (`:157-186`); inicia `readLoop` |
| `readLoop` | `:203-256` | lê respostas, entrega ao canal por `id`; log redigido (`:234-248`); em queda, `failAllPending` e zera a conexão |
| `buildConnectionURLs` | `:272-291` | recusa `ws://`; aceita `wss://` completo ou monta `wss://<host>:443/websocket` |
| `Authenticate` / `Call` | `:293-325` / `:327-344` | `auth.login_with_api_key`; `Call` conecta e autentica sob demanda |
| `sensitiveKeyFragments`, `isSensitiveKey`, `redactValue`, `RedactJSONForLog` | `:511`, `:513-521`, `:524-545`, `:551-561` | redação por chave |
| `maxDebugLogLineBytes`, `TruncateForLog` | `:581`, `:587-592` | teto de 4.000 bytes |
| `bareScalarSecretResultMethods`, `redactResponseForLog` | `:611-613`, `:642-661` | redação por método |
| `scrubSecrets`, `redactParams` | `:665-670`, `:676-689` | valor exato e parâmetros |
| `callRaw` | `:693-826` | até 2 tentativas; canal registrado **antes** de escrever (`:736-744`); `writeMu` (`:762-765`); timeout de 120 s (`:815-821`) |
| `isConnectionError`, `Close`, `formatAPIErrorWithContext` | `:829-841`, `:843-851`, `:856-878` | classificação por substring do texto do erro, fechamento e erro com contexto |

`:346-510` é um comentário de ~165 linhas (histórico de cada fragmento de `sensitiveKeyFragments`), útil como registro mas ruim como documentação (a lição está em `80-Licoes_Aprendidas.md`).

Observações de design (para o mcpollama): a multiplexação por `id` + canal de resposta é específica de WebSocket; num cliente HTTP, cada `Call` é uma requisição independente e o equivalente é `http.Client` com `Timeout`/`context`. A classificação de erro de conexão por **texto** (`isConnectionError`, `:829-841`) é frágil; em Go usar `errors.Is(err, syscall.ECONNRESET)`/`net.Error` (**sugestão**).

## `truenas/tls.go` (34 linhas)

`NewTLSConfig(insecure bool, caPath string) (*tls.Config, error)` (`:18-34`). Ver `30-Configuracao_e_Seguranca.md`. Importa só `crypto/tls`, `crypto/x509`, `fmt`, `os`.

## `tools/` — por família

- `tools/registry.go` (26.107): `Registry` (`:15-19`), `Tool` (`:21-24`), `NewRegistry` (`:26-34`), `registerTools` (`:36-~20442`), `ListTools` (`:20444-20450`), `CallTool` (`:20452-20461`) e handlers históricos (`system_info`, `query_*`, métricas, `tasks_*`, atualizações, até `~:26107`).
- `tools/dryrun.go` (85): contrato de dry-run (`:10-76`).
- `tools/core.go` (840): `getJobByID` (`:28-45`), `coreJobWaitPollBudget` 100 s (`:510-516`), `handleCoreJobWait` (`:552-~596`).
- `tools/boot.go` (808): ver `20-Tools_e_Contratos.md`.
- Demais famílias (`pool*`, `vm*`, `iscsi`, `nvmet_*`, `app*`, ...) seguem o mesmo molde "leitura simples / escrita com dry-run / job com task"; não catalogadas.
- `tools/kmip.go` começa com BOM UTF-8 (`EF BB BF`), único arquivo assim; quebra `go test -cover` (medido).

## O que **não** existe

Sem pacote de log próprio (usa `log` do Go para stderr), sem camada de configuração (flags soltas em `main`), sem cliente HTTP (nenhum `net/http` fora de testes), sem gerador de documentação de tools no repositório (só scripts Python em `scripts/`, 3 arquivos de 55 a 147 linhas, que geram coleção Postman e lista de tools).
