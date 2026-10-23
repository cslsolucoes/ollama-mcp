# truenas-mcp — Arquitetura

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## Visão em camadas

```
cliente MCP (stdio, 1 JSON por linha)
   └─ cmd/truenas-mcp/main.go   StdioHandler: parse → handleRequest → resposta
        └─ mcp/types.go         Request/Response/Tool/ToolCallResult + interface ToolRegistry
             └─ tools/registry.go   map[string]Tool{Definition, Handler}; ListTools/CallTool
                  ├─ tools/<familia>.go   handlers (client.Call("metodo.json.rpc", ...))
                  ├─ tasks/               jobs assíncronos (CreateJobTask) + poller
                  └─ truenas/client.go    WebSocket + auth + redação de log + TLS
```

## `cmd/` (entrypoint)

- `cmd/truenas-mcp/main.go` (308 linhas): flags globais (`:20-27`), `Version` injetável por `-ldflags` (`:29-31`), fallback flag→env (`:41-69`), TLS (`:74-80`), cliente (`:83-87`), `tasks.Manager` com poller de 5 s (`:99-107`), `tools.NewRegistry` (`:110`) e `StdioHandler.Run()` (`:113-116`).
- **Autenticação lazy:** o comentário em `main.go:89-97` explica por que o `Authenticate()` eager foi removido (bug-021): `initialize`/`tools/list` não precisam do backend e o handshake de ~1,4 s corria contra o timeout/reconexão do cliente.
- `cmd/fetch-system-info/main.go` (53) e `cmd/fetch-update-info/main.go` (59): programas de uma chamada para validar o cliente; só aceitam `-insecure`, **não** `--tls-ca` (`cmd/fetch-system-info/main.go:11-31`, `:32-38`).

## `mcp/` (protocolo)

`mcp/types.go` (71 linhas): `Request{JSONRPC, ID interface{}, Method, Params map}` (`:5-10`), `Response` com `Result`/`Error` (`:12-23`), `InitializeResult`/`ServerInfo`/`Capabilities` (`:27-40`), `Tool{Name, Description, InputSchema map}` (`:42-46`), `ToolCallParams`/`ToolCallResult{Content []ContentBlock, IsError}` (`:52-65`) e `ToolRegistry` (`:68-71`). Só tipo `text` em `ContentBlock`; sem `structuredContent`, sem `annotations`.

Cobertura do protocolo, **medida** com um binário compilado: só `initialize`, `notifications/initialized`, `tools/list`, `tools/call` (`main.go:175-195`). Efeitos medidos: `protocolVersion` é sempre `2024-11-05` (`main.go:199`, sem negociação); `ping` devolve `-32601`; notificações desconhecidas são ignoradas; `Response.ID` com `omitempty` omite o `id` na resposta de erro de parse (o JSON-RPC pede `null`, **inferência** sobre a especificação).

## `proxy/` — o que é de fato

O nome sugere "modo stdio↔HTTP", mas **não é isso** neste código:

- `proxy/stdio.go` (86 linhas): `StdioHandler` com `ReadRequest`/`WriteResponse`/`WriteError` sobre stdin/stdout (`:17-86`), duplicando o `StdioHandler` de `main.go:119-133`.
- `proxy/config.go` (66): flags `--server-url`, `--api-key`, `--timeout` (30 s), `--debug`, `--insecure` e envs `TRUENAS_MCP_SERVER_URL`/`TRUENAS_MCP_API_KEY` (`:26-63`).
- **Ninguém importa o pacote:** `grep` por `truenas-mcp/proxy` em todo o código fora de `docs/` só acha as declarações `package proxy` (`proxy/stdio.go:1`, `proxy/config.go:1`). Não há cliente HTTP nem repasse. É o esqueleto de um "proxy" (cliente MCP stdio que falaria com um servidor MCP remoto em `--server-url`, **inferência** pelas flags) que nunca foi concluído, ou foi removido. O `.vscode/MCP-VSCODE.md:45` e o `STATUS.md` não o mencionam.
- **Quando usar o conceito:** só se o mcpollama virar servidor remoto (HTTP) com um shim stdio local. Hoje o CLAUDE.md do mcpollama prevê stdio direto contra o Ollama; **não copiar** `proxy/`.

## `tasks/` — jobs assíncronos e poller

Objetivo: operações do TrueNAS que viram *jobs* no middleware (resilver, upgrade de app) devolvem rápido um `task_id`; o cliente consulta depois.

| Peça | Fonte | O que faz |
| --- | --- | --- |
| `Task` | `tasks/types.go:27-44` | `TaskID`, `Status`, `StatusMessage`, `TTL`, `PollInterval` (JSON) + campos internos `json:"-"`: `OperationType`, `JobID`, `StatusMethod`, `ToolName`, `Arguments`, `Result`, `Error` |
| Estados | `tasks/types.go:10-16` | `working`, `input_required`, `completed`, `failed`, `cancelled` (mesmo vocabulário da proposta de tarefas do MCP, **inferência**) |
| `TaskStore` | `tasks/store.go:11-160` | `map` + `expiry` sob `RWMutex`; `Add`, `Get` (erro se expirou), `Update`, `List` (cursor, padrão 50), `GetActive`, `CleanExpired` |
| `Manager` | `tasks/manager.go:13-150` | `Start` (poller + `cleanupLoop` a cada 1 min), `CreateJobTask`, `CreateStatusTask`, `Get`, `List`, `Cancel` |
| `Poller` | `tasks/poller.go:12-207` | a cada `PollInterval` (5 s), consulta `core.get_jobs` por `id` e traduz `RUNNING/WAITING/SUCCESS/FAILED/ABORTED` em status da task |

