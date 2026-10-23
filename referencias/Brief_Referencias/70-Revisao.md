# Revisão adversarial do brief (onda B6)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B6 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` (revisão em Opus) |

Revisão adversarial dos 37 documentos do brief, escritos por agentes Haiku e Sonnet e conferidos por amostragem nas ondas B1 a B5. A postura é desconfiar: um falso-OK é pior que um FAIL. Convenções em [00-CONVENCOES.md](00-CONVENCOES.md).

## Veredito

**COM RESSALVAS — confiança alta (~85%) nas citações e nos números; média nas classificações de cobertura do `61` antes desta revisão.**

- As citações `caminho:linha` de código são, na grande maioria, exatas: 62 de 64 confirmadas (as 2 restantes, imprecisas, com o conteúdo a poucas linhas); nenhuma aponta para código inexistente. No total da amostra, 71 de 84 confirmadas; os erros estão na camada derivada (`60`/`61`).
- Todos os números recontados batem (dimensionamento, 171 requisitos, 104/53/14, MUST por fase, lições, testes, tools, bytes). Os números da validação empírica batem com a saída bruta da sonda.
- Gate de segredos limpo (`known_hits=0`) e nenhum IP ou host de laboratório de terceiros no brief.
- **O ponto fraco era o otimismo da coluna "Cobertura" do `61`:** 5 requisitos marcados "Resolvido" em que a referência tem o defeito oposto ou só parte (3 deles MUST), e 2 marcados "Parcial" sem referência nenhuma. Corrigidos nesta onda: a cobertura passou de 53/45/73 para **48/47/76** (resolvido/parcial/GAP).
- Nenhum achado crítico. Ficam 6 pendências para o owner ou para a B7 (`PB6-n`).

## Método

1. **Amostragem estratificada de citações** (seção "Amostra"): para cada citação, o arquivo foi aberto na linha citada (`sed -n`/`grep -n` sobre `referencias/`, só leitura) e o trecho comparado com a afirmação. Classes: **CONFIRMADA**, **IMPRECISA** (o conteúdo existe, mas a linha ou o atributo está errado), **NÃO SUSTENTADA**. Prioridade para afirmações que geram requisito MUST e para cada "Resolvido" do `61`.
2. **Recontagem por comando**, com a saída lida: `wc -l`, `find | wc -l`, `grep -c`, um script Python que relê as tabelas do `60` e do `61` (prioridades, cobertura por área, fases, MUST por fase, coerência de prioridade entre os dois) e a suíte do `ollama-mcp-go` rodada numa **cópia** no scratchpad.
3. **Coerência** contra os fatos decididos e medidos (P-1, P-B2, `num_ctx` 131072, defeito do prompt grande) e contra a **especificação MCP oficial** (WebFetch em `modelcontextprotocol.io`, versão 2025-11-25, coleta 2026-10-23; conteúdo web tratado como dado).
4. **Segredos:** `scan-segredos-brief.ps1` antes e depois das correções, mais busca própria de IPv4 e de nomes de host de laboratório.
5. **Links:** todo link markdown relativo resolvido contra o disco.
6. **Validação empírica:** cada número do `validacao-empirica.md` comparado com `scratchpad/b4probe/saida-b4.txt` e `saida-b4-stream.txt` (sem chamar o servidor).

Arquivos **não abertos**, conforme a lista de exclusão: `**/.mcp.json`, `**/.claude/settings.local.json`, `**/*.env`, `**/*.pem`, `**/*.key`, `DADOS_CONEXAO_TESTE.md`, `config_teste.json`, coleções Postman e os reports `*configuracao-servidores-teste*`/`*servidor-teste-config*`. Nenhum `git` foi rodado dentro de `referencias/`; nada fora do `Brief_Referencias/` foi escrito.

## Amostra de citações (84)

Caminhos relativos a `referencias/`. "Onde" é o documento do brief que afirma.

### `ollama-mcp-go` (18)

