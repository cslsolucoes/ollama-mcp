# ollama-mcp-go — tools e contratos

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Contagem de tools: **4**, medida em `ollama-mcp-go/main.go:172-230` e confirmada por `TestToolsList` (`ollama-mcp-go/main_test.go:43`). O `tools/list` completo ocupa **1.901 bytes** (linha JSON inteira, com `_meta`; medido rodando o servidor).

## Convenções comuns

- **Registro:** slice global `tools` (`ollama-mcp-go/main.go:172`), ordem fixa (o comentário em `ollama-mcp-go/main.go:254` diz que é para clientes e caches de prompt poderem confiar na ordem). Não há `--tools-verbosity`: a descrição é sempre a mesma (curta).
- **Esquema:** `mcpSchema{Type, Properties map[string]mcpProp, Required}` e `mcpProp{Type, Description, Enum, Default, Items}` (`ollama-mcp-go/main.go:61-74`). `Enum` e `Default` existem no tipo mas **nenhuma tool os usa**. Todos os valores são lidos de `map[string]any` por *type assertion* (`ollama-mcp-go/main.go:454-456`, `544-545`): tipo errado vira `""`, e depois "required".
- **Anotações** (`ollama-mcp-go/main.go:51-56`): `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`; sem `omitempty`, então os 4 booleanos saem sempre. Todas as tools: `readOnlyHint:true`, `destructiveHint:false`, `openWorldHint:false`; `idempotentHint:true` para `list_models` e `embed`, `false` para `generate` e `chat`. O comentário (`ollama-mcp-go/main.go:48-50`) justifica: roda um modelo local e não altera nada; `openWorldHint:false` porque "Ollama is local". **Para o mcpollama isso muda:** o Ollama da CSL é remoto (LAN/Cloudflare), embora privado; decidir o valor com critério próprio (inferência).
- **Destrutiva?** Nenhuma. Sem tools administrativas (`ollama-mcp-go/README.md:12`: "No admin tools (pull/push/delete)"). Não há gate `confirm`.
- **Sem `model` padrão:** `model` é obrigatório em `generate`, `chat` e `embed`. O README usa exemplos como `qwen3:14b`, `llama3:8b`, `nomic-embed-text` (`ollama-mcp-go/README.md:104-130`). O mcpollama tem default `qwen3-ia` (flag `--model`).
- **Sem `options`:** nenhuma tool aceita `temperature`, `num_ctx`, `num_predict`, `think`, `format`, `keep_alive`; os structs de request só têm os campos listados abaixo (`ollama-mcp-go/main.go:92-112`, `133-136`).

## Resultado de tool e erros

Estrutura (`ollama-mcp-go/main.go:76-88`): `content: [{type:"text", text}]`, `isError`, `resultType:"complete"`, `_meta.io.modelcontextprotocol/serverInfo`, e `prompt_eval_count` / `eval_count` no topo (só `generate` e `chat`).

| Situação | Formato | Fonte |
| --- | --- | --- |
| Sucesso | `content[0].text` com o texto | `ollama-mcp-go/main.go:594-598` |
| Erro de validação/rede/HTTP | `isError:true` + texto da falha, **dentro de `result`** | `ollama-mcp-go/main.go:600-605` |
| `params` não decodifica | erro JSON-RPC `-32602` "invalid params: ..." | `ollama-mcp-go/main.go:267-269`, `607-613` |
| Tool desconhecida | erro JSON-RPC `-32602` "unknown tool: X" | `ollama-mcp-go/main.go:282` |
| Método desconhecido | erro JSON-RPC `-32601` | `ollama-mcp-go/protocol.go:78-79` |
| Versão `_meta` não suportada | erro JSON-RPC `-32022` com `data.supported` | `ollama-mcp-go/protocol.go:56-62` |

Não há códigos de erro próprios, nem distinção entre erro de rede, 401 (token) e 403 (endpoint bloqueado): o corpo do Ollama é repassado como texto (`ollama-mcp-go/main.go:391`, `434`, `567`).

## 1. `list_models`

