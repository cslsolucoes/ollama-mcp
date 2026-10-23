# MCPVMWare — tools e contratos

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. As centenas de tools geradas **não** são catalogadas aqui (estão em `MCPVMWare/TOOLS.md`).

## 1. Como uma tool é declarada

Três elementos passados a `r.register(name, description, schema, Tool{Handler: fn})` (`MCPVMWare/src/tools/registry.go:372-380`):

- **nome** (string);
- **descrição** (string longa, em inglês);
- **`inputSchema`** como `map[string]interface{}` literal (não há struct nem gerador de schema); o handler é `func(ctx, client, args map[string]interface{}) (string, error)` (`registry.go:204`).

Exemplo mínimo e idiomático (`MCPVMWare/src/tools/vm.go:21-50`): fragmentos de schema reutilizados (`vmArg`, `confirmArg`) e `registerDestructive` com o `tier`. Tool sem argumentos: `{"type":"object","properties":{}}` (`system.go:17-25`). O `mcp.Tool` serializado tem só `name`, `description`, `inputSchema` (`mcp/types.go:45-49`): **sem `title`, `annotations` nem `outputSchema`**.

Argumentos chegam como `map[string]interface{}` e cada handler converte à mão (`args["vm"].(string)`, `toInt32/toInt64` para números, que o JSON entrega como `float64`, `vm.go:201-229`). Há dezenas de helpers duplicados com nomes por arquivo (`requiredStringArg` em `generated_library_core.go:571`, `requiredArgString` em `generated_library_sessions.go:427`, `guestRequiredArg` em `generated_guest_ops.go:482`, `cloudAWSSDDCRequireString` etc.), consequência de geração paralela por agentes. Para o mcpollama: **um** helper de argumentos num único arquivo.

## 2. Convenções observadas

| Aspecto | Convenção | Evidência |
| --- | --- | --- |
| Nome | `vmware_<domínio>_<ação>` em `snake_case`; produto no prefixo (`vmware_workstation_*`, `vmware_cloudaws_*`) | `gen/main.go:523-556` (regra do gerador); amostras `vm.go`, `workstation_vm.go:148` |
| Domínio | mapa **explícito** tipo→prefixo, sem inferência, para não haver ambiguidade | `gen/main.go:49-93` |
| Colisão | gerador avisa colisão de nome proposto e exclui nomes já escritos à mão | `gen/main.go:255-285` |
| Descrição | 1ª frase = o que faz; depois pré-requisitos, efeitos, o que a tool **não** faz ("does not power it off for you") | `vm.go:88-90`, `workstation_vm.go:148` |
| Pré-requisito de modo | sufixo constante anexado à descrição (`requiresVCSA`, `requiresVmrestLocal`) | `appliance.go:27`, `workstation_vm.go:59` |
| `inputSchema` | `type:object`, `properties`, `required` (lista de `interface{}`), `default` informativo em alguns casos (`path` com `"default":"*"`) | `system.go:26-31` |
| Descrição de propriedade | prosa longa com exemplos ("e.g. ..."), mediana 154 caracteres (medida no plano B1) | `b1-desenho...plan.md:57-62` |
| Destrutiva | propriedade `confirm` (boolean) **e** `required` contendo `confirm` | `vm.go:26-29, 41-50` |
| Espera ("wait") | toda tool de espera tem `timeout_seconds` com default limitado (5 min) | `bug-006`, `LIC-VMW-004` |
| Lista vazia | sempre `[]`, nunca `null`; contrato `count` + array | `system.go:46-100` |

## 3. Formato de saída

A tool devolve **uma string**; quase sempre `json.MarshalIndent(..., "", "  ")` de um `map[string]interface{}` (`system.go:91-100`). Exemplo (`vmware_list_vms`, `system.go:121-146`): `{"path": "...", "count": N, "vms": [...]}`. Consequências:

- Formato legível, mas o **indent de 2 espaços custa tokens**; o mcpollama deve avaliar JSON compacto para respostas lidas pelo modelo.
- O contrato é `count` + lista nomeada; o cliente não precisa de tratar `null` graças à rede central (`normalizeNilSlices`, `system.go:69-89`).
- Sem `structuredContent`. O texto é o único canal.

`normalizeNilSlices` desce recursivamente por `map[string]interface{}` e `[]interface{}` e troca slices `nil` por vazios do mesmo tipo; **não** entra em structs do govmomi (`system.go:57-89`). Só normaliza listas; campo ausente continua `null`, que é o significado correto.

## 4. Formato de erro

Dois canais, usados com disciplina (`MCPVMWare/src/mcpvmware-mcp/main.go:387-420`):

| Situação | Canal | Medido |
| --- | --- | --- |
| Falha dentro da tool (argumento ausente, backend recusou, gate fechado, confirm ausente, tool desconhecida, panic recuperado) | `result.isError:true`, texto `Error: <mensagem>` | sim (§3 do doc 10 e testes do gate) |
| `params` inválido em `tools/call` | erro JSON-RPC `-32602` | `main.go:389-395` (não exercitado) |
| JSON inválido | `-32700` (sem `id`) | sim |
| Método desconhecido com `id` | `-32601` (inclui `ping`) | sim |

