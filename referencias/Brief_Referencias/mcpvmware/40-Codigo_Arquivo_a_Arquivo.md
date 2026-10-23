# MCPVMWare — código, arquivo a arquivo

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`; prefixo `MCPVMWare/src/` omitido nas tabelas (`tools/vm.go` = `MCPVMWare/src/tools/vm.go`). Linhas físicas medidas com `wc -l`. São os **23 arquivos de núcleo** (não gerados, não teste), que somam 8.708 linhas, mais `gen/` (artefatos). Ordem: do mais importante para o mcpollama ao menos.

| # | Arquivo | Linhas | Grupo |
| ---: | --- | ---: | --- |
| 1 | `mcpvmware-mcp/main.go` | 439 | entrypoint |
| 2 | `mcp/types.go` | 74 | protocolo |
| 3 | `tools/registry.go` | 461 | registry |
| 4 | `tools/discovery.go` | 292 | descoberta |
| 5 | `tools/destructive.go` | 243 | segurança |
| 6 | `tools/system.go` | 144 | núcleo de saída |
| 7 | `tools/finder.go` | 51 | auxiliar (govmomi) |
| 8 | `cmd/dumptools/main.go` | 47 | catálogo |
| 9 | `gen/main.go` | 611 | classificador |
| 10 | `vmware/client.go` | 196 | cliente |
| 11 | `workstation/client.go` | 163 | cliente |
| 12 | `cloudaws/client.go` | 264 | cliente |
| 13-17 | `tools/vm.go` (491), `host.go` (227), `inventory.go` (252), `datastore.go` (116), `appliance.go` (109) | 1.195 | tools à mão (vSphere) |
| 18-19 | `tools/workstation_vm.go` (474), `workstation_network.go` (599) | 1.073 | tools à mão (Workstation) |
| 20-23 | `tools/cloudaws_orgs.go` (967), `cloudaws_sddcs.go` (861), `cloudaws_networking_core.go` (700), `cloudaws_networking_edge.go` (927) | 3.455 | tools à mão (VMC) |

Conferência: 439+74+461+292+243+144+51+47+611+196+163+264 = 2.985; 491+227+252+116+109 = 1.195; 474+599 = 1.073; 967+861+700+927 = 3.455; total 2.985+1.195+1.073+3.455 = **8.708**.

## 1. `mcpvmware-mcp/main.go` (439 linhas)

**Papel:** entrypoint stdio. **Tipos:** `StdioHandler` (`:303-308`). **Funções:** `resolveConnectionMode` (`:70-123`), `main` (`:125-300`), `NewStdioHandler` (`:310-316`), `Run` (`:318-351`), `handleRequest` (`:353-369`), `handleInitialize` (`:371-380`), `handleToolsList` (`:382-385`), `handleToolsCall` (`:387-416`), `createErrorResponse` (`:418-420`), `sendResponse` (`:422-435`), `sendError` (`:437-439`). **Globais:** 16 flags (`:44-61`), `Version` (`:65`). **Observações:** nenhuma função é testada (não há `main_test.go`; `ls src/mcpvmware-mcp` mostra só `main.go`). Armadilhas em [10](10-Arquitetura.md) §8.

## 2. `mcp/types.go` (74 linhas)

**Papel:** tipos JSON-RPC 2.0 e MCP escritos à mão. **Tipos:** `Request` (`:8-13`, `ID interface{}` com `omitempty`, `Params map[string]interface{}`), `Response` (`:15-20`), `Error` (`:22-26`), `InitializeResult` (`:30-34`), `ServerInfo` (`:36-39`), `Capabilities` (`:41-43`, só `tools`), `Tool` (`:45-49`), `ToolsListResult` (`:51-53`), `ToolCallParams` (`:55-58`), `ToolCallResult` (`:60-63`), `ContentBlock` (`:65-68`, só `type` e `text`), `ToolRegistry` (interface, `:70-74`). **Faltam:** `title`, `annotations`, `structuredContent`, `isError` em `omitempty` (está), blocos de imagem/recurso, `id:null` explícito no erro de parse. Cabe em 74 linhas porque só cobre `initialize`, `tools/list`, `tools/call`.

## 3. `tools/registry.go` (461 linhas)

**Papel:** registro, filtros e despacho. **Tipos:** `Registry` (`:20-48`), `RegistryOptions` (`:53-94`), `ConnectionMode` + 6 constantes (`:104-146`), `toolMode` + 5 constantes (`:154-166`), `Tool` (`:202-207`). **Funções:** `connectionModeAllows` (`:170-192`), `NewRegistry` (`:218-238`), `registerTools` (`:243-358`, 89 chamadas `withClass`), `withClass` (`:363`), `register` (`:372`), `registerWorkstation` (`:385`), `registerCloudAWS` (`:398`), `ListTools` (`:409`), `CallTool` (`:438`, com `recover`). **Padrão a copiar:** campos novos em `RegistryOptions` em vez de parâmetros posicionais, para não quebrar nenhum teste existente (`cerebrum.md:415-426`). **A evitar no mcpollama:** 3 campos de handler por tipo de cliente.

## 4. `tools/discovery.go` (292 linhas)

**Papel:** `compact`, `profile`, tools meta. **Tipos:** `ToolsVerbosity` (`:47`) com `VerbosityFull`/`VerbosityCompact` (`:49-55`). **Vars:** `metaToolNames` (`:60-63`), `fimDeFrase` (`:68`), `abreviaturas` (`:72-76`). **Funções:** `firstSentence` (`:86-114`), `stripPropertyDescriptions` (`:123-150`), `stripNodeDescription` (`:160-189`), `compactDefinition` (`:193-201`), `toolProfile` (`:206-212`), `profileAllows` (`:216-221`), `registerDiscoveryTools` (`:226-250`), `handleDescribeTool` (`:254-269`), `handleListProfiles` (`:273-292`). **Valor:** o comentário de cabeçalho (`:1-31`) é a medição que justifica o desenho; copiar o hábito de pôr a medição junto do código.

## 5. `tools/destructive.go` (243 linhas)

**Papel:** gate, `confirm`, audit. **Tipos:** `tier` (`:23`), `destructiveHandler` (`:32`), `auditEntry` (`:38-47`), `workstationDestructiveHandler` (`:60`), `cloudAWSDestructiveHandler` (`:115`). **Funções:** `registerDestructive` (`:52`), `registerDestructiveWorkstation` (`:65`), `wrapDestructiveWorkstation` (`:72-110`), `registerDestructiveCloudAWS` (`:126`), `wrapDestructiveCloudAWS` (`:133-171`), `wrapDestructive` (`:177-215`), `writeAudit` (`:222-243`). **Dívida visível:** os três `wrap*` são **cópias** com a mesma lógica (~40 linhas cada) por causa dos 3 tipos de handler; com um só tipo de cliente isso some. Audit grava `args` crus (ver [30](30-Configuracao_e_Seguranca.md) §5).

## 6. `tools/system.go` (144 linhas)

**Papel:** primeiras tools (`vmware_about`, `vmware_list_vms`) e os helpers de resposta. **Funções:** `registerSystemTools` (`:17`), `nonNilSlice[T]` (`:54`), `normalizeNilSlices` (`:69`), `marshalJSON` (`:91`), `handleAbout` (`:102`), `handleListVMs` (`:121`). `marshalJSON` é o ponto único de serialização: **958 sites de resposta** passam por ele (`buglog.json`, bug-015, citando medição do projeto) e a rede central cobre as tools futuras.

## 7. `tools/finder.go` (51 linhas)

**Papel:** `dcScopedPath(folder, path)` (`:32`) e `emptyOnNotFound(err)` (`:45`). Específico do govmomi (caminhos de inventário e `find.NotFoundError`). **Sem valor direto** para o mcpollama; a lição é o padrão "zero resultados não é erro" e o caminho absoluto com curinga (bug-002).

## 8. `cmd/dumptools/main.go` (47 linhas)

**Papel:** imprime `name`, `description`, `inputSchema` de cada modo em JSON, sem backend. **Tipos:** `dumpTool` (`:17-21`). **Funções:** `dump(mode)` (`:23-33`; `NewRegistry(ctx, nil, {ConnectionMode, AllowDestructive:true})`), `main` (`:35-47`; 4 modos: `vcenter`, `vmware`, `workstation`, `cloudaws`). Só funciona porque `NewRegistry` não desreferencia o cliente (handlers são valores de função, `:3-6`). `AllowDestructive:true` é inofensivo: tools destrutivas listam sempre, o gate é na chamada (`:26`).

## 9. `gen/main.go` (611 linhas)

**Papel:** classificador (não gera código nem `TOOLS.md`; ver [50](50-Testes_Build_Release.md) §3). **Tipos:** `Method` (`:31-44`). **Vars:** `domainPrefix` (`:49-93`), `vcenterOnlyFiles` (`:140-150`), `existingHandWrittenTools` (`:160-190`), `tier1Patterns` / `tier2Patterns` / `readOnlyPatterns` (`:194-196`), `tier1VerbAnywhere` (`:205`), `skipMethods` (`:211-215`). **Funções:** `main` (`:217-315`), `countPkg` (`:317`), `findGoPackageDirs` (`:331-360`), `parsePackageDir` (`:365-421`), `firstParamIsContext` (`:423`), `receiverTypeName` (`:436`), `renderParams` (`:450`), `renderReturns` (`:471`), `exprString` (`:482`), `firstDocLine` (`:505`), `toSnake` (`:518`), `classify` (`:523-556`), `writeJSON` (`:558`), `writeMarkdown` (`:572-611`). **Artefatos ao lado:** `gen/classification.json` (6.853 linhas, 527 métodos), `gen/CLASSIFICATION_REPORT.md` (631 linhas), mais `src/gen.exe` (binário; ver LIC-VMW-017).

## 10. `vmware/client.go` (196 linhas)

**Papel:** cliente govmomi com *keepalive*, `Finder`, REST e SSO lazy. **Tipos:** `Config` (`:24-31`), `Client` (`:41-49`). **Funções:** `NewClient` (`:64`), `newClient` (`:71-104`, idle parametrizável para teste), `REST` (`:113-128`), `SSO` (`:139-156`), `Close` (`:160-179`), `sdkURL` (`:184-196`, host nu → `https://host/sdk`). **Padrão:** `Config` com URL aceitando "host nu" ou URL completa; mensagens de erro que nomeiam a causa provável.