- **Descrição:** "List all available Ollama models" (`ollama-mcp-go/main.go:177`). Título "List models". Anotações: `readOnly`, `idempotent`.
- **`inputSchema`:** `{type:"object", properties:{}}` sem campos nem `required` (`ollama-mcp-go/main.go:178-181`).
- **Ação:** `GET <ollamaURL>/api/tags` (`ollama-mcp-go/main.go:426`), decodifica `{models:[{name,size,modified_at}]}` (`ollama-mcp-go/main.go:142-150`, `437-440`).
- **Saída:** texto, uma linha por modelo no formato `- <name> (<size/(1024*1024)> MB)` (`ollama-mcp-go/main.go:443-446`), por exemplo `- testmodel:7b (3814 MB)`; sem modelos: `No models found. Pull one with: ollama pull <model>` (`ollama-mcp-go/main.go:447-449`). `modified_at` é lido, mas **não** é impresso.
- **Erros:** falha de conexão `failed to reach Ollama: ...`; status ≠ 200 `Ollama returned N: <corpo>`; JSON inválido `failed to parse response: ...` (`ollama-mcp-go/main.go:427-440`).
- **Contadores:** nenhum (não há `prompt_eval_count`).
- **Tamanho:** `Size` é `int64` em bytes; a divisão inteira trunca.

## 2. `generate`

- **Descrição:** "Generate a one-shot response from an Ollama model" (`ollama-mcp-go/main.go:187`). Título "Generate". Anotações: `readOnly`, não idempotente.
- **`inputSchema`** (`ollama-mcp-go/main.go:188-196`):

| Campo | Tipo | Obrigatório | Default | Descrição (fonte) |
| --- | --- | --- | --- | --- |
| `model` | string | sim | nenhum | "Model name (e.g. qwen3:14b, llama3:8b)" |
| `prompt` | string | sim | nenhum | "The prompt to send" |
| `system` | string | não | vazio (omitido do request) | "Optional system prompt to set context/behavior" |

- **Request ao Ollama:** `POST /api/generate` com `{model, prompt, system (omitempty), stream:true}` (`ollama-mcp-go/main.go:92-97`, `462-467`).
- **Validação:** `model == "" || prompt == ""` → `'model' and 'prompt' are required` (`ollama-mcp-go/main.go:458-460`).
- **Saída:** texto da resposta inteira (tokens concatenados) em `content[0].text`, mais `prompt_eval_count` e `eval_count` no topo do resultado (`ollama-mcp-go/main.go:480-483`).
- **Erros:** `marshal error`, `Ollama request failed: ...`, `Ollama returned N: ...`, `failed to parse stream chunk: ...`, `stream read failed: ...` (`ollama-mcp-go/main.go:383-419`).
- **Efeito colateral:** escreve o prompt e a resposta no trace file (padrão: ligado; ver doc 30).

## 3. `chat`

- **Descrição:** "Send a multi-turn conversation to an Ollama model" (`ollama-mcp-go/main.go:202`). Título "Chat". Anotações: `readOnly`, não idempotente.
- **`inputSchema`** (`ollama-mcp-go/main.go:203-214`):

| Campo | Tipo | Obrigatório | Default | Descrição (fonte) |
| --- | --- | --- | --- | --- |
| `model` | string | sim | nenhum | "Model name (e.g. qwen3:14b, llama3:8b)" |
| `messages` | array de `object` | sim | nenhum | "Conversation messages, each with 'role' (system/user/assistant) and 'content'" |

  `items` é só `{type:"object"}`, **sem** as propriedades `role`/`content` declaradas (`ollama-mcp-go/main.go:210`): o esquema não ensina o formato ao cliente; a descrição em texto é que o faz.
- **Validação em ordem** (`ollama-mcp-go/main.go:487-518`): `model` vazio → `'model' is required`; `messages` ausente → `'messages' is required`; não é array → `'messages' must be an array`; elemento não é objeto → `each message must be an object with 'role' and 'content'`; `role` ou `content` vazios → `each message must have non-empty 'role' and 'content'`; lista vazia → `'messages' must not be empty` (cobre o caso do `messages: []`, sem *panic*, `ollama-mcp-go/main_test.go:473-484`).
- **Limites do contrato:** só `role` e `content` são repassados (`ollama-mcp-go/main.go:514`). `images`, `tool_calls`, `tool_name` e `thinking` de entrada são **descartados**. Não há validação do valor de `role` (qualquer string não vazia passa). `content` vazio é rejeitado (mesmo para `assistant`).
- **Request:** `POST /api/chat` com `{model, messages, stream:true}` (`ollama-mcp-go/main.go:108-112`, `520-524`).
- **Saída:** texto da resposta inteira + contadores (`ollama-mcp-go/main.go:537-540`). O texto do trace usa **só a última mensagem** como prompt (`ollama-mcp-go/main.go:529`).
- **Estado:** nenhum no servidor; o cliente reenvia o histórico a cada chamada (`ollama-mcp-go/README.md:30-32`).

## 4. `embed`

