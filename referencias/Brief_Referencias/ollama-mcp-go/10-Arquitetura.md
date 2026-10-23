# ollama-mcp-go — arquitetura

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Todas as citações são relativas a `referencias/`. Comportamentos marcados "(executado)" foram reproduzidos nesta onda com `go run .` e stdin controlado (`OLLAMA_MCP_TRACE=off`, Ollama indisponível em `localhost:11434`).

## Visão em camadas

Um único pacote `main`, em 2 arquivos de produção. Não há subpacotes.

| Camada | Onde | O que faz |
| --- | --- | --- |
| Entrypoint e loop stdio | `ollama-mcp-go/main.go:627-662` | lê linhas de stdin, decodifica JSON-RPC, ignora notifications, escreve 1 linha por resposta |
| Dispatch e protocolo | `ollama-mcp-go/protocol.go:53-109` | checa versão de `_meta`, roteia `server/discover`, `initialize`, `ping`, `tools/list`, `tools/call` |
| Handlers MCP | `ollama-mcp-go/main.go:234-292` | `handleInitialize`, `handleToolsList`, `handleToolsCall` |
| Implementação das tools | `ollama-mcp-go/main.go:425-590` | `toolListModels`, `toolGenerate`, `toolChat`, `toolEmbed` |
| Cliente Ollama + streaming | `ollama-mcp-go/main.go:382-421`, `159-170` | `http.Client` com timeout de 5 min; `postStream` lê NDJSON |
| Trace | `ollama-mcp-go/main.go:296-377` | espelha tokens em arquivo |
| Helpers | `ollama-mcp-go/main.go:594-623` | `textResult`, `errResult`, `errorResult`, `writeResponse` |

Não existe separação de cliente Ollama em pacote próprio: tipos de request/response do Ollama (`ollama-mcp-go/main.go:92-150`) e o `http.Client` ficam no `server` (`ollama-mcp-go/main.go:154-157`), junto das tools. Para o mcpollama, o molde CSL separa `src/ollama/` e `src/tools/`.

## Entrypoint (`main`)

`ollama-mcp-go/main.go:627-662`:

1. `log.SetOutput(os.Stderr)` e `log.SetFlags(log.Ltime)` (`ollama-mcp-go/main.go:628-629`): **todo log vai para stderr**, stdout fica só para JSON-RPC. Confirmado (executado): as linhas `ollama-mcp-go starting (...)` e `invalid JSON-RPC: ...` saíram em stderr e `out.txt` só continha respostas.
2. `newServer()` (`ollama-mcp-go/main.go:159-170`) lê **só** `OLLAMA_HOST` (default `http://localhost:11434`), remove `/` final e cria `http.Client{Timeout: 5*time.Minute}`. **Não há `flag`**: nenhum parse de argumentos, nem `--version` (`grep` por `flag.` no pacote: 0 ocorrências).
3. Loga o URL do Ollama no start (`ollama-mcp-go/main.go:632`), ou seja, o URL (com host) vai para o stderr do cliente.

## Loop stdio e framing

