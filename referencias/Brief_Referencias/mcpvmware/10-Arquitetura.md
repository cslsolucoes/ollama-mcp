# MCPVMWare — arquitetura

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Todas as citações são relativas a `referencias/`. Medições marcadas "em cópia" foram feitas com uma cópia de `MCPVMWare/src` na pasta temporária da sessão (a referência não foi tocada).

## 1. Camadas

```
mcpvmware-mcp/main.go  → flags/env → modo de conexão → cliente(s) → NewRegistry → StdioHandler.Run
mcp/types.go           → tipos JSON-RPC 2.0 + MCP (à mão) + interface ToolRegistry
tools/registry.go      → Registry: mapa nome→Tool, filtro por modo e por profile, CallTool com recover
tools/discovery.go     → verbosity compact, profile, tools meta (describe_tool, list_profiles)
tools/destructive.go   → gate + confirm + audit (um wrapper por tipo de cliente)
tools/*.go             → tools por domínio (à mão e `generated_*`)
vmware/ workstation/ cloudaws/ → um cliente por produto
cmd/dumptools/         → dump offline do catálogo (alimenta TOOLS.md)
gen/                   → classificador de métodos do govmomi (Fase 0, uso único)
```

Dependência de pacotes: `main` importa `mcp`, `tools` e os 3 clientes (`mcpvmware-mcp/main.go:19-23`); `tools` importa `mcp` e os clientes (`tools/registry.go:13-16`); `mcp` não importa nada. A interface que o `main` conhece é só `mcp.ToolRegistry` com `ListTools()` e `CallTool(name, args) (string, error)` (`mcp/types.go:70-74`). O handler stdio fica desacoplado do domínio: a tool devolve **string** e o handler embrulha em `content[0].text`.

## 2. Entrypoint

`mcpvmware-mcp/main.go:125-300`, na ordem:

1. `flag.Parse()` (`:126`); `--version` imprime `mcpvmware-mcp version <Version>` e sai (`:128-131`). `Version` é `var` com default `"dev"`, sobrescrita por `-ldflags "-X main.Version=..."` (`:63-65`).
2. Fallback para env, flag a flag (`:133-165`), todos com a mesma regra: flag vazia → lê a env. Booleanos aceitam tudo exceto `""`, `0`, `false`, `no`, `off` (`:143-159`). Tabela completa em [30](30-Configuracao_e_Seguranca.md).
3. `resolveConnectionMode()` (`:70-123`): exatamente 1 de 5 flags de conexão, ou `--all-url` sozinha (que aceita `--workstation-url` e `--cloud-aws-url` como secundárias). `log.Fatalf` em 0 ou 2+.
4. Validação de credenciais (`:169-175`) e avisos em **stderr** para `--insecure` e `--allow-destructive` (`:176-181`). O pacote `log` do Go escreve em stderr por padrão; o stdout fica exclusivo do JSON-RPC.
5. **Conexão ao backend antes do loop** (`:191-269`): `vmware.NewClient` faz login SOAP; falha → `log.Fatalf` e processo morre. Ver §8 (armadilha).
6. Valida `--tools-verbosity` (`:271-277`) e quebra `--profile` em lista (`:279-284`).
7. `tools.NewRegistry(ctx, client, RegistryOptions{...})` (`:286-294`) e `NewStdioHandler(registry, debug).Run()` (`:296-299`). O `ctx` é `context.Background()` (`:184`): **não há deadline por chamada nem cancelamento**.

## 3. Loop stdio

`StdioHandler` (`main.go:303-316`) guarda `bufio.NewScanner(os.Stdin)` **sem** `Buffer()`, um mutex de escrita e o flag de debug. `Run()` (`:318-351`) lê **uma linha por requisição**, faz `json.Unmarshal` em `mcp.Request`, chama `handleRequest` e escreve a resposta, tudo **sequencial** (uma requisição por vez; uma tool lenta bloqueia as demais). Não há goroutine por requisição, apesar do mutex de `stdout` (`:306, 423-424`), que então só protege de uso futuro.

Respostas saem por `fmt.Printf("%s\n", data)` (`:433`) sob o mutex. Com `--debug`, cada linha de entrada e saída é logada em stderr (`:325-327, 430-432`), **inclusive os argumentos das tools** (ver [30](30-Configuracao_e_Seguranca.md) §6).

### Comportamentos medidos (em cópia, binário real em modo `--workstation-url` apontando para porta morta, que não faz I/O no boot)