- **Descrição:** "Generate embeddings for a text using an Ollama model" (`ollama-mcp-go/main.go:220`). Título "Embed text". Anotações: `readOnly`, idempotente.
- **`inputSchema`** (`ollama-mcp-go/main.go:221-228`):

| Campo | Tipo | Obrigatório | Default | Descrição (fonte) |
| --- | --- | --- | --- | --- |
| `model` | string | sim | nenhum | "Embedding model name (e.g. nomic-embed-text)" |
| `text` | string | sim | nenhum | "Text to embed" |

- **Request:** `POST /api/embed` com `{model, input:<string>}` (`ollama-mcp-go/main.go:133-136`, `551-557`), sem streaming (`json.NewDecoder(resp.Body).Decode`, `ollama-mcp-go/main.go:570`). Só **um** texto por chamada (`input` é `string`; a API aceita lista, mas isso é conhecimento externo do Ollama, não está no código).
- **Saída:** **não devolve o vetor.** Devolve JSON em texto: `{"dimensions": N, "preview": [primeiros 16 valores]}` (`ollama-mcp-go/main.go:580-589`). Para RAG o cliente não consegue usar o resultado; a tool serve apenas para checar que o modelo responde (inferência).
- **Erros:** `no embeddings returned` se a lista vier vazia (`ollama-mcp-go/main.go:576-578`).
- **Contadores:** `ollamaEmbedResp` só decodifica `embeddings` (`ollama-mcp-go/main.go:138-140`), então o contador de tokens do embed **não** é devolvido.

## Como o streaming do Ollama é consumido e devolvido inteiro

`postStream` (`ollama-mcp-go/main.go:382-421`), usado por `generate` e `chat`:

1. `POST` com `stream:true`. Falha de conexão → `Ollama request failed: <erro>`; status ≠ 200 → lê o corpo inteiro (`io.ReadAll`, **sem limite de tamanho**) e devolve `Ollama returned N: <corpo>` (`ollama-mcp-go/main.go:383-392`).
2. `bufio.Scanner` sobre o corpo, buffer de linha de até 1 MiB (`ollama-mcp-go/main.go:394-395`). Cada linha é um JSON NDJSON; linhas vazias são puladas (`ollama-mcp-go/main.go:397-400`).
3. Cada linha vira `ollamaStreamChunk` (`ollama-mcp-go/main.go:118-124`). Um **único** struct cobre os dois endpoints: `response` (generate) ou `message.content` (chat); `token()` devolve `Response` se não vazio, senão `Message.Content` (`ollama-mcp-go/main.go:126-131`). Linha com JSON inválido aborta com `failed to parse stream chunk` (`ollama-mcp-go/main.go:402-404`).
4. Se `message.thinking` vier, vai **só** para o trace (`writeThinking`, `ollama-mcp-go/main.go:405-407`, `358-367`), nunca para a resposta (teste: `TestThinkingStaysOutOfReply`, `ollama-mcp-go/main_test.go:436-471`).
5. Token não vazio → `sink.write(tok)`: acumula em `strings.Builder` e espelha no arquivo (`ollama-mcp-go/main.go:343-353`, `410-412`).
6. No chunk `done:true`, guarda `prompt_eval_count` e `eval_count` (`ollama-mcp-go/main.go:413-415`). Os contadores só são lidos nesse chunk (o mock os coloca no final, `ollama-mcp-go/main_test.go:97`).
7. Ao fim, `scanner.Err()` (por exemplo, linha acima de 1 MiB, ou o corte do `Timeout` de 5 min) → `stream read failed` (`ollama-mcp-go/main.go:417-419`). **O timeout do `http.Client` (5 min) cobre a leitura do corpo inteiro**, então uma geração longa pode ser cortada no meio (inferência pela semântica de `http.Client.Timeout`).
8. A resposta devolvida ao cliente é `sink.text()`, a concatenação de todos os tokens (`ollama-mcp-go/main.go:377`, `480`).

Lacunas deste consumo, relevantes para o mcpollama (todas inferidas da leitura do struct/loop, não executadas):

- O chunk **não tem campo `error`**: se o Ollama enviar `{"error":"..."}` no meio de um stream `200`, o servidor devolveria texto vazio sem erro (`ollama-mcp-go/main.go:118-124`).
- `done_reason` não é lido: truncamento por contexto cheio ou `num_predict` não é sinalizado ao cliente.
- Se o stream terminar sem chunk `done:true`, os contadores saem 0 sem erro.
- Não há cancelamento: se o cliente desiste, a requisição ao Ollama continua até o fim (loop sequencial, `ollama-mcp-go/main.go:656`).
