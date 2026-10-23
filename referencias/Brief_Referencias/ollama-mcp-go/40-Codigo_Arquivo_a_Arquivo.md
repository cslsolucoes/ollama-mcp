# ollama-mcp-go — código arquivo a arquivo

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Linhas físicas medidas com `wc -l`. Todos os números de linha abaixo vêm da leitura do arquivo com numeração.

## Dependências (`ollama-mcp-go/go.mod:1-3`)

```
module github.com/KTCrisis/ollama-mcp-go
go 1.24.2
```

Nenhuma diretiva `require`: **0 dependências externas**. Só biblioteca padrão: `bufio`, `bytes`, `encoding/json`, `fmt`, `io`, `log`, `net/http`, `os`, `strings`, `time` (`ollama-mcp-go/main.go:3-14`) e `encoding/json` em `protocol.go` (`ollama-mcp-go/protocol.go:3-5`). O README diz "Requires Go 1.22+" (`ollama-mcp-go/README.md:44`), mas o `go.mod` exige `go 1.24.2`: **inconsistência** entre os dois.

## `ollama-mcp-go/main.go` (662 linhas)

**Papel:** tudo que não é protocolo: tipos, tools, cliente Ollama, streaming, trace, loop. Pacote `main`.

### Tipos

| Tipo | Linhas | Observação |
| --- | --- | --- |
| `rpcRequest` | `ollama-mcp-go/main.go:18-23` | `ID`, `Params` como `json.RawMessage` |
| `rpcResponse` | `ollama-mcp-go/main.go:25-30` | `Result any`, `Error *rpcError`, ambos `omitempty` |
| `rpcError` | `ollama-mcp-go/main.go:32-36` | `code`, `message`, `data` |
| `mcpTool` | `ollama-mcp-go/main.go:40-46` | `name`, `title`, `description`, `inputSchema`, `annotations` |
| `mcpAnnotations` | `ollama-mcp-go/main.go:51-56` | 4 booleanos sem `omitempty` |
| `mcpSchema` | `ollama-mcp-go/main.go:61-65` | `type`, `properties`, `required` |
| `mcpProp` | `ollama-mcp-go/main.go:67-74` | `type`, `description`, `enum`, `default`, `items` |
| `mcpContent` | `ollama-mcp-go/main.go:76-79` | `type`, `text` (só texto; sem `image`/`resource`) |
| `mcpToolResult` | `ollama-mcp-go/main.go:81-88` | `resultType`, `content`, `isError`, `_meta`, `prompt_eval_count`, `eval_count` |
| `ollamaGenerateReq` | `ollama-mcp-go/main.go:92-97` | `model`, `prompt`, `system`, `stream` |
| `ollamaChatMessage` | `ollama-mcp-go/main.go:99-106` | `role`, `content`, `thinking` (`omitempty`, só recebido) |
| `ollamaChatReq` | `ollama-mcp-go/main.go:108-112` | `model`, `messages`, `stream` |
| `ollamaStreamChunk` | `ollama-mcp-go/main.go:118-124` | `response`, `message`, `done`, contadores; **sem `error`**, sem `done_reason` |
| `ollamaEmbedReq` / `ollamaEmbedResp` | `ollama-mcp-go/main.go:133-140` | `input` string; `embeddings [][]float64` |
| `ollamaModel` / `ollamaTagsResp` | `ollama-mcp-go/main.go:142-150` | `name`, `size`, `modified_at` |
| `server` | `ollama-mcp-go/main.go:154-157` | `ollamaURL`, `client *http.Client` |
| `streamSink` | `ollama-mcp-go/main.go:314-318` | arquivo, `strings.Builder`, flag `inThinking` |

Constantes e variáveis: `version = "0.2.0"` (`ollama-mcp-go/main.go:59`), `defaultTracePath` (`ollama-mcp-go/main.go:309`), slice `tools` (`ollama-mcp-go/main.go:172-230`).

### Funções e métodos