Como as tasks chegam ao cliente: **como tools comuns**, não como método de protocolo. `tasks_list` e `tasks_get` estão em `tools/registry.go:4880` e `:4902`, handlers em `:24007` e `:24035`. Um handler de escrita (ex.: `handleStartApp`, `tools/registry.go:22789-22830`) chama o método JSON-RPC, extrai o `job_id`, cria a task com TTL (ex.: 10 min) e devolve `task_id`, `task_status`, `poll_interval`, `job_id` e uma frase "acompanhe com tasks_get".

Limitações medidas na leitura:

1. **`Result` nunca é exposto:** `tasks/types.go:42` (`json:"-"`) + `handleTasksGet` serializa o struct direto (bug-015, `.wolf/buglog.json`). O poller guarda o resultado (`poller.go:129-131`), mas ninguém o devolve.
2. **`Manager.Cancel` não tem chamador** (`grep` de `.Cancel(` em `tools/ tasks/ cmd/` só acha a definição, `tasks/manager.go:123`), logo não há tool de cancelamento.
3. **Acoplado ao `truenas.Client`:** `Manager` e `Poller` guardam `*truenas.Client` (`manager.go:14`, `poller.go:14`); só `store.go` e `types.go` são independentes do backend.
4. **Possível corrida de dados (inferência):** `Store.Get` devolve o ponteiro `*Task`, o poller altera seus campos sem o lock do store (`poller.go:150-153`) e `handleTasksGet` serializa o mesmo objeto. Não foi rodado com `-race`.
5. **Erros de rede no poll são engolidos** (`poller.go:67-70`): a task fica `working` até o TTL.

### Serviria para gerações longas do Ollama que estouram o timeout do cliente?

**Sim para o desenho, não para o código.** O que o Ollama precisa:

- Uma tool `generate` (ou `summarize_file`) que, se a geração passar de um limite configurável abaixo do timeout do cliente MCP, devolve `{task_id, status:"working", poll_interval}` e continua rodando.
- A diferença para o TrueNAS: lá o trabalho roda **no servidor remoto** e o poller só observa; no Ollama não há *job id* remoto. A geração (`POST /api/generate` ou `/api/chat`) precisa rodar numa **goroutine do próprio mcpollama**, com `context.Context` cancelável (isso resolve o item 2), gravando o texto acumulado e os contadores `prompt_eval_count`/`eval_count` no store. Não é preciso `Poller` nem `StatusMethod`.
- Reaproveitar: `TaskStore` (TTL, cursor, `CleanExpired`), `Task`/`TaskStatus`, o par de tools `tasks_get`/`tasks_list`, e o padrão "espera limitada + fallback" (`tools/core.go:510-516`, `:569-596`; `tools/audit.go:810-830`).
- Corrigir ao copiar: expor `Result` (item 1), proteger campos mutáveis com o lock do store (item 4), adicionar `tasks_cancel`.
- **Fila da GPU:** como só uma geração roda por vez, a task pode ficar em `working` com mensagem "na fila" até obter o semáforo; o TrueNAS não tem equivalente.
- Ressalva: se o cliente MCP suportar tarefas nativas do protocolo, `tasks_get` como tool continua funcionando em qualquer cliente (inclusive os que só falam `tools/*`). O servidor não anuncia nenhuma capability de tasks (`main.go:204-208`).

## Fluxo de um `tools/call`

1. `Scan()` lê uma linha de stdin (`main.go:140-141`; `bufio.Scanner` **sem** `Buffer()`, logo linhas acima de 64 KiB abortam o loop com `stdin error`, `main.go:168-170` — `grep` não achou `.Buffer(` em `cmd/ mcp/ proxy/`).
2. `json.Unmarshal` em `mcp.Request`; falha → `sendError(nil, -32700, ...)` (`:146-153`).
3. `handleRequest` despacha por `Method` (`:175-195`); notificação não tem resposta.
4. `handleToolsCall` re-serializa `Params` e decodifica `ToolCallParams` (`:231-241`; passo redundante marshal→unmarshal) e chama `registry.CallTool(name, args)` (`:244`).
5. `Registry.CallTool` procura no `map`; ausente → erro `unknown tool` (`tools/registry.go:20452-20461`); senão executa `Handler(client, args)`.
6. O handler valida argumentos manualmente (`args["x"].(string)`; números chegam como `float64`), chama `client.Call(metodo, params...)` (conecta e autentica na 1ª vez, `client.go:327-344`) e devolve **uma string** (JSON indentado).
7. Erro de handler → `ToolCallResult{IsError:true, Content:[text "Error: ..."]}` (`main.go:244-259`) — erro de **ferramenta**, não erro JSON-RPC. Sucesso → `Content:[text]` (`:261-272`).
8. `sendResponse` serializa e escreve `fmt.Printf("%s\n")` sob `stdoutMutex` (`:286-301`). Logs vão para stderr via `log` (padrão do pacote).
9. `callRaw` registra um canal em `pending[id]`, escreve no WebSocket e espera até **120 s** (`client.go:815-821`); uma nova tentativa só em erro de conexão (`:697`, `:829-841`).

## Pontos de atenção para o mcpollama

- O loop é **sequencial**: um `tools/call` lento bloqueia `ping`, `tools/list` e o próximo `tools/call`. O cliente WebSocket é seguro para chamadas concorrentes (`pending` com mutex, `client.go:48-53`), mas o loop não as aproveita.
- Sem cancelamento (`notifications/cancelled` não tratado) e sem `ping`.
- `Registry.tools` é um `map`: `ListTools` devolve ordem aleatória (`registry.go:20444-20450`); para `tools/list` estável (e cache de prompt melhor) ordenar por nome.