| # | Onde | Citação | Afirmação | Veredito |
| ---: | --- | --- | --- | --- |
| 1 | `60` PROTO-003, OMG-10 | `ollama-mcp-go/main.go:644-648` | JSON inválido só é logado (`continue`), sem `-32700` | CONFIRMADA |
| 2 | OMG-10, `60` PROTO-004 | `ollama-mcp-go/main.go:651-654` | `id` nulo tratado como notificação | CONFIRMADA |
| 3 | `60` PROTO-009 | `ollama-mcp-go/main.go:282` | tool desconhecida → `errorResult` → `-32602` (`:607-613`) | CONFIRMADA |
| 4 | OMG-30, `60` CFG-011 | `ollama-mcp-go/main.go:298-309` | trace com default `/tmp/ollama-mcp-trace.log`; `off` desliga | CONFIRMADA |
| 5 | OMG-30, `60` OBS-004 | `ollama-mcp-go/main.go:329-338` | arquivo de trace com `0o644`, grava o prompt | CONFIRMADA |
| 6 | OMG-20, `60` TOK-002 | `ollama-mcp-go/main.go:86-87` | `prompt_eval_count`/`eval_count` em campos de topo | CONFIRMADA |
| 7 | OMG-20, `60` TOOL-012 | `ollama-mcp-go/main.go:580-589` | `embed` devolve dimensões + 16 valores | CONFIRMADA |
| 8 | `60` CFG-003 | `ollama-mcp-go/main.go:454-456` | "sem default" de `model` | **IMPRECISA**: `454-456` só faz a conversão; a exigência está em `:458-460` (`'model' and 'prompt' are required`). Corrigido para `454-460` |
| 9 | `60` OLL-010 | `ollama-mcp-go/main.go:405-407` | `thinking` vai para o trace, não para o cliente | CONFIRMADA |
| 10 | `60` SEC-005 | `ollama-mcp-go/main.go:390`, `433`, `566` | `io.ReadAll` sem limite no corpo de erro | CONFIRMADA |
| 11 | `60` PROTO-005 | `ollama-mcp-go/protocol.go:97-109`; `protocol_test.go:64-72` | `negotiate` + teste de tabela (`1999-01-01` → `2025-11-25`) | CONFIRMADA |
| 12 | `60` PROTO-021 | `ollama-mcp-go/protocol.go:87-88` | "Nothing leaves the machine", só em `server/discover` | CONFIRMADA |
| 13 | `60` OBS-008 | `ollama-mcp-go/main.go:622` | `w.Flush()` sem checar erro | CONFIRMADA |
| 14 | OMG-40, OMG-50 | `ollama-mcp-go/main.go:59`; `protocol.go:34` | `version = "0.2.0"` usada no `serverInfo` | CONFIRMADA |
| 15 | OMG-30, OMG-50 | `ollama-mcp-go/main_test.go:73-76` | `TestMain` força `OLLAMA_MCP_TRACE=off` | CONFIRMADA |
| 16 | `60` TEST-001 | `ollama-mcp-go/main_test.go:515-534` | o teste copia o loop em vez de chamar o `main` | CONFIRMADA |
| 17 | `60` TOOL-005 | `ollama-mcp-go/main.go:48-56` | `openWorldHint` falso justificado por "Ollama is local" | CONFIRMADA |
| 18 | OMG-README | execução | 26 testes PASS, 75,2% de cobertura | CONFIRMADA (re-executado em cópia: 26 `--- PASS`, 0 FAIL, `coverage: 75.2%`) |

### `what-is-mcp` (8)

| # | Onde | Citação | Afirmação | Veredito |
| ---: | --- | --- | --- | --- |
| 19 | WIM-10, `60` PROTO-016 | `04-what-is-mcp/mcp-server/main.go:32`, `35`, `37` | `fmt.Println`/`Printf` no stdout | CONFIRMADA |
| 20 | WIM-20, `60` TOOL-007 | `04-what-is-mcp/mcp-server/main.go:42-45` | tipo errado → `NewToolResultError` | CONFIRMADA |
| 21 | WIM-10 | `04-what-is-mcp/mcp-server/go.mod:1`, `:3`, `:5`, `:7` | `hello-mcp-server`, `go 1.23.4`, `mcp-go v0.8.2`, `uuid` indireta | CONFIRMADA |
| 22 | WIM-10 | `04-what-is-mcp/mcp-server/go.sum:5-6` | somas do `mcp-go` | CONFIRMADA |
| 23 | WIM-10 | `04-what-is-mcp/.devcontainer/compose.yml:8` | contêiner com Go 1.23.1 | CONFIRMADA |
| 24 | WIM-README | `04-what-is-mcp/README.md:231`, `:235` | `[!Claude Desktop](...)` | CONFIRMADA |
| 25 | WIM-README | `04-what-is-mcp/go.mod:1`; `mcp-server/compose.yml:1` | módulo `01-json-output`; compose só com `# in progress` | CONFIRMADA |
| 26 | WIM-20 | `04-what-is-mcp/README.md:53-80` | exemplo de `tools` (`add_numbers`, `subtract_numbers`) | CONFIRMADA |

### `mcpvmware` (18)