| Símbolo | Linhas | O que faz |
| --- | --- | --- |
| `(ollamaStreamChunk).token` | `ollama-mcp-go/main.go:126-131` | `Response` se não vazio, senão `Message.Content` |
| `newServer` | `ollama-mcp-go/main.go:159-170` | lê `OLLAMA_HOST`, cria `http.Client` com 5 min |
| `(*server).handleInitialize` | `ollama-mcp-go/main.go:234-246` | resposta de `initialize` legado |
| `(*server).handleToolsList` | `ollama-mcp-go/main.go:248-260` | lista fixa + `ttlMs`/`cacheScope` |
| `(*server).handleToolsCall` | `ollama-mcp-go/main.go:262-292` | decodifica e roteia; define `resultType` e `_meta` |
| `traceFile` | `ollama-mcp-go/main.go:298-307` | resolve caminho do trace (env/`off`/padrão) |
| `newStreamSink` | `ollama-mcp-go/main.go:323-338` | abre trace (append, `0o644`), escreve cabeçalho |
| `(*streamSink).write` | `ollama-mcp-go/main.go:343-353` | acumula token + espelha; fecha bloco de "pense" |
| `(*streamSink).writeThinking` | `ollama-mcp-go/main.go:358-367` | só espelha raciocínio |
| `(*streamSink).close` | `ollama-mcp-go/main.go:369-375` | rodapé `[done: ...]` e `Close` |
| `(*streamSink).text` | `ollama-mcp-go/main.go:377` | `buf.String()` |
| `(*server).postStream` | `ollama-mcp-go/main.go:382-421` | POST + leitura NDJSON + contadores |
| `(*server).toolListModels` | `ollama-mcp-go/main.go:425-451` | `GET /api/tags` |
| `(*server).toolGenerate` | `ollama-mcp-go/main.go:453-484` | `/api/generate` em stream |
| `(*server).toolChat` | `ollama-mcp-go/main.go:486-541` | validação de `messages` + `/api/chat` em stream |
| `(*server).toolEmbed` | `ollama-mcp-go/main.go:543-590` | `/api/embed`, devolve dimensões + 16 valores |
| `textResult`, `errResult` | `ollama-mcp-go/main.go:594-605` | construtores de resultado |
| `(*server).errorResult` | `ollama-mcp-go/main.go:607-613` | erro JSON-RPC `-32602` |
| `writeResponse` | `ollama-mcp-go/main.go:615-623` | marshal + `\n` + `Flush`; erro de marshal só loga |
| `main` | `ollama-mcp-go/main.go:627-662` | loop stdio |

### Particularidades de código (úteis ao mcpollama)

- `Result any` com `omitempty` (`ollama-mcp-go/main.go:28`): uma resposta sem `Result` nem `Error` omitiria ambos. O código nunca produz esse caso (todo `dispatch` devolve um dos dois).
- `handleToolsList` devolve o slice `tools` direto (`ollama-mcp-go/main.go:254`); o teste faz *type assertion* para `[]mcpTool` (`ollama-mcp-go/main_test.go:42`), logo a lista é tipada, e não um `map`.
- `s.errorResult` e `errResult` têm nomes quase iguais com semânticas diferentes (erro JSON-RPC versus resultado `isError`): fonte de confusão (`ollama-mcp-go/main.go:600-613`).
- `writeResponse` ignora o erro de `Flush` (`ollama-mcp-go/main.go:622`); em stdout quebrado o servidor segue lendo.
- O `http.Client` compartilhado é reaproveitado entre chamadas (conexões keep-alive) (`ollama-mcp-go/main.go:166-168`).

## `ollama-mcp-go/protocol.go` (109 linhas)

**Papel:** versionamento "dual-era" do MCP e roteamento de métodos.