## 11. `workstation/client.go` (163 linhas)

**Papel:** HTTP + Basic Auth para o `vmrest`. **Tipos:** `Config` (`:31-39`), `Client` (`:44-49`), `errorModel` (`:55-58`, chaves **maiúsculas** `Code`/`Message`, diferente da spec, medido ao vivo). **Funções:** `NewClient` (`:63-90`, valida URL absoluta), `Do` (`:92-109`), `DoRawBody` (`:111-116`, uma rota com corpo cru), `doRaw` (`:118-163`). **Padrão a copiar:** erro com método + URL + status + mensagem do servidor, decodificando o corpo de erro estruturado quando existe (`:150-153`). **A corrigir no mcpollama:** a URL no erro pode conter segredo (ver [30](30-Configuracao_e_Seguranca.md) §6); `io.ReadAll` sem limite de tamanho (`:141-145`).

## 12. `cloudaws/client.go` (264 linhas)

**Papel:** HTTP com troca de token CSP. **Tipos:** `Config` (`:42-56`, `AuthURL`/`BaseURL` só para teste), `Client` (`:62-71`), `tokenResponse` (`:80-83`), `errorModel` (`:88-92`). **Funções:** `NewClient` (`:98-129`), `ensureToken` (`:133-177`, margem de 60 s `:39`), `Do` (`:186-202`, **uma** retentativa em `401`, nunca em laço), `doOnce` (`:206-264`). **Padrão a copiar:** cache de token com mutex e margem de expiração; retentativa única e limitada.