| # | Onde | Citação | Afirmação | Veredito |
| ---: | --- | --- | --- | --- |
| 27 | `60` PROTO-002 | `MCPVMWare/src/mcpvmware-mcp/main.go:310-316` | `bufio.NewScanner` sem `Buffer()` (`:313`) | CONFIRMADA |
| 28 | `60` PROTO-003 | `MCPVMWare/src/mcpvmware-mcp/main.go:330-333`; `src/mcp/types.go:17` | `-32700` com `id` nil e `omitempty` | CONFIRMADA |
| 29 | `60` PROTO-005 | `MCPVMWare/src/mcpvmware-mcp/main.go:371-380` | `protocolVersion` fixo `2024-11-05` (`:373`) | CONFIRMADA |
| 30 | `60` PROTO-014 | `MCPVMWare/src/tools/registry.go:409-423` | `sort.Slice` por nome (`:421`) | CONFIRMADA |
| 31 | `60` PROTO-009 | `MCPVMWare/src/tools/registry.go:439-442`; `main.go:397-406` | tool desconhecida vira `isError:true` com `Error: unknown tool` | CONFIRMADA |
| 32 | `60` PROTO-019 | `MCPVMWare/src/tools/registry.go:443-447` | `recover` por chamada | CONFIRMADA |
| 33 | `60` CFG-011, SEC-014 | `MCPVMWare/src/tools/registry.go:59-65` | audit desligado com caminho vazio; "nunca grava em disco sem ser pedido" | CONFIRMADA |
| 34 | `60` SEC-015 | `MCPVMWare/src/tools/destructive.go:38-47`, `222-243` | `Args` no registro de audit; arquivo `0o600` | CONFIRMADA |
| 35 | `60` CFG-002 | `MCPVMWare/src/mcpvmware-mcp/main.go:143-159`, `150-152` | `MCPVMWARE_DEBUG` só aceita `1` | CONFIRMADA |
| 36 | `60` SEC-007, OBS-006 | `MCPVMWare/src/mcpvmware-mcp/main.go:176-181` | `WARNING` para `--insecure` e `--allow-destructive` | CONFIRMADA |
| 37 | `60` OBS-002 | `MCPVMWare/src/mcpvmware-mcp/main.go:325-327` | `--debug` loga a linha inteira do stdin | CONFIRMADA |
| 38 | `60` SEC-002, SEC-004 | `MCPVMWare/src/workstation/client.go:139`, `151`, `153`; `src/vmware/client.go:86`, `92` | `u.String()` no erro (vaza); `u.Redacted()` no outro cliente | CONFIRMADA |
| 39 | `60` PROTO-011 | `MCPVMWare/src/mcpvmware-mcp/main.go:217-226`, `258-268`; `MCP-GAPS-VMWARE.md:61-63` | conexão no boot com `log.Fatalf`; "Connection closed" | CONFIRMADA (o `MCP-GAPS` tem IPs de laboratório; o brief cita só a linha, sem copiá-los) |
| 40 | `60` BUILD-003 | `MCPVMWare/src/mcpvmware-mcp/main.go:63-65`, `128-131` | `var Version = "dev"` e `--version` | CONFIRMADA |
| 41 | `60` OLL-002 | `MCPVMWare/src/mcpvmware-mcp/main.go:184` | `context.Background()` sem prazo | CONFIRMADA |
| 42 | `60` CFG-006 | `MCPVMWare/src/mcpvmware-mcp/main.go:271-277` | `--tools-verbosity` validado no arranque | CONFIRMADA (ressalva: a validação vem **depois** da conexão das linhas `217-268`; o exemplo sem rede é o TNS) |
| 43 | `60` TOOL-010 | `MCPVMWare/src/tools/system.go:91-100` | `json.MarshalIndent` | CONFIRMADA |
| 44 | `60` TEST-008 | `MCPVMWare/src/tools/mode_test.go:1165-1193` | igualdade de conjunto (`assertExactToolSet`) | CONFIRMADA |

### `truenas-mcp` (20)

| # | Onde | Citação | Afirmação | Veredito |
| ---: | --- | --- | --- | --- |
| 45 | `60` SEC-006 | `MCPNas/truenas-mcp/truenas/tls.go:18-34`, `:33` | `NewCertPool()` em `RootCAs` substitui o pool do SO | CONFIRMADA |
| 46 | `60` SEC-008 | `MCPNas/truenas-mcp/truenas/client.go:145-147` | dica `--tls-ca` no erro x509 | CONFIRMADA |
| 47 | `60` SEC-009 | `MCPNas/truenas-mcp/truenas/client.go:273-276` | `ws://` recusado | CONFIRMADA |
| 48 | `60` SEC-003 | `MCPNas/truenas-mcp/truenas/client.go:665-670` | `scrubSecrets` troca o valor exato por `[REDACTED]` | CONFIRMADA |
| 49 | `60` OBS-003 | `MCPNas/truenas-mcp/truenas/client.go:581-592` | `maxDebugLogLineBytes = 4000` | CONFIRMADA |
| 50 | `60` OBS-001 | `MCPNas/truenas-mcp/truenas/client.go:234-248` | log mínimo sem `--debug` | CONFIRMADA |
| 51 | `61` PB5-3 | `MCPNas/truenas-mcp/truenas/client.go:815-821` | 120 s é o timeout por chamada do cliente WebSocket | CONFIRMADA |
| 52 | `60` INTEG-005 | `MCPNas/truenas-mcp/tools/core.go:510-516` | 100 s "well under truenas.Client's own 120-second per-call timeout" | CONFIRMADA |
| 53 | `60` TOOL-013 | `MCPNas/truenas-mcp/tasks/types.go:42` | `Result` com `json:"-"` | CONFIRMADA |
| 54 | `60` TOOL-013 | `MCPNas/truenas-mcp/tasks/manager.go:123` | `Cancel` | CONFIRMADA |
| 55 | `60` PROTO-012 | `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:123`, `286-301` | `stdoutMutex` na escrita | CONFIRMADA |
| 56 | `60` PROTO-003 | `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:146-153` | `-32700` com `id` nil | CONFIRMADA |
| 57 | `60` PROTO-014 | `MCPNas/truenas-mcp/tools/registry.go:20444-20450` | `ListTools` itera `map` sem ordenar | CONFIRMADA |
| 58 | `60` PROTO-011 | `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:89-97` | autenticação lazy, comentário do bug | CONFIRMADA |
| 59 | `60` CFG-001, CFG-002 | `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:41-69`, `63-69` | flag vence, env de fallback; booleano `""/0/false/no/off` | CONFIRMADA |
| 60 | `60` TEST-005, TEST-006 | `MCPNas/truenas-mcp/truenas/tls_test.go:43`; `client_test.go:772-802` | testes de TLS e de chave ecoada | CONFIRMADA |
| 61 | `60` TOOL-003 | `MCPNas/truenas-mcp/.wolf/cerebrum.md:36` | limite de 60 do Cursor (bug-017) | CONFIRMADA |
| 62 | `60` INTEG-002 | `MCPNas/truenas-mcp/.vscode/MCP-VSCODE.md:25-28`, `36-45` | `inputs`; `servers` + `"type":"stdio"` | CONFIRMADA |
| 63 | `60` SEC-002 (via TNS-30) | `MCPNas/truenas-mcp/truenas/client.go:305` | a chave vai em `auth.login_with_api_key`, no corpo | CONFIRMADA (é a evidência que derruba o "Resolvido" do `61`, ver REV-M-01) |
| 64 | TNS-README item 7 | `MCPNas/truenas-mcp/tools/core.go:510-516` | "abaixo do timeout de 120 s do cliente" | **IMPRECISA**: o texto deixava ler "cliente MCP"; o 120 s é do cliente WebSocket. Corrigido (REV-M-07) |

