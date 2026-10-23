# truenas-mcp — Tools e contratos

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## Como as tools são declaradas

- **Um único lugar de declaração:** `MCPNas/truenas-mcp/tools/registry.go` (26.107 linhas) contém as **730** atribuições `r.tools["nome"] = Tool{Definition: mcp.Tool{...}, Handler: ...}` dentro de `registerTools()` (`registry.go:36` até ~`:20442`). Os handlers ficam nos arquivos por família (`tools/boot.go` etc.); só a declaração é centralizada. (Medido: `grep -c '^\s*r\.tools\["' tools/*.go` dá 730 em `registry.go` e 0 nos demais.)
- Tipo: `Tool{Definition mcp.Tool; Handler func(*truenas.Client, map[string]interface{}) (string, error)}` (`registry.go:21-24`). Handlers que precisam de `taskManager` são **métodos** de `*Registry` (`r.handleBootAttach`); os demais são funções soltas.
- O `InputSchema` é um `map[string]interface{}` escrito à mão (JSON Schema simples: `type`, `properties`, `required`, `description`, `default`, `minLength`, `enum`). Exemplo: `boot_attach` em `registry.go:1434-1460` (campos `dev` obrigatório, `expand` e `dry_run` opcionais).
- Convenção de nome: método do TrueNAS com pontos trocados por `_` (`boot.attach` → `boot_attach`). Limite prático de 60 caracteres entre identificador do servidor e nome (bug-017); maior nome atual: 44 caracteres (`.wolf/buglog.json`, bug-017).
- Descrições são **longas** (frequente passar de 500 caracteres, com RBAC, riscos e "Supports dry_run"; ex.: `registry.go:1437`). Isso explica os 892.704 bytes de `tools/list`.

## Convenções de entrada

- Validação manual por asserção de tipo: `dev, ok := args["dev"].(string); if !ok || dev == ""` → `fmt.Errorf("dev is required")` (`tools/boot.go:212-216`). Números JSON chegam como `float64` (`registry.go:24007-24020`, `args["limit"].(float64)`).
- Sem biblioteca de validação de schema: o servidor **não** valida `inputSchema` contra os argumentos; a validação é de cada handler. Há helpers por família (ex.: `poolCopyInt` com sentinela `2147483647` para "sem limite", bug-005).
- Campo `dry_run: boolean` em quase toda escrita arriscada (ver abaixo) e `confirm: boolean` quando a resposta contém segredo em texto claro.

## Convenções de saída

- O handler devolve **uma string**, quase sempre `json.MarshalIndent(map, "", "  ")` (`tools/boot.go:268-272`; helper `marshalJSON` em `tools/dryrun.go:79-85`). A string vira um único `ContentBlock{type:"text"}` (`main.go:261-272`). Não há `structuredContent`.
- Escritas assíncronas devolvem o mesmo envelope: `{..., "job_id", "task_id", "task_status", "poll_interval", "message": "... Track progress with tasks_get using task_id: ..."}` (`tools/boot.go:254-266`, `registry.go:22812-22830`).
- Tools de leitura "simplificam" a resposta bruta (ex.: `simplifyDataset`, `simplifySnapshot`, `simplifyVM`, `registry.go:20805`, `:21075`, `:21300`), para gastar menos tokens do que o JSON cru do middleware (**inferência** pelo nome e uso).

## Convenções de erro

- Erro de handler → `isError:true` com texto `Error: <mensagem>` (`main.go:244-259`). Tool inexistente cai no mesmo caminho (medido: `Error: unknown tool: nope`, `isError:true`).
- Erro JSON-RPC (`-32700`, `-32602`, `-32601`) só para falha de protocolo (`main.go:151,236,241,190`).
- Erros do backend vêm formatados com contexto: `API error: <msg> (code N)` + `Request: Method/Params` (parâmetros **redigidos**) + `Trace` (com a chave removida) (`truenas/client.go:856-878`). A mensagem de erro de TLS acrescenta o conselho de `--tls-ca`/`--insecure` (`client.go:145-147`).
- Pré-checagens de segurança retornam erro acionável antes de chamar o backend (ex.: `boot_attach` recusa quando o pool já tem 2 discos, `tools/boot.go:217-231`).

## Dry-run (contrato transversal)

`tools/dryrun.go` (85 linhas): interface `DryRunnable{ExecuteDryRun(client, args) (*DryRunResult, error)}` (`:10-12`), `DryRunResult{Tool, CurrentState, PlannedActions[], Warnings, Requirements, EstimatedTime}` (`:15-46`) e `ExecuteWithDryRun(client, args, dryRunnable, normalHandler)` (`:50-76`), que desvia para a prévia quando `args["dry_run"] == true`. Cada tool de escrita arriscada registra o handler `...WithDryRun` (ex.: `r.handleBootAttachWithDryRun`, `registry.go:1458`); `grep` achou 311 linhas `ExecuteDryRun(client` no pacote `tools` e 347 ocorrências de `WithDryRun` em `registry.go`. Para o mcpollama o análogo seria um `dry_run` em `summarize_file`/`extract` que mostra **quais arquivos seriam lidos e quantos bytes/tokens estimados** sem chamar o modelo (**sugestão**, não existe na referência).