| Entrada | Saída observada | Evidência de código |
| --- | --- | --- |
| `initialize` com `protocolVersion:"2025-06-18"` | responde `"2024-11-05"` fixo, `capabilities.tools.listChanged:false`, `serverInfo` | `main.go:371-380` |
| `notifications/initialized` | nenhuma saída | `:357-358` |
| `ping` (com `id`) | `{"error":{"code":-32601,"message":"Method not found"}}` | `:363-367` |
| método desconhecido sem `id` | nenhuma saída | `:364-367` |
| `resources/list` (com `id:0`) | `-32601`, `id:0` preservado | `mcp/types.go:10` |
| linha que não é JSON | `{"jsonrpc":"2.0","error":{"code":-32700,...}}` **sem o campo `id`** (a spec pede `"id":null`) | `main.go:330-333`, `types.go:17` (`omitempty`) |
| `tools/call` de tool inexistente | `result.isError:true`, texto `Error: unknown tool: nope` | `registry.go:439-442`, `main.go:397-407` |
| linha de **30 KB** | processada normalmente | — |
| linha de **70 KB** | processa as anteriores, depois `Stdio handler error: stdin error: bufio.Scanner: token too long` em stderr, **exit 1** | `main.go:310-316, 347-349` |

Os dois últimos são os mais caros para o mcpollama: prompts grandes passam de 64 KB com facilidade (ctx 32k tokens ≈ 100+ KB de texto). [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]

## 4. Dispatch

`handleRequest` (`main.go:353-369`) é um `switch req.Method` com 4 casos: `initialize`, `notifications/initialized`, `tools/list`, `tools/call`. Default: erro `-32601` **só se há `id`**; sem `id` (notificação) ignora. Não existem `ping`, `prompts/*`, `resources/*`, `logging/*`, `notifications/cancelled`.

`handleToolsCall` (`:387-416`) faz **re-marshal** de `req.Params` (`map[string]interface{}`) para bytes e `Unmarshal` em `mcp.ToolCallParams` (`:389-395`); erro nisso vira `-32602`. Depois `registry.CallTool(name, arguments)`. Erro de handler vira resultado com `isError:true` e texto `Error: <msg>`; sucesso vira `content:[{type:"text",text:<string>}]`. Não há `structuredContent`, anotações (`readOnlyHint` etc.) nem `title`.

## 5. Registry

`Registry` (`tools/registry.go:20-48`) guarda `client`, `wsClient`, `cloudClient`, `ctx`, `tools map[string]Tool`, flags do gate destrutivo, `connectionMode`, `currentClass` (classe do `registerXTools` em curso), `verbosity` e `profiles`.

- **Registro:** `registerTools()` (`:243-358`) faz 89 chamadas a `withClass(classe, fn)` (contadas com `grep -c`) (`:363-366`), que define `r.currentClass` antes de cada uma. `register()` (`:372-380`) só adiciona a tool se `connectionModeAllows(modo, classe)` **e** `profileAllows(nome)`; caso contrário a tool **nem existe** (não aparece em `tools/list`, e `CallTool` vê "unknown tool").
- **Classes (`toolMode`, `:154-166`):** `vcenter-only`, `vsphere-general`, `workstation`, `cloud-aws`, `meta`. `meta` passa em todos os modos (`:171-173`).
- **Modos (`ConnectionMode`, `:104-146`):** `vcenter`, `vmware`, `workstation`, `all`, `cloud-aws`, `everything`, e `""` = tudo (default de testes). Matriz em `connectionModeAllows` (`:170-192`).
- **Despacho por cliente (`Tool`, `:202-207`):** três campos de handler (`Handler`, `WSHandler`, `CloudHandler`), com **exatamente um** preenchido. `CallTool` (`:438-461`) escolhe pelo campo não nulo e devolve erro claro se o cliente secundário não foi conectado (`:448-457`). A decisão de não generalizar para `interface{}` está registrada em `Tool` (`:194-201`) e em `cerebrum.md:415-426`.
- **`CallTool` tem `defer recover()`** (`:443-447`): um panic em handler vira `error`, o servidor não cai. Origem: nil-pointer real contra ESXi standalone (comentário `:425-437`).
- **`ListTools()`** (`:409-423`): copia as definições, aplica `compactDefinition` se `verbosity==compact` (`:415-418`), **ordena por nome** (`:421`). A ordenação torna o `tools/list` determinístico, necessário para comparar versões e para o `TOOLS.md`.

## 6. Discovery: verbosity, profile, tools meta

`tools/discovery.go`. Tudo **aditivo e opt-in**; sem flags o payload é idêntico ao histórico (`:29-30`, teste `TestListTools_OmissaoEIgualAFull`, `discovery_test.go:139`).