### `60` e `61`: requisito → fonte e cobertura (12)

| # | Onde | Afirmação | Veredito |
| ---: | --- | --- | --- |
| 65 | `61` SEC-002 | "Resolvido" por TNS | **NÃO SUSTENTADA** (REV-M-01) |
| 66 | `61` TEST-005 | "Resolvido" por TNS | **NÃO SUSTENTADA** (REV-M-02) |
| 67 | `61` TOOL-011 | "Resolvido" por OMG | **NÃO SUSTENTADA** (REV-M-03) |
| 68 | `61` TOOL-009 | "Resolvido" por VMW | **NÃO SUSTENTADA** (REV-M-04) |
| 69 | `61` TEST-012 | "Resolvido" por VMW e TNS | **NÃO SUSTENTADA** (REV-M-05) |
| 70 | `61` BUILD-007, BUILD-009; Comparativo | TNS como solução de `.gitattributes`/`.gitignore` | **NÃO SUSTENTADA** (REV-M-06); o status segue "Resolvido" pelo VMW |
| 71 | `60` PROTO-009 | `-32602` "é o que a especificação indica (inferência)" | CONFIRMADA pela especificação (REV-M-08) |
| 72 | `60` CFG-005 | `load_ms` 31.963 em `VAL §5` | **IMPRECISA**: o número está no changelog do plano, não no `VAL` (REV-m-03) |
| 73 | `60` SEC-001; `61` §5 | `OMG-30:33` "ainda sugere ou de arquivo" | **NÃO SUSTENTADA** (desatualizada: corrigido no B5; REV-m-04) |
| 74 | `60` SEC-016 | `STATUS.md:101`: `.mcp.json` fora do `.gitignore` | **IMPRECISA**: a mesma linha diz que foi corrigido; hoje em `MCPVMWare/.gitignore:6` (REV-m-05) |
| 75 | `61` PROTO-011, OBS-003, TOOL-003, PERF-012, TOK-005 | "Resolvido" (TNS/VMW) | CONFIRMADA (5 conferidos contra o código acima: #58, #49, #61, #58, `discovery_test.go:139`, `:150`) |
| 76 | `61` INTEG-009, INTEG-011 | "Parcial" com referência "nenhuma" | **NÃO SUSTENTADA** pela própria definição de Parcial (REV-m-01) |

### Web (5)

| # | Ficha | Afirmação | Veredito |
| ---: | --- | --- | --- |
| 77 | W1 | tools `reason_task`/`embed_text`/`filter_docs`; 4 env (3 obrigatórias, `OLLAMA_HOST` default `localhost:11434`); stdio; `go build`; GPL v3; `slog` | CONFIRMADA (README `master`, WebFetch 2026-10-23) |
| 78 | W3 | timeout de 60 s; Node.js; tools `pull`/`create` | CONFIRMADA (README do GitHub, WebFetch 2026-10-23) |
| 79 | W7 | `use_curl`, "url must be a string", `curl -s` sem limite nem timeout | CONFIRMADA (`main.go` do 05, WebFetch 2026-10-23) |
| 80 | W6 | anotações por tool, idempotência em `list_models` e `embed` | CONFIRMADA (código local `ollama-mcp-go/main.go:176`, `186`, `201`, `219`) |
| 81 | W1 ideia 1 | "(inferência: o servidor atual tem um único modelo residente)" | **NÃO SUSTENTADA** depois da B4 (o `nomic-embed-text` existe). Ressalva "desatualizado" acrescentada (REV-m-06) |

### Servidor Ollama da CSL (3)

| # | Onde | Afirmação | Veredito |
| ---: | --- | --- | --- |
| 82 | `VAL` §1 a §7 | todos os números (21.035–21.057 ms; 46/8/8 ms; 536/438/437 ms; 44 tokens; 41/37/37 tokens; 96,5/96,8/97,0 tok/s; 678/866/878 tok/s; `num_ctx` 131072; 262144; 30.532.122.624; 128/8 experts; 25.562.028.438 B; `modified_at` 15:36:22; 3.230/9.936/20.896 ms; 768 dim; 13 tokens; 45.026/90.049/90.026/45.019 ms; `models: []`; 36.449/36.675 ms) | CONFIRMADA contra `saida-b4.txt` e `saida-b4-stream.txt`; "24k"/"40k" estão corretamente rotulados como não medidos |
| 83 | `SRV` | `CONEXAO-IA.md:14-21`, `:103-113`, `:133-136`, `:160-172` | CONFIRMADA (lidas só essas linhas) |
| 84 | `SRV` | `ia-local.ps1:3`, `:44-46`, `:48-51`, `:55` | CONFIRMADA |

**Totais:** 84 citações ou afirmações conferidas: **71 CONFIRMADAS**, **4 IMPRECISAS** (#8, #64, #72, #74) e **9 NÃO SUSTENTADAS** (#65 a #70, #73, #76, #81; a #70 cobre 3 lugares). Por estrato: `ollama-mcp-go` 18, `what-is-mcp` 8, `mcpvmware` 18, `truenas-mcp` 20, `60`/`61` 12, Web 5, servidor 3.

Leitura: as citações de **código** (#1 a #64) estão 62/64 corretas; os erros se concentram nas **classificações e sínteses** do B5 (`61` e trechos do `60`), que são a camada derivada.

## Recontagens

| Item | Afirmado | Recontado (comando) | Resultado |
| --- | --- | --- | --- |
| `ollama-mcp-go` | 4 arquivos, 1.426 linhas | `wc -l *.go` → 662 + 562 + 109 + 93 = 1.426 | bate |
| `04-what-is-mcp` | 1 arquivo, 47 linhas | `wc -l main.go` = 47 | bate |
| `truenas-mcp` | 116 arquivos, 109.126 linhas | `find -name '*.go' \| wc -l` = 116; `cat \| wc -l` = 109.126 | bate |
| `MCPVMWare/src` | 176 arquivos, 85.397 linhas; 134 `generated_*`; 23 núcleo | 176; 85.397; 134; 23 | bate |
| `MCPVMWare` linhas por tipo | 8.708 + 47.309 + 29.380; 86 arquivos de teste; 6.146 à mão em `tools/` | idem | bate |
| `MCPVMWare/TOOLS.md` | 1.027.395 bytes, 14.738 linhas, 1.047 `####` | `wc -c`, `wc -l`, `grep -c '^#### '` | bate |
| `MCPVMWare` `Test*` | 485 | `grep -c '^func Test'` somado = 485 | bate |
| `truenas-mcp` tools / testes | 730 / 64 | `grep -c '^\s*r\.tools\["'` = 730; `grep '^func Test'` = 64 | bate |
| `ollama-mcp-go` testes | 26 PASS + `TestMain`, 75,2% | 27 declarações; `go test -count=1 -v` em cópia: 26 PASS, 0 FAIL; `-cover` 75,2% | bate |
| Requisitos do `60` | 171; 104 MUST, 53 SHOULD, 14 COULD; por área como na tabela | script: 171; 104/53/14; as 12 áreas batem | bate |
| `60` × `61` | mesmos IDs e prioridades | 0 só num lado; 0 divergência de prioridade | bate |
| MUST por fase (`61` §3) | 43 / 29 / 37 / 23 / 14 / 6; fase 0: 3 | script: idem; listas idênticas | bate |
| Matriz fase × área (`61` §3) | totais 60 / 40 / 64 / 40 / 21 / 14 | idem | bate |
| Cobertura (`61` §1) | 53 / 45 / 73 | 53 / 45 / 73 antes; **48 / 47 / 76 depois das correções** | contagem batia; a classificação estava otimista |
| Lições | OMG 10, VMW 36, TNS 30 | IDs distintos: 10, 36, 30 (não há `LIC-WIM`, `LIC-WEB`, `LIC-SRV`) | bate |
| Documentos | 37 (+ este = 38) | `find -name '*.md'` = 37 antes deste | bate |
| Formato | LF, sem BOM | 0 arquivos com CRLF e 0 com BOM (antes e depois das correções) | bate |

## Achados

### Críticos (REV-C)

Nenhum. Não foi achado segredo, número errado em dimensionamento ou medição, nem citação de código inventada.

### Médios (REV-M)

- **REV-M-01 — `SEC-002` (MUST) marcado "Resolvido" pelo TNS, que faz o oposto.** Evidência: o TrueNAS autentica com `auth.login_with_api_key` e a chave como parâmetro da mensagem, ou seja, **no corpo** (`MCPNas/truenas-mcp/truenas/client.go:305`; o próprio `truenas-mcp/30-Configuracao_e_Seguranca.md` §Autenticação diz isso). O requisito proíbe token no corpo. Nenhuma referência usa Bearer em cabeçalho. **Corrigido:** `61` SEC-002 → GAP.
- **REV-M-02 — `TEST-005` (MUST) "Resolvido" pelo TNS.** O cenário 3 do requisito exige `SystemCertPool()` + PEM; o TNS testa a CA com `NewCertPool()` (`truenas/tls.go:29`), o mesmo ajuste que deixou `SEC-006` como Parcial. **Corrigido:** Parcial.
- **REV-M-03 — `TOOL-011` (MUST) "Resolvido" pelo OMG.** O requisito pede `model?` opcional, `temperature?` e `num_predict?`; o OMG exige `model` (`ollama-mcp-go/main.go:458-460`, `487-490`) e não repassa `options` (`:92-112`). Contradizia o próprio `61`, que marca `CFG-003` como GAP pelo mesmo motivo. **Corrigido:** Parcial.
- **REV-M-04 — `TOOL-009` (SHOULD) "Resolvido" pelo VMW, cujas descrições são longas.** `mcpvmware/20-Tools_e_Contratos.md:16` ("string longa") e o anti-padrão nº 21 do `60` apontam o VMW como exemplo do defeito. **Corrigido:** Parcial (o VMW cobre "estática" e "pré-requisito no fim").
- **REV-M-05 — `TEST-012` (MUST) "Resolvido" pelo VMW e pelo TNS.** O CI do VMW faz `go clean -testcache`, `build`, `vet` e `gofmt -l`, mas **sem `-cover`** (`MCPVMWare/.github/workflows/ci.yml:47-71`, `grep cover` = 0); no TNS o `-cover` de `tools/` quebra por BOM. **Corrigido:** Parcial.
- **REV-M-06 — TNS creditado com `.gitattributes` e `.gitignore`.** `61` BUILD-007/BUILD-009 e o `Comparativo` (§2, "copiar do TNS: `.gitattributes` contra CRLF e BOM") apontavam o TNS como solução; a cópia do TNS **não tem** `.gitattributes` nem `.gitignore` (`ls -a`) e tem BOM em `tools/kmip.go`. O VMW tem os dois (`MCPVMWare/.gitattributes`: `*.go text eol=lf`; `.gitignore:6`: `.mcp.json`). **Corrigido:** referências passam a ser o VMW; o status continua "Resolvido".
- **REV-M-07 — "Timeout de 120 s do cliente" ambíguo** em `truenas-mcp/README.md:89` (item 7). O 120 s é do cliente WebSocket do TrueNAS (`truenas/client.go:815-821`; `tools/core.go:510-516` diz "truenas.Client's own 120-second per-call timeout"), não do cliente MCP, que ninguém mediu. **Corrigido** no texto. O `60` (INTEG-005) e o `61` (PB5-3) já estavam certos.
- **REV-M-08 — `PROTO-009`: a "inferência" é fato da especificação.** A especificação MCP 2025-11-25 (`server/tools`, "Error Handling") lista "Unknown tools" como *Protocol Error*, com exemplo `{"code": -32602, "message": "Unknown tool: invalid_tool_name"}`; o `schema` (`CallToolResult.isError`) diz que erros ao *encontrar* a tool vão como erro MCP. O OMG está conforme; VMW e TNS (`isError`) não. **Corrigido:** fonte citada no `60` e no `61`. A prioridade (SHOULD × MUST) fica com o owner (**PB6-1**).

### Menores (REV-m)

- **REV-m-01 — `INTEG-009` e `INTEG-011` "Parcial" com referência "nenhuma",** contra a definição do próprio `61`. **Corrigido:** GAP.
- **REV-m-02 — 7 GAPs com referência parcial na coluna** (`PROTO-012`, `PROTO-022`, `CFG-010`, `TOOL-012`, `TOOL-014`, `INTEG-005`, `PERF-006`). A classificação é defensável (a referência é anti-padrão, conceito ou só uma peça), mas não segue a letra da definição. **Não alterado**: decisão de critério (**PB6-3**).
- **REV-m-03 — `CFG-005` atribuía o `load_ms` 31.963 ao `VAL §5`.** O número veio da recarga das 18:28 (PB4-4), registrada no changelog do plano, e a saída bruta não foi arquivada. **Corrigido:** a fonte passa a ser o PBR; o `validacao-empirica.md` ganhou a linha da execução 4, com a ressalva.
- **REV-m-04 — "Contradição" desatualizada:** `60` SEC-001 e `61` §5 ainda diziam que `ollama-mcp-go/30:33` sugeria "ou de arquivo"; foi corrigido no B5 (a linha hoje descarta o `--api-key-file`). **Corrigido.**
- **REV-m-05 — `60` SEC-016 citava `.mcp.json` fora do `.gitignore` sem dizer que foi corrigido** na mesma linha do `STATUS.md:101`. **Corrigido.**
- **REV-m-06 — W1, ideia 1:** inferência "um único modelo residente" desmentida pela B4. **Corrigido** com ressalva "desatualizado", no mesmo padrão das ressalvas de 32k.
- **REV-m-07 — `CFG-003`:** linha `454-456` → `454-460`. **Corrigido.**
- **REV-m-08 — `openWorldHint`:** a especificação define default `true` e `false` como "domínio fechado" (exemplo: ferramenta de memória). O `TOOL-005` propunha `false` sem citar isso. **Corrigido:** a fonte cita o `schema`. O valor final continua sendo a P-9 do owner (**PB6-2**).
- **REV-m-09 — `Comparativo`, configuração do TNS:** "6 flags com env", mas o `--version` não tem env. **Corrigido:** "6 flags, 5 delas com env".
- **REV-m-10 — `CFG-006` cita o VMW como exemplo de validação sem rede,** mas o VMW valida o `--tools-verbosity` (`main.go:271-277`) só **depois** de conectar ao backend (`:217-268`). O status "Resolvido" se sustenta pelo TNS. Registrado na amostra (#42); sem mudança.
- **REV-m-11 — Gate de segredos não persiste a triagem dos genéricos:** cada rodada mostra `generic_untriaged=52`, todos já triados à mão nas ondas anteriores. O gate duro (`known_hits=0`) funciona; o contador mole perde valor como sinal (**PB6-4**).
- **REV-m-12 — Convenção prevê `LIC-WIM`, `LIC-WEB` e `LIC-SRV`, que não existem;** o `60` lista WIM entre as fontes de lição. Sem impacto; sem mudança.

### Pontos de coerência conferidos sem achado

- **P-1 = não:** nenhuma menção a `--api-key-file` como opção em todo o brief (`grep`); `truenas-mcp/30:24`, `truenas-mcp/80:114`, `mcpvmware/30:38` e `ollama-mcp-go/30:33` alinhados.
- **P-B2:** `60` BUILD-006 e o fato 2 apontam `src/cmd/dumptools` (`MCPVMWare/src/cmd/dumptools/main.go:23-47`), sem Python. Coerente.
- **32k × 131072:** as 6 linhas antigas com "32k" têm a ressalva "desatualizado"; as demais menções a 32k são sobre a divergência ou proíbem o literal. Coerente.
- **Defeito do prompt grande:** `60` (fatos 4, PERF-004/005, OLL-005), `61` (PB5-1, PB5-10), `SRV` e `VAL §7` dizem o mesmo (~42 mil caracteres, ~45 s, 2 reproduções, limiar desconhecido). Coerente.

## Segredos e dados sensíveis

- `scan-segredos-brief.ps1` antes das correções: `sources=15 loaded=25 allowlisted=1`, `files=37 known_hits=0 generic_hits=56 generic_untriaged=52`. Depois das correções e com este documento: ver o changelog do `README.md` (1.3.0).
- Busca própria de IPv4: só `10.100.2.2` e `10.100.0.0/16` (servidor da CSL, permitidos), `0.0.0.0`, `127.0.0.1` e `192.168.1.10` (exemplo `OLLAMA_HOST` do `ollama-mcp-go/README.md:72`, não é da CSL nem de laboratório). Os IPs de ESXi que existem nas fontes (`MCPVMWare/.wolf/STATUS.md`, `MCPVMWare/MCP-GAPS-VMWARE.md:62`) e o IP da estação que aparece na saída bruta da sonda **não** entraram no brief.
- Nomes de host de laboratório (ESXi, vCenter, TrueNAS): nenhum; só palavras genéricas ("ESXi", `vcenter-url`).

## Links

39 links markdown relativos no brief antes deste documento; 0 quebrados (script que resolve cada alvo contra o disco).

## Correções aplicadas (antes → depois)

| Arquivo:linha | Antes | Depois |
| --- | --- | --- |
| `60-Requisitos_mcpollama.md:79` (PROTO-009) | "...que é o que a especificação MCP indica (inferência)" | citação da especificação 2025-11-25 (`server/tools` e `schema`), OMG conforme, VMW/TNS fora |
| `60-Requisitos_mcpollama.md:102` (CFG-003) | `ollama-mcp-go/main.go:454-456` | `454-460`, com a exigência em `:458-460` |
| `60-Requisitos_mcpollama.md:104` (CFG-005) | "VAL §5 (`load_ms` 36.449 e 31.963 ms)" | "VAL §5 (`load_ms` 36.449); PBR (changelog 18:29: `load_ms` 31.963, saída não arquivada)" |
| `60-Requisitos_mcpollama.md:119` (SEC-001) | "linha 33 do doc ainda sugere 'ou de arquivo': contradição" | "linha 33, alinhada à P-1 no B5" |
| `60-Requisitos_mcpollama.md:134` (SEC-016) | "`.mcp.json` fora do `.gitignore`" | "estava fora...; corrigido lá mesmo, hoje em `MCPVMWare/.gitignore:6`" |
| `60-Requisitos_mcpollama.md:169` (TOOL-005) | sem a especificação | defaults de `ToolAnnotations` da especificação |
| `60-Requisitos_mcpollama.md` cabeçalho e changelog | 1.0.0 | 1.1.0 |
| `61-Matriz_Cobertura.md` §1 | SEC 7/5/4, TOOL 6/5/4, TEST 7/3/8, INTEG 3/5/4; total 53/45/73 | SEC 6/5/5, TOOL 4/7/4, TEST 5/5/8, INTEG 3/3/6; total **48/47/76** |
| `61` SEC-002 | TNS, Resolvido | nenhuma (TNS manda a chave no corpo), GAP |
| `61` TEST-005 | TNS, Resolvido | TNS parcial (`NewCertPool`), Parcial |
| `61` TEST-012 | VMW, TNS, Resolvido | VMW parcial (sem `-cover`); TNS anti-padrão; Parcial |
| `61` TOOL-009 | VMW, Resolvido | VMW parcial (descrições longas), Parcial |
| `61` TOOL-011 | OMG, Resolvido | OMG parcial (`model` obrigatório), Parcial |
| `61` INTEG-009, INTEG-011 | nenhuma, Parcial | nenhuma, GAP |
| `61` BUILD-007 | TNS, VMW | VMW; TNS como anti-padrão |
| `61` BUILD-009 | VMW, TNS | VMW; TNS sem `.gitignore` |
| `61` PROTO-009 | "Conferir na especificação MCP oficial antes de fixar" | conferido; prioridade para o owner (PB6-1) |
| `61` §4 (GAPs de baixo risco) | `SEC` sem o Bearer | `SEC` com "Bearer só no cabeçalho" |
| `61` §5 (P-1) | "contradição ... ainda sugere" | "corrigida no B5" |
| `61` cabeçalho e changelog | 1.0.0 | 1.1.0 |
| `Comparativo-Referencias-mcpollama.md` §1 (Configuração, TNS) | "6 flags com env" | "6 flags, 5 delas com env (`--version` não tem)" |
| `Comparativo` §2 (TNS, Copiar) | "`.gitattributes` contra CRLF e BOM (`BUILD-007`)" | "a lição de CRLF e BOM ...; o `.gitattributes` a copiar é o do `MCPVMWare`" |
| `Comparativo` §2 (MCPVMWare, Copiar) | sem `.gitattributes` | "`.gitattributes` com `*.go text eol=lf` (`BUILD-007`)" |
| `Comparativo` cabeçalho e changelog | 1.0.0 | 1.1.0 |
| `truenas-mcp/README.md:89` | "abaixo do timeout de 120 s do cliente" | "abaixo do timeout de 120 s por chamada do cliente WebSocket do TrueNAS, `truenas/client.go:815-821`; não é o timeout do cliente MCP" |
| `truenas-mcp/README.md` cabeçalho | 1.0.0 | 1.0.1 |
| `Web/W1-dxeo-ollama-mcp.md:51` | inferência "único modelo residente" | + ressalva "desatualizado" (B4: `nomic-embed-text` instalado; `CFG-004`) |
| `Web/W1` cabeçalho | 1.0.0 | 1.0.1 |
| `Servidor-Ollama-CSL/dados/validacao-empirica.md:22` | 3 execuções | + execução 4 (recarga 18:28, `load_ms` 31.963, saída não arquivada) |
| `validacao-empirica.md` cabeçalho | 1.0.0 | 1.0.1 |
| `README.md` | B6 pendente; 1.2.0 | B6 feito; changelog 1.3.0 |

## Pendências para o owner ou para a B7 (`PB6-n`)

1. **PB6-1 — `PROTO-009` sobe para MUST?** A especificação 2025-11-25 trata tool desconhecida como erro de protocolo (exemplo `-32602`). Subir mudaria a contagem para 105 MUST / 52 SHOULD. Decisão do owner.
2. **PB6-2 — `openWorldHint` (P-9).** Com o default `true` da especificação, as tools do mcpollama precisam declarar o valor explicitamente; `false` ("domínio fechado": um Ollama privado e arquivos dentro de `--allow-root`) é a proposta do `TOOL-005`. Decisão do owner.
3. **PB6-3 — Critério para "GAP com referência parcial"** (7 linhas, REV-m-02): manter como GAP ou reclassificar como Parcial. Muda a contagem do `61` §1.
4. **PB6-4 — Persistir a triagem dos genéricos no `scan-segredos-brief.ps1`** (por SHA-256 de `arquivo:linha:conteúdo`), para o `generic_untriaged` voltar a ser sinal. O script fica em `.workspace/` e não foi tocado nesta onda.
5. **PB6-5 — B7:** `CLAUDE.md`, `README.md` da raiz e o plano de implementação ainda dizem `qwen3-ia` com ctx 32k (já sinalizado no `60`, fato 5, e em `BUILD-010`).
6. **PB6-6 — Arquivar a saída bruta das sondas futuras** (a execução 4 da B4 ficou só no changelog do plano). Regra de processo para as próximas validações ao vivo.

## Changelog

- **1.0.0 (2026-10-23):** B6. 84 citações e afirmações conferidas (71 confirmadas, 4 imprecisas, 9 não sustentadas), recontagens, coerência com a especificação MCP 2025-11-25, segredos e links. 8 achados médios, 12 menores, nenhum crítico; correções aplicadas no `60`, `61`, `Comparativo`, `truenas-mcp/README.md`, `Web/W1` e `validacao-empirica.md`; 6 pendências `PB6-n`.