## Famílias em `tools/`

`tools/` tem 102 arquivos `.go`: 94 de código (94 = 102 − 8 de teste). As tools por prefixo (medido com `grep -o 'r\.tools\["[a-z0-9_]*' | awk -F_ '{print $1}'`), as maiores:

| Prefixo | Tools | Arquivos de handler (exemplos) |
| --- | ---: | --- |
| `pool` | 65 | `pool.go` (2.772 linhas), `pool_dataset.go` (2.188), `snapshot.go`, `snapshottask.go`, `scrub_handlers.go` |
| `vm` | 49 | `vm.go` (1.834), `vm_device.go` (1.673) |
| `system` | 49 | `system_advanced.go`, `system_general.go`, `truenas_system.go` e handlers em `registry.go` |
| `iscsi` | 39 | `iscsi.go` (4.231 linhas, 1 arquivo) |
| `nvmet` | 36 | 7 arquivos `nvmet_*.go` (357 a 864 linhas) |
| `app` | 29 | `app.go`, `apps.go` |
| `interface` | 22 | `interface.go` |
| `cloudsync` | 20 | `cloudsync.go`, `cloudsync_credentials.go` |
| `boot` | 7 | `boot.go` (808) |
| `tasks` | 2 | handlers em `registry.go:24007-24046` |

(Somam mais de 60 prefixos; o catálogo completo não é objetivo deste brief. Há um relatório gerado com todas as tools, `MCPNas/truenas-mcp/.workspace/reports/truenas-mcp2026-08-03-015307-tools-reference.report.md`, 2.253 linhas, defasado: cobre 305 tools.)

## Três famílias em detalhe

### 1. `tasks` (2 tools) — acompanhar trabalho longo

- `tasks_list` (`registry.go:4880-4900`): `cursor` (string) e `limit` (inteiro, padrão 50); resposta `{tasks:[...], next_cursor?}` (`:24007-24033`).
- `tasks_get` (`registry.go:4902-4920`): `task_id` obrigatório; devolve o `Task` serializado, **sem** `Result` (`tasks/types.go:42`).
- Contrato do `Task` visível ao cliente: `taskId`, `status`, `statusMessage`, `createdAt`, `lastUpdatedAt`, `ttl` (s), `pollInterval` (s) (`tasks/types.go:27-34`).
- Leitura para o mcpollama: é o contrato mínimo para `generate` assíncrono; acrescentar `result` e `tasks_cancel`.

### 2. `boot` (7 tools) — leitura, escrita com dry-run e job

- Leituras síncronas: `boot_get_disks`, `boot_get_state` (`tools/boot.go:129-191`).
- Escritas: `boot_attach`, `boot_detach`, `boot_replace`, `boot_scrub`, `boot_set_scrub_interval`, todas com `dry_run` e descrição de risco (`registry.go:1434-1560`).
- Fluxo de `boot_attach` (`tools/boot.go:212-272`): valida argumento, **pré-checagem de segurança** (lê `boot.get_disks`, recusa 3-way mirror e disco já presente), chama o método, espera 500 ms, localiza o job com `findLatestBootJob` (`:81-127`, porque o método documenta resultado `null` e o id não vem na resposta), cria task com TTL de 1 h e devolve envelope. O `findLatestBootJob` é específico do TrueNAS; **não** copiar.
- Lição de contrato: o handler aplica "ler estado → validar → agir → devolver acompanhamento", e a prévia (`dry_run`) reaproveita as mesmas leituras (`tools/boot.go:280-347`).

### 3. Gates de confirmação (`confirm=true`) — segredos e operações destrutivas

- `system_advanced_sed_global_password` devolve uma senha em texto claro; sem `confirm=true` o handler **recusa** com uma mensagem que explica o risco e aponta a alternativa que não expõe o valor (`..._is_set`) (`tools/system_advanced.go:383-395`).
- `system_shutdown` tem `reason` obrigatório, `delay` opcional e `confirm` (`registry.go:454-475`; handler `registry.go:25106`, dry-run `:25220`); `config_reset` espelha o mesmo gate (`tools/config.go:61`).
- Padrão geral: o schema declara `confirm`, a descrição começa com `*** EXTREMELY DANGEROUS ***`, e o **servidor recusa** se faltar. Para o mcpollama, que **não tem tools administrativas** (CLAUDE.md), o equivalente é um gate para qualquer tool que **leia arquivos fora de uma allowlist** ou devolva conteúdo de arquivo sensível — decisão do owner.

## Observações para o mcpollama

- Manter a declaração das tools num único `registry` com `Definition` + `Handler` é simples e deu certo; **dividir a declaração por família** (um arquivo por família, como `src/tools/`) evita o arquivo de 26 mil linhas, que obrigou a editar sempre o mesmo arquivo (e gerou os problemas de CRLF, bug-003/006).
- Ordenar `ListTools` por nome (hoje é ordem de `map`).
- Contrato de saída com **contadores do Ollama** (`prompt_eval_count`, `eval_count`) no mesmo envelope JSON, como o TrueNAS faz com `task_status`/`poll_interval`.