Observação: "tool desconhecida" vai como `isError` e não como `-32602`; a spec permite as duas leituras, e o cliente (modelo) vê o texto de qualquer modo.

As mensagens de erro de tool são escritas para **o modelo agir**: dizem o que fazer ("restart it with that flag/env var", "call vmware_list_profiles to see what is available", "(is the target a vCenter Server Appliance? an ESXi standalone host has no VAMI)", `vmware/client.go:124, 149`; `discovery.go:261`). Padrão a copiar.

## 5. `compact` vs `full` no `tools/list`

Mecanismo: doc 10 §6. Medição **própria** desta onda, em cópia: `NewRegistry(nil client)` por modo, `json.Marshal(mcp.ToolsListResult)` (é o que vai no fio, JSON compacto), `full` vs `compact`, sem `--profile`:

| Modo | Tools | `full` (bytes) | `compact` (bytes) | Redução |
| --- | ---: | ---: | ---: | ---: |
| `vcenter` | 924 | 902.083 | 323.811 | **64,1%** |
| `vmware` (ESXi) | 395 | 411.483 | 138.264 | **66,4%** |
| `workstation` | 30 | 28.775 | 9.536 | **66,9%** |
| `cloud-aws` | 97 | 98.429 | 36.107 | **63,3%** |
| `vcenter` + `--profile=host` | 164 | 152.618 | 55.100 (compact) | 63,9% sobre o `full` do perfil; 93,9% sobre o `full` completo |

Conta de custo por tool, a partir dos números acima: `full` ≈ 976 B/tool (902.083 / 924), `compact` ≈ 350 B/tool. Com a razão do próprio projeto (~4 B por token, `b1-desenho...plan.md:21-26`: 910.314 B ↔ ~227.600 tokens; é heurística, não tokenizador), um servidor de ~8 tools custa ~2 mil tokens em `full` e ~0,7 mil em `compact`. O plano B1 mediu o mesmo efeito contra um ESXi real: **−63,1%** com `compact` e **−93,8%** com `compact` + `--profile=host` (229.938 → 14.240 tokens, `b1-desenho...plan.md:246-255`). A diferença entre a medição do plano (909 tools, 910.314 B) e a minha (924 tools, 902.083 B) vem de o plano medir o manifesto do `dumptools` (JSON indentado) e eu medir a resposta do fio; e de o registro ter crescido (**(inferência)**; não reconciliado byte a byte).

**Composição do payload** (medida pelo plano B1, modo vCenter): `inputSchema` 71,1%; dentro dele, `description` das propriedades 66,4% do schema (47,2% do payload) e estrutura 33,5%; `description` da tool 20,8% do payload; nome 3,6%. Ou seja, encurtar só a descrição da tool poupa 10,6%; o ganho vem das descrições de **propriedade**.

**Risco central e o que foi medido:** que o modelo escolha pior a tool com descrição curta (RB-1). O proxy usado foi "quantas tools têm a mesma 1ª frase": 911 tools, 911 frases distintas, zero colisões. **Não é avaliação com modelo**; ficou como pendência P-B1 (`b1-desenho...plan.md:267-269, 288`). Para o mcpollama, o descritor das tools é pequeno; o mais valioso é manter `describe_tool` ou, mais simples, manter as descrições já curtas e evitar o modo.

## 6. Famílias de tools (contagem)

Medido em cópia (`--all-url`, 1.047 tools; família = 2º segmento do nome; 52 famílias). Maiores: `host` 162, `appliance` 135 (VAMI), `cloudaws` 95, `vm` 72, `library` 68, `namespace` 43, `crypto` 30, `workstation` 28, `tags` 27, `guest` 26, `cluster` 23, `fcd` 22, `dvs` 20, `datastore` 20, `fcdhost` 18. Por produto (`MCPVMWare/README.md:15-21`): vSphere 922, VMC 95, Workstation 28, meta 2; por modo: 529 vcenter-only e 393 vsphere-general (`README.md:30-32`). Destrutivas (com `confirm`): 607 (medido; `README.md:32`).

Origem por técnica (`README.md:17-19`, `cerebrum.md`): (a) à mão (29 tools da primeira leva + discovery + workstation + cloudaws); (b) `generated_*` escritas por subagentes a partir do relatório do `gen` (métodos `object.*` e `vapi/*` do govmomi); (c) ondas "SOAP cru" para métodos `vim25` sem wrapper `object.*` (iSCSI, FT, Guest Ops, KMIP, DVS, vSAN, FCD...), invisíveis ao `gen` (`cerebrum.md:74-88`); (d) VAMI legado a partir de uma collection Postman.

## 7. O que isto significa para o contrato das tools do mcpollama

1. Declarar tools com structs e um construtor de schema, não `map` literal à mão (menos erro de tipo e de `required`).
2. Prosa de descrição **curta e direta**; incluir pré-requisito de modo/flag no final, como o sufixo constante.
3. Exigir `timeout`/limites no schema das tools que podem demorar (`generate`, `summarize_file`).
4. Devolver os contadores do Ollama no texto e, se possível, também em `structuredContent` (MCPVMWare não tem como referência; o `ollama-mcp-go` põe em campos de topo, ver brief dele).
5. Nunca devolver `null` onde o contrato diz lista.
6. Respostas em JSON compacto ou texto simples, não indentado.