## 13. Tools à mão, vSphere (5 arquivos)

| Arquivo | Conteúdo | Pontos |
| --- | --- | --- |
| `tools/vm.go` (491) | ciclo de vida da VM, snapshots, info | `registerVMTools` `:21`, `resolveVM` `:176`, `waitForTask` `:192`, `toInt32/64` `:201, 216`, handlers `:231-470`. Modelo de tool destrutiva |
| `tools/host.go` (227) | manutenção, reconectar, IPs, info | `registerHostTools` `:18`, `resolveHost` `:92` |
| `tools/inventory.go` (252) | `list_hosts/datastores/networks/resource_pools/clusters/datacenters` | `registerInventoryTools` `:27`, `pathArg` `:90` |
| `tools/datastore.go` (116) | `vmware_datastore_upload_file` | **lê arquivo local sem allowlist** (`:71-109`), ver [30](30-Configuracao_e_Seguranca.md) §7 |
| `tools/appliance.go` (109) | VAMI: versão, uptime, health | `registerApplianceTools` `:25`, `applianceGet` `:60`, lista de subsistemas `:18-20` |

## 14. Tools à mão, Workstation (2 arquivos)

`tools/workstation_vm.go` (474): constantes de schema `wsIDArg`/`wsVMPasswordArg` (`:63-76`), `wsAppendVMPassword` (`:78-87`, com o defeito de `?` no path, ver [30](30-Configuracao_e_Seguranca.md) §6), `registerWorkstationVMTools` (`:89`), `registerWorkstationPowerTools` (`:225`), handlers `:263-470`. `tools/workstation_network.go` (599): helpers `wsGet`/`wsMutate`/`wsRequiredInt` (`:32, 45, 61`), famílias de pastas compartilhadas, NIC e rede do host (`:75, 218, 391`).