| Símbolo | Linhas | O que faz |
| --- | --- | --- |
| Comentário de contexto | `ollama-mcp-go/protocol.go:7-10` | explica modernos (`_meta`) versus legados (`initialize`) |
| `modernVersion`, `legacyVersions`, `supportedVersions` | `ollama-mcp-go/protocol.go:12`, `16`, `20` | `2026-07-28`; 4 legadas; união (5) |
| Constantes `metaVersion`, `metaServerInfo`, `errUnsupportedVersion` (`-32022`), `toolsTTLMs` (`3600000`) | `ollama-mcp-go/protocol.go:22-32` | chaves de `_meta`, código de erro, TTL de 1 h |
| `serverInfo` | `ollama-mcp-go/protocol.go:34` | `{"name":"ollama-mcp-go","version":version}` |
| `requestVersion` | `ollama-mcp-go/protocol.go:38-47` | lê `params._meta[protocolVersion]` |
| `(*server).dispatch` | `ollama-mcp-go/protocol.go:53-80` | checa versão, `switch` de métodos |
| `(*server).handleDiscover` | `ollama-mcp-go/protocol.go:82-93` | `server/discover` com `instructions` |
| `negotiate` | `ollama-mcp-go/protocol.go:97-109` | versão legada de `initialize` |

Notas: `append([]string{modernVersion}, legacyVersions...)` cria um novo slice, sem alias com `legacyVersions` (`ollama-mcp-go/protocol.go:20`). O texto de `instructions` (`ollama-mcp-go/protocol.go:87-88`) diz "Nothing leaves the machine", falso para um Ollama remoto como o da CSL: não reutilizar como está.

## `ollama-mcp-go/main_test.go` (562 linhas)

**Papel:** testes das tools com mock `httptest`, testes de streaming/trace, e um round-trip de stdio. Detalhe no doc 50.

| Símbolo | Linhas |
| --- | --- |
| `TestInitialize`, `TestToolsList`, `TestToolsCallUnknown` | `ollama-mcp-go/main_test.go:17-69` |
| `TestMain` (força `OLLAMA_MCP_TRACE=off`) | `ollama-mcp-go/main_test.go:73-76` |
| `writeNDJSON` (emite NDJSON no formato do Ollama) | `ollama-mcp-go/main_test.go:83-104` |
| `mockOllama` (servidor falso de `/api/tags`, `/generate`, `/chat`, `/embed`) | `ollama-mcp-go/main_test.go:106-159` |
| `testServer` (monta `*server` apontando para o mock) | `ollama-mcp-go/main_test.go:161-169` |
| Testes de tools | `ollama-mcp-go/main_test.go:171-356` |
| Testes de streaming/trace/thinking/mensagens vazias | `ollama-mcp-go/main_test.go:362-484` |
| `TestStdioRoundTrip` | `ollama-mcp-go/main_test.go:488-562` |

## `ollama-mcp-go/protocol_test.go` (93 linhas)

| Símbolo | Linhas |
| --- | --- |
| Helper `call` (decodifica JSON bruto, chama `dispatch`, devolve `map`) | `ollama-mcp-go/protocol_test.go:8-18` |
| `modernMeta` | `ollama-mcp-go/protocol_test.go:20` |
| `TestDiscover`, `TestModernToolsList`, `TestUnsupportedModernVersion`, `TestLegacyHandshake`, `TestPingOnlyForLegacy`, `TestToolsCallCarriesResultType` | `ollama-mcp-go/protocol_test.go:22-93` |

## Demais arquivos (não Go)

| Arquivo | Linhas | Conteúdo |
| --- | ---: | --- |
| `ollama-mcp-go/README.md` | 186 | motivação (`7-14`), tools (`16-32`), instalação (`34-48`), setup Claude Code (`50-77`) e agent-mesh (`79-93`), exemplos (`95-134`), configuração (`136-141`), trace (`143-173`), protocolo (`175-182`), licença (`184-186`) |
| `ollama-mcp-go/go.mod` | 3 | ver acima |
| `ollama-mcp-go/.gitignore` | 3 | `ollama-mcp-go`, `*.exe`, `CLAUDE.md` (o binário e um `CLAUDE.md` local do autor) |
| `ollama-mcp-go/.git/` | n/d | histórico de 8 commits (`git log --oneline`); não analisado além disso |

Linhas do README vêm de `wc -l`, e os intervalos, da leitura com numeração. Segredos: nenhum encontrado em nenhum arquivo (a única URL de rede privada é o exemplo `http://192.168.1.10:11434` em `ollama-mcp-go/README.md:72`, endereço de exemplo, não da CSL).