- **Verbosity** (`:44-55`): `full` (ou vazio) e `compact`. `compactDefinition` (`:193-201`) faz duas coisas: `firstSentence(description)` e `stripPropertyDescriptions(inputSchema)`. Nunca toca `type`, `required`, `properties`, `enum`, `default`.
- **`firstSentence`** (`:86-114`): corta no primeiro `[.!?]\s` (`:68`), mas rejeita candidatos quando o caractere antes do ponto é dígito (versões "7.0.3"), quando a palavra é abreviatura conhecida (`:72-76`) ou quando o corte deixa menos de 15 caracteres (`:108`). O limiar 25 → 15 foi corrigido porque engolia "Is the VM a template?" (`discovery.go:104-107`, plano B1 §10.1).
- **`stripPropertyDescriptions` / `stripNodeDescription`** (`:123-189`): **cópia profunda** sem `description` nas propriedades, em qualquer nível (incluindo `items` e `properties` aninhadas). Copiar é obrigatório porque o Registry guarda a definição completa para `describe_tool` (`:116-121`).
- **Profile** (`:206-221`): grupo = 2º segmento do nome (`vmware_host_info` → `host`), derivado do próprio nome de propósito, para não desincronizar de uma lista curada (`:203-205`). `--profile` filtra **no registro**, junto do filtro por modo. As tools meta ficam fora do filtro (`:57-63`, `:217`).
- **Tools meta** (`:226-292`): `vmware_describe_tool(name)` devolve nome, descrição **completa**, `inputSchema` **completo** e profile da definição guardada; `vmware_list_profiles` conta o que está **registrado** (não o catálogo). Registradas em todos os modos (`modeMeta`).
- **Por que não uma meta-tool de invocação (`invoke_tool`)** (plano B1 §4, `b1-desenho...plan.md:94-104`): quebraria a validação de schema do cliente e o modelo de invocação; recusada.

## 7. Fluxo de um `tools/call`

1. Cliente envia uma linha JSON `{"method":"tools/call","params":{"name":"vmware_vm_power_off","arguments":{...}}}`.
2. `Run` lê a linha (`Scanner`, limite 64 KB), `Unmarshal` em `mcp.Request` (`main.go:329-333`).
3. `handleRequest` → `handleToolsCall` (`:361-362, 387`): re-marshal de `params` → `ToolCallParams`.
4. `Registry.CallTool` (`registry.go:438`): busca no mapa; escolhe `Handler`/`WSHandler`/`CloudHandler`; instala o `recover`.
5. Se a tool foi registrada por `registerDestructive`, o handler já vem embrulhado (`destructive.go:52-54, 177-215`): gate de servidor → `confirm` booleano estrito → handler real → audit (permitido ou negado).
6. O handler resolve os argumentos (`requiredStringArg`, `resolveVM`...), chama o backend com `r.ctx` (sem timeout) e devolve `marshalJSON(map...)` (JSON indentado em 2 espaços, `system.go:91-100`).
7. `handleToolsCall` embrulha em `ToolCallResult` (`:409-415`) ou, em erro, em `isError:true` com `Error: <msg>` (`:399-407`).
8. `sendResponse` serializa e escreve em stdout sob mutex (`:422-435`).

## 8. Armadilhas de arquitetura (para o mcpollama evitar)

1. **Conexão ao backend no boot com `log.Fatalf`** (`main.go:217-226, 258-268`). O cliente MCP só vê "Connection closed" quando o backend está fora; o próprio dono do projeto registrou isso (`MCP-GAPS-VMWARE.md:61-63`). Para o mcpollama (Ollama pode estar fora ou atrás do Cloudflare): subir sempre, responder `initialize`/`tools/list` e falhar na chamada.
2. **Sem timeout por chamada** (`main.go:184`; `LIC-VMW-004`). Em tools do mcpollama (`generate` pode levar 40–90 s e entrar em fila de GPU) é preciso `context.WithTimeout` por chamada e uma flag.
3. **Processamento sequencial** (`main.go:323-345`). Combinado com o item 2, uma chamada pendurada trava o servidor inteiro, inclusive `ping` (que de todo modo não existe).
4. **Scanner com limite de 64 KB** (medido, §3).
5. **Versão de protocolo fixa** (`main.go:373`): ignora a que o cliente enviou.
6. **Erros de parse sem `id:null`** (medido, §3).
7. **Ordem de inicialização do cliente HTTP** não é problema aqui, mas há uma lição de ordem no cliente SOAP: o keepalive precisa envolver o transporte **antes** do login (`vmware/client.go:51-64, 84-91`). Análogo para qualquer *round tripper* que o mcpollama coloque no cliente do Ollama (trace, retry): instalar antes da primeira chamada.
