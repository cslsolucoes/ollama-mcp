# MCPVMWare — visão geral

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## O que é

Servidor **MCP em Go, stdio, JSON-RPC 2.0**, do mesmo autor do mcpollama (módulo `github.com/cslsoftwares/mcpvmware`, `MCPVMWare/src/go.mod:1`). Expõe o VMware por **1.047 tools** em três produtos (vSphere/vCenter/ESXi, VMware Cloud on AWS, Workstation Pro), escolhidos por **modo de conexão** (uma flag por execução). Tipos MCP escritos à mão, sem SDK MCP; a única dependência externa é o `govmomi` v0.55.1 (`MCPVMWare/src/go.mod:3-8`).

É o **molde CSL** do mcpollama por causa do desenho, não do domínio: `mcp/types.go` à mão, `flags → env`, `--tools-verbosity compact`, `--profile`, gate destrutivo com `confirm`, audit log, `TOOLS.md` gerado e `Build-Release.ps1`. O mcpollama terá ~8 tools, não 1.047; por isso o valor está nos **mecanismos** e nas **armadilhas pagas**, não no catálogo.

**Descoberta que corrige a premissa do plano:** `src/gen` **não** gera `generated_*` nem `TOOLS.md`. É um classificador de uma só vez (Fase 0). Detalhe em [50-Testes_Build_Release.md](50-Testes_Build_Release.md) §3. Resumo: `src/gen` lê o `govmomi` e escreve um relatório (`classification.json` + `CLASSIFICATION_REPORT.md`); os `generated_*` foram escritos por subagentes de LLM a partir desse relatório; o `TOOLS.md` sai do **registro de tools em execução**, via `src/cmd/dumptools` + `scripts/gen_tools_md.py`.

## Fontes

| Fonte | Papel | Como usada |
| --- | --- | --- |
| `MCPVMWare/src/` (23 arquivos de núcleo + `gen/`) | código | lida inteira |
| `MCPVMWare/src/tools/generated_*` (134 arquivos) | tools geradas por agentes | só mecanismo (cabeçalhos, 2 amostras, helpers) |
| `MCPVMWare/scripts/gen_tools_md.py`, `MCPVMWare/tools/Build-Release.ps1`, `MCPVMWare/.github/workflows/ci.yml` | geração de doc, release, CI | lidos inteiros |
| `MCPVMWare/TOOLS.md` | catálogo gerado | só o início + contagem + comparação de nomes com o registro |
| `MCPVMWare/.wolf/cerebrum.md`, `buglog.json`, `STATUS.md` | lições | lidos (cerebrum e buglog inteiros; STATUS por busca) |
| `MCPVMWare/.workspace/plans/*` (6), `.workspace/reports/*` (amostra: fase 2 + busca nos demais) | decisões e resultados | leitura parcial dirigida |
| `MCPVMWare/CLAUDE.md`, `MCP-GAPS-VMWARE.md`, `README.md` | contexto | lidos inteiros |

Não lidos, por regra do plano: `.mcp.json`, `settings.local.json`, `*.env`, `*.pem`, `*.key`, `.workspace/DADOS_CONEXAO_TESTE.md`, `.workspace/config_teste.json`, `*.postman_collection.json`, relatório `*configuracao-servidores-teste*`. Fora do âmbito: `referencia/`, `Develop/`, `dist/`, `.remember/`, `.wolf/hooks/`, `.wolf/token-ledger*`.

## Dimensionamento (medido em 2026-10-23)

| Item | Valor | Como medido |
| --- | ---: | --- |
| Arquivos `.go` em `src/` | 176 | `find src -name '*.go' \| wc -l` |
| Linhas físicas em `src/` | 85.397 | `find src -name '*.go' -print0 \| xargs -0 cat \| wc -l` |
| Núcleo (não gerado, não teste) | 23 arquivos, 8.708 linhas | `find ... ! -name 'generated_*' ! -name '*_test.go' \| xargs wc -l` |
| `generated_*.go` (sem teste) | 67 arquivos, 47.309 linhas | `cat generated_*.go` (excluindo `_test`) |
| `generated_*_test.go` | 67 arquivos, 22.865 linhas | `cat generated_*_test.go \| wc -l` |
| Testes escritos à mão em `tools/` | 6.146 linhas | `cat` dos `*_test.go` sem prefixo `generated_` |
| Todos os testes (86 arquivos) | 29.380 linhas | `find src -name '*_test.go'` |
| Conferência | 8.708 + 47.309 + 29.380 = 85.397 | soma fecha com o total |
| Tools registradas (modo `--all-url`) | **1.047** | `NewRegistry(nil client, ConnectionModeEverything).ListTools()` em cópia fora de `referencias/` |
| Tools destrutivas (com `confirm` no schema) | 607 | mesma execução, contando `properties.confirm` (bate com `README.md:32`) |
| Famílias (`--profile` = 2º segmento do nome) | 52 | mesma execução |
| Propriedades de schema somadas | 2.974 | mesma execução |
| `TOOLS.md` | 1.027.395 bytes, 14.738 linhas, 1.047 seções `####` | `wc -c`, `wc -l`, `grep -c '^#### '` |
| Nomes do registro vs nomes do `TOOLS.md` | **idênticos** (diff vazio, 1.047/1.047) | `sort` + `diff` das duas listas |
| `gen/classification.json` | 527 métodos (tier2 331, sem tier 139, tier1 57; `object` 309, `vapi` 218) | leitura do JSON |
| Dependências externas | 1 (`govmomi`) + 2 indiretas | `go.mod:5-8` |