- **Framing: uma mensagem JSON por linha (NDJSON), sem `Content-Length`.** Leitura com `bufio.Scanner` sobre `os.Stdin` (`ollama-mcp-go/main.go:634`), buffer máximo de **4 MB** (`ollama-mcp-go/main.go:635`), escrita com `fmt.Fprintf(w, "%s\n", data)` + `Flush` (`ollama-mcp-go/main.go:621-622`). Alinhado ao transporte stdio do MCP (mensagens delimitadas por `\n`, sem `\n` embutido); `json.Marshal` nunca emite quebra de linha, então o framing é seguro na saída.
- **Linha vazia** (só espaços): `continue` (`ollama-mcp-go/main.go:640-642`).
- **JSON inválido:** `log.Printf("invalid JSON-RPC: %v")` e `continue` (`ollama-mcp-go/main.go:645-648`). **Nenhuma resposta `-32700` é enviada** (executado: a linha `not json` gerou só log em stderr e nada em stdout; as 5 linhas de entrada produziram 4 respostas de stdout, `id` 1, 2, 3 e 4). Um cliente que enviou algo malformado com `id` fica esperando.
- **Linha maior que 4 MB:** `scanner.Scan()` devolve `false` com `bufio.ErrTooLong`; o `for` termina, `main` loga `stdin error` (`ollama-mcp-go/main.go:659-661`) e **o processo sai** (inferência pela semântica de `bufio.Scanner`; não executado). Um `tools/call` com um prompt muito grande derruba o servidor.
- **EOF em stdin:** o loop termina e o processo sai com código 0 (`ollama-mcp-go/main.go:638-662`).
- **Sequencial:** cada requisição é tratada na própria goroutine do loop (`ollama-mcp-go/main.go:656`: `writeResponse(writer, s.dispatch(req))`). Enquanto `tools/call` espera o Ollama (até 5 min, `ollama-mcp-go/main.go:167`), **nenhuma outra linha é lida**: `ping`, `tools/list` e notifications como cancelamento ficam na fila do pipe. Para o Ollama da CSL (uma GPU lógica, fila única) isso é conveniente; para o cliente é uma limitação (inferência).
- **ID tratado como `json.RawMessage`** (`ollama-mcp-go/main.go:20`), então `id` numérico ou string volta idêntico na resposta.

## Notifications

- Critério: `req.ID == nil || string(req.ID) == "null"` (`ollama-mcp-go/main.go:651`). Qualquer mensagem sem `id` é tratada como notification: só `log.Printf("notification: %s", req.Method)` e `continue` (`ollama-mcp-go/main.go:651-654`).
- Consequências: `notifications/initialized` e `notifications/cancelled` são aceitas e **ignoradas** (a segunda não cancela nada); uma **requisição** sem `id` (erro do cliente) também é engolida em silêncio; `"id": null` é tratado como notification, e não como requisição inválida.
- A checagem de `_meta.protocolVersion` do `dispatch` **não** se aplica a notifications, porque elas nunca chegam ao `dispatch` (`ollama-mcp-go/main.go:651-656` vem antes de `ollama-mcp-go/protocol.go:53`).
- Teste: `TestStdioRoundTrip` inclui `notifications/initialized` e espera 4 respostas para 5 linhas (`ollama-mcp-go/main_test.go:498-541`), mas **reimplementa** o loop dentro do teste em vez de chamar `main` (`ollama-mcp-go/main_test.go:515-534`).

## Dispatch (`ollama-mcp-go/protocol.go:53-80`)

Ordem de avaliação:

1. `requestVersion(req.Params)` (`ollama-mcp-go/protocol.go:38-47`) procura `params._meta["io.modelcontextprotocol/protocolVersion"]` (`ollama-mcp-go/protocol.go:23`). Se vier, a requisição é **"moderna"**; se não, **"legada"**.
2. Se moderna e a versão não for `2026-07-28`: devolve erro `-32022` "Unsupported protocol version" com `data = {supported: [...5 versões...], requested: v}` (`ollama-mcp-go/protocol.go:56-62`).
3. `switch req.Method`:

| Método | Resposta | Fonte |
| --- | --- | --- |
| `server/discover` | `resultType`, `supportedVersions`, `capabilities.tools`, `instructions`, `_meta.serverInfo`, `ttlMs`, `cacheScope:"public"` | `ollama-mcp-go/protocol.go:65-66`, `82-93` |
| `initialize` | `protocolVersion` negociada, `capabilities.tools.listChanged=false`, `serverInfo` | `ollama-mcp-go/protocol.go:67-68`, `ollama-mcp-go/main.go:234-246` |
| `ping` | `{}` se legada; se moderna cai para "method not found" (a revisão 2026-07-28 removeu o `ping`, segundo o comentário em `ollama-mcp-go/protocol.go:51-52`) | `ollama-mcp-go/protocol.go:69-72` |
| `tools/list` | `resultType:"complete"`, `tools`, `ttlMs:3600000`, `cacheScope:"public"`, `_meta.serverInfo` | `ollama-mcp-go/protocol.go:73-74`, `ollama-mcp-go/main.go:248-260` |
| `tools/call` | resultado da tool (ver abaixo) | `ollama-mcp-go/protocol.go:75-76`, `ollama-mcp-go/main.go:262-292` |
| outro | erro `-32601` "method not found: X" | `ollama-mcp-go/protocol.go:78-79` |

Não existem: `resources/*`, `prompts/*`, `completion/*`, `logging/setLevel`, paginação (`nextCursor`) em `tools/list`.

Observação: `initialize` **não valida** `_meta` nem devolve `resultType`, e responde sempre com uma versão **legada** (`negotiate`, `ollama-mcp-go/protocol.go:97-109`), mesmo se o cliente mandar a moderna. Isso é coerente com o comentário "handleInitialize serves legacy clients" (`ollama-mcp-go/main.go:232-233`).

## Negociação de `protocolVersion`

- Constantes: moderna `2026-07-28` (`ollama-mcp-go/protocol.go:12`); legadas `2025-11-25`, `2025-06-18`, `2025-03-26`, `2024-11-05` (`ollama-mcp-go/protocol.go:16`); `supportedVersions` = moderna + legadas (5 itens, `ollama-mcp-go/protocol.go:20`).
- `negotiate` (`ollama-mcp-go/protocol.go:97-109`): devolve `params.protocolVersion` se estiver em `legacyVersions`; caso contrário devolve `legacyVersions[0]` = `2025-11-25`. Se o `JSON` de `params` for inválido ou vazio, também cai em `2025-11-25`.
- Execução (executado): `initialize` com `protocolVersion:"2025-06-18"` respondeu `"protocolVersion":"2025-06-18"`. Testes que fixam o contrato: `TestLegacyHandshake` (pedindo `1999-01-01` recebe `2025-11-25`, `ollama-mcp-go/protocol_test.go:64-72`).
- **Limite:** a especificação `2026-07-28` e os campos `resultType`, `ttlMs`, `cacheScope`, `server/discover` e o código `-32022` **não foram verificados contra a spec oficial nesta onda**; o brief só registra o que este código implementa e o que o autor afirma (`ollama-mcp-go/README.md:177-182`, `ollama-mcp-go/protocol.go:7-10`).
- Para o mcpollama: os clientes alvo (Claude Code, Cursor) abrem com `initialize` (inferência; não testado aqui). A negociação legada acima basta; o modo moderno é opcional.

## Fluxo de uma chamada `tools/call` ponta a ponta

Exemplo: `generate`.