## 15. Tools à mão, VMC (4 arquivos, 3.455 linhas)

`cloudaws_orgs.go` (967): helpers `cloudGet/cloudMutate/cloudQueryString/cloudOptional*` (`:87-190`), `registerCloudAWSOrgsTools` (`:191`). `cloudaws_sddcs.go` (861): `registerCloudAWSSDDCsTools` (`:72`), helpers com prefixo `cloudAWSSDDC*`. `cloudaws_networking_core.go` (700) e `cloudaws_networking_edge.go` (927): helpers `cloudAWSNetCore*` / `cloudAWSEdge*`. **O prefixo único por arquivo** evitou colisões de símbolo entre 4 grupos paralelos (`cerebrum.md:464-471`): vale para qualquer geração paralela.

## 16. Os `generated_*` (67 arquivos, 47.309 linhas; só mecanismo)

Cada arquivo tem cabeçalho de comentário de ~40-60 linhas (ex.: `tools/generated_alarm.go:18-60`) que registra: de qual *managed object* vem, a lista de métodos confirmada contra o `methods.go` do govmomi, a classe (`modeVCenterOnly` ou `modeVSphereGeneral`) com a **evidência** (nulidade em `simulator/esx/service_content.go` vs `vpx/`), e as exclusões. **Não há cabeçalho `DO NOT EDIT`** (`grep -l "DO NOT EDIT\|Code generated" src/tools/*.go` vazio): são código normal, escrito por subagentes e mantido à mão. Cada um tem `registerXTools(r)` ligado em `registerTools()` (`registry.go:243-358`) e um `_test.go` irmão.