> A medição do registro foi feita **em cópia** de `src/` na pasta temporária da sessão; `referencias/` não foi alterada. O binário compilado ali respondeu a `--version` com `mcpvmware-mcp version dev`.

## Índice

| Arquivo | Conteúdo |
| --- | --- |
| [10-Arquitetura.md](10-Arquitetura.md) | entrypoint, loop stdio, dispatch, registry, modos, verbosity, profile, camadas, fluxo de `tools/call`, comportamentos de protocolo medidos |
| [20-Tools_e_Contratos.md](20-Tools_e_Contratos.md) | como a tool é declarada, convenções, saída e erro, `compact` vs `full` medido, famílias |
| [30-Configuracao_e_Seguranca.md](30-Configuracao_e_Seguranca.md) | flags→env completas, TLS, auth, gate destrutivo, audit log, segredos |
| [40-Codigo_Arquivo_a_Arquivo.md](40-Codigo_Arquivo_a_Arquivo.md) | ficha dos 23 arquivos de núcleo + `gen/` |
| [50-Testes_Build_Release.md](50-Testes_Build_Release.md) | testes, build, release, versão, mecanismo de geração (`gen`, `generated_*`, `TOOLS.md`) |
| [80-Licoes_Aprendidas.md](80-Licoes_Aprendidas.md) | `LIC-VMW-nnn` |

## O que tirar para o mcpollama

1. **Registry com `Definition` + handler e `register()` único** (`MCPVMWare/src/tools/registry.go:202-207, 372-380`). Uma tool = nome + descrição + `inputSchema` + função. O mcpollama copia o desenho, com um só tipo de cliente (o do Ollama), sem os três campos `Handler/WSHandler/CloudHandler`.
2. **`ListTools()` ordena por nome e a compactação acontece só na saída** (`registry.go:409-423`, `discovery.go:193-201`). O registro guarda sempre a definição completa; assim `describe_tool` serve o texto integral. Vale para o mcpollama mesmo com poucas tools.
3. **`--tools-verbosity compact` com a regra "poda prosa, nunca estrutura"** (`discovery.go:22-28`, `116-201`). Medido: −63% a −67% do `tools/list` (doc 20 §5). Com ~8 tools o ganho absoluto é pequeno (~2 mil tokens → ~0,7 mil), mas o desenho é barato e o teste de corpus real vale a pena.
4. **Gate destrutivo em 3 camadas** (`destructive.go:177-215`): gate de servidor, `confirm:true` booleano estrito, audit. O mcpollama não tem tools destrutivas, mas as tools que **leem arquivos** são a superfície sensível; o equivalente é `--allow-root` + auditoria (REQ futuro, SEC/FILE).
5. **Audit log nunca grava `args` crus** (anti-lição). O de MCPVMWare grava os argumentos em texto puro, inclusive um campo `password` (medido, `destructive.go:38-47, 222-243`). No mcpollama o equivalente são prompts e conteúdo de arquivo: auditar **caminho + tamanho + hash**, não o texto.
6. **Erro de tool = `result.isError:true`; só protocolo = erro JSON-RPC** (`main.go:397-407, 418-420`). Convenção já adotada no `ollama-mcp-go`; confirmada aqui.
7. **Buffer do `bufio.Scanner` é armadilha fatal** (medido: linha > 64 KB derruba o processo, exit 1). O mcpollama recebe prompts grandes; precisa de `scanner.Buffer(...)` ou `bufio.Reader` + limite explícito. Ver `LIC-VMW-001`.
8. **Responder `ping`** (medido: o servidor devolve `-32601`). O mcpollama deve responder `{}`. Também negociar `protocolVersion` em vez de fixar `2024-11-05` (`main.go:372-374`).
9. **Não fazer I/O de rede antes do loop stdio** (`main.go:258-269`: `log.Fatalf` se o backend não responde). Backend fora = cliente vê "Connection closed" (`MCP-GAPS-VMWARE.md:61-63`). O mcpollama deve subir sempre e falhar **por chamada**, com mensagem útil.
10. **`TOOLS.md` gerado do registro, não do código-fonte** (`cmd/dumptools/main.go:23-33` + `scripts/gen_tools_md.py`). Registry com `client=nil` é seguro porque handlers são valores. O mcpollama copia o mecanismo e **acrescenta o que falta**: um teste que compara o `TOOLS.md` com o registro (MCPVMWare não tem; a igualdade de hoje é disciplina, não garantia).
11. **`marshalJSON` único, com rede de segurança de `null` vs `[]`** (`system.go:54-100`). Ponto único de serialização da resposta evita defeitos de contrato; vale para o texto compacto das respostas do mcpollama.
12. **Smoke por stdio com binário real, sem backend** (reproduzido aqui com `--workstation-url` apontando para porta morta). O mcpollama pode ter um modo equivalente (`--ollama-url` inválido não impede `initialize` e `tools/list`).
13. **Deploy no Windows por renomeação do `.exe` em uso** (`LIC-VMW-016`): o Windows deixa renomear, não sobrescrever. Evita matar sessões MCP vivas.
14. **Versão por `-ldflags "-X main.Version=..."`**, com `git describe --tags --always --dirty` (`tools/Build-Release.ps1:52-66, 78`). O sufixo `-dirty` aparece de fato quando a árvore está suja (`v0.1.0-11-g26490a2-dirty` medido). Para o mcpollama: ignorar no `git describe` o ruído do tooling (`LIC-VMW-017`).
15. **Teste de conjunto exato por modo** (`mode_test.go:1165-1193`): igualdade de conjunto, não `contains`. Para o mcpollama, um teste de "lista exata de tools" protege contra tool administrativa entrar por engano.