1. **stdin** recebe uma linha `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"generate","arguments":{"model":"..","prompt":".."}}}` (`ollama-mcp-go/main.go:638-639`).
2. `json.Unmarshal` em `rpcRequest` (`ollama-mcp-go/main.go:644-648`, tipo em `ollama-mcp-go/main.go:18-23`). `id` presente, então não é notification (`ollama-mcp-go/main.go:651`).
3. `s.dispatch(req)` (`ollama-mcp-go/protocol.go:53`): sem `_meta`, é "legada"; método `tools/call` → `handleToolsCall` (`ollama-mcp-go/protocol.go:75-76`).
4. `handleToolsCall` (`ollama-mcp-go/main.go:262-292`): decodifica `{name, arguments map[string]any}`. Falha de decode → erro JSON-RPC `-32602` "invalid params: ..." (executado: `tools/call` **sem** `params` respondeu `-32602 invalid params: unexpected end of JSON input`).
5. `switch call.Name` (`ollama-mcp-go/main.go:272-283`): nome desconhecido → erro `-32602` "unknown tool: X" (`ollama-mcp-go/main.go:282`, `607-613`; **não** vira resultado `isError`). Aqui o JSON-RPC `-32602` é o código para tool desconhecida; os demais erros de negócio usam `isError`.
6. `toolGenerate(args)` (`ollama-mcp-go/main.go:453-484`): extrai `model`, `prompt`, `system` por *type assertion* silenciosa (`args["model"].(string)`); tipo errado vira string vazia, e então "required". Valida `model` e `prompt` não vazios (`ollama-mcp-go/main.go:458-460`).
7. Monta `ollamaGenerateReq{Stream:true}` e serializa (`ollama-mcp-go/main.go:462-471`).
8. `newStreamSink` abre/cria o trace (padrão ligado; `ollama-mcp-go/main.go:323-338`).
9. `postStream("/api/generate", body, sink)` (`ollama-mcp-go/main.go:382-421`): `POST` ao Ollama com `Content-Type: application/json`, **sem** `Authorization`; status ≠ 200 → erro `Ollama returned N: <corpo>`; senão lê NDJSON linha a linha, alimenta o `sink` com cada token e guarda os contadores do chunk `done:true`.
10. `sink.close(promptEval, eval)` escreve o rodapé e fecha o arquivo (`ollama-mcp-go/main.go:369-375`, `478`).
11. `textResult(sink.text())` + `PromptEvalCount`/`EvalCount` (`ollama-mcp-go/main.go:480-483`).
12. De volta ao `handleToolsCall`: `ResultType = "complete"` e `Meta = {serverInfo}` (**sobrescreve** qualquer `Meta` anterior, `ollama-mcp-go/main.go:284-285`), embrulha em `rpcResponse` (`ollama-mcp-go/main.go:287-291`).
13. `writeResponse` serializa e escreve **uma linha** em stdout com `\n` e `Flush` (`ollama-mcp-go/main.go:615-623`).

Saída observada (executado, Ollama indisponível): `{"jsonrpc":"2.0","id":3,"result":{"resultType":"complete","content":[{"type":"text","text":"Ollama request failed: Post \"http://localhost:11434/api/generate\": dial tcp ..."}],"isError":true,"_meta":{...}}}`. Notar: a mensagem de erro **inclui o URL completo do Ollama** e o erro do SO (em pt-BR no Windows).

## Resultado da tool (`mcpToolResult`, `ollama-mcp-go/main.go:81-88`)

Campos: `resultType`, `content[]`, `isError` (omitido se `false`), `_meta`, e **dois campos fora do padrão MCP no topo**: `prompt_eval_count` e `eval_count` (ambos `omitempty`, então 0 some do JSON). Um cliente que só entende `content` não os verá (inferência). Para um resultado de erro de rede, os contadores não aparecem.

## Pontos de atenção para o mcpollama

| # | Ponto | Fonte |
| --- | --- | --- |
| 1 | Nenhum `-32700` para JSON inválido | `ollama-mcp-go/main.go:644-648` |
| 2 | Linha > 4 MB encerra o servidor | `ollama-mcp-go/main.go:635`, `659-661` (inferência) |
| 3 | Loop sequencial; `notifications/cancelled` só é lida depois da chamada | `ollama-mcp-go/main.go:651-656` |
| 4 | Requisição sem `id` e `"id":null` são descartadas sem erro | `ollama-mcp-go/main.go:651-654` |
| 5 | Erro de rede expõe o URL do Ollama ao cliente | `ollama-mcp-go/main.go:384`, `428` |
| 6 | `Meta` é substituído, então não dá para acrescentar contadores em `_meta` sem alterar a linha 285 | `ollama-mcp-go/main.go:285` |
| 7 | `initialize` não devolve `instructions` (só `server/discover` devolve) | `ollama-mcp-go/main.go:234-246`, `ollama-mcp-go/protocol.go:87-88` |
