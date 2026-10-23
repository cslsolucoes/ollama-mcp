# Matriz de cobertura: requisito × fase × referência

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.2.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B5 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Cruza cada requisito de [60-Requisitos_mcpollama.md](60-Requisitos_mcpollama.md) com as fases do plano de implementação (`.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`) e com o que as referências já resolvem. O que nenhuma referência resolve aparece como **GAP**, com decisão proposta ou pendência `PB5-n`.

## Como ler

**Fases do plano de implementação:**

| Fase | Nome | Estado (plano) |
| ---: | --- | --- |
| 0 | Ambiente e repositório (concluída) | concluída, sem commit |
| 1 | Esqueleto do protocolo | pendente |
| 2 | Cliente Ollama | pendente |
| 3 | Tools base | pendente |
| 4 | Tools de economia (leitura local) | pendente |
| 5 | Integração com os clientes | pendente |
| 6 | Medição e documentação | pendente |

A Fase 0 já está concluída e entra só como referência (`.gitignore`, `git init`). Em "Fase(s)", `todas` significa as Fases 1 a 6.

**Cobertura** (o que as referências já entregam para o requisito):

- **Resolvido:** pelo menos uma referência implementa o requisito por inteiro; dá para copiar o desenho (a coluna "Referência(s)" diz de qual).
- **Parcial:** as referências cobrem parte; falta ajuste (a fonte do requisito em `60` diz o quê).
- **GAP:** nenhuma referência resolve; o mcpollama precisa inventar (a coluna "GAP / pendência" aponta o `PB5-n`).

Uma referência marcada "(anti-padrão)" ou "(medido)" na coluna é evidência do defeito, não solução.

## 1. Resumo

| Área | Requisitos | Resolvido | Parcial | GAP |
| --- | ---: | ---: | ---: | ---: |
| `PROTO` | 22 | 12 | 7 | 3 |
| `CFG` | 12 | 3 | 4 | 5 |
| `SEC` | 16 | 6 | 5 | 5 |
| `OLL` | 16 | 2 | 8 | 6 |
| `TOOL` | 15 | 4 | 7 | 4 |
| `FILE` | 17 | 0 | 0 | 17 |
| `TOK` | 9 | 1 | 2 | 6 |
| `OBS` | 10 | 2 | 5 | 3 |
| `TEST` | 18 | 5 | 5 | 8 |
| `BUILD` | 12 | 9 | 1 | 2 |
| `INTEG` | 12 | 3 | 3 | 6 |
| `PERF` | 12 | 1 | 0 | 11 |
| **Total** | **171** | **48** | **47** | **76** |

Recontado no B6 (antes: 53 / 45 / 73). Mudaram 8 linhas: `SEC-002` (Resolvido → GAP), `TEST-005`, `TEST-012`, `TOOL-009` e `TOOL-011` (Resolvido → Parcial), `INTEG-009` e `INTEG-011` (Parcial → GAP); detalhe em `70-Revisao.md`.

Leitura: as referências resolvem bem o **protocolo base** e a **higiene de build/teste**, que vêm das duas bases CSL; os GAPs se concentram em `FILE` (leitura segura), `PERF` (fila de GPU, limites do servidor) e `OLL` (resiliência), que são justamente o que o mcpollama tem de novo.

## 2. Requisito × fase × referência

### PROTO

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-PROTO-001` | Framing NDJSON | MUST | 1 | OMG, VMW, TNS | Resolvido |  |
| `MCPO-REQ-PROTO-002` | Linha longa não mata o servidor | MUST | 1 | OMG (parcial: 4 MB) | Parcial | Nenhuma referência sobrevive a linha acima do teto; o teto padrão decide-se na Fase 1 |
| `MCPO-REQ-PROTO-003` | JSON inválido responde -32700 | MUST | 1 | VMW (parcial), TNS (parcial) | Parcial |  |
| `MCPO-REQ-PROTO-004` | Notificação x requisição inválida | SHOULD | 1 | VMW (parcial) | Parcial |  |
| `MCPO-REQ-PROTO-005` | Negociação de protocolVersion | MUST | 1 | OMG | Resolvido |  |
| `MCPO-REQ-PROTO-006` | Ciclo de vida e capabilities | MUST | 1 | OMG, VMW, TNS | Resolvido |  |
| `MCPO-REQ-PROTO-007` | ping responde {} | MUST | 1 | OMG | Resolvido |  |
| `MCPO-REQ-PROTO-008` | Método desconhecido | MUST | 1 | OMG, VMW, TNS | Resolvido |  |
| `MCPO-REQ-PROTO-009` | Tool desconhecida: um só comportamento | MUST | 1, 3 | OMG, VMW, TNS (divergem) | Parcial | Conferido no B6: a especificação MCP 2025-11-25 manda erro de protocolo (exemplo `-32602`); OMG conforme, VMW e TNS (`isError`) fora. **MUST por decisão do owner (PB6-1, 19:21)** |
| `MCPO-REQ-PROTO-010` | Dois canais de erro | MUST | 1, 3 | OMG, VMW, TNS, WIM | Resolvido |  |
| `MCPO-REQ-PROTO-011` | Sem I/O de rede no boot | MUST | 1 | TNS, VMW (anti-padrão documentado) | Resolvido |  |
| `MCPO-REQ-PROTO-012` | Loop concorrente | SHOULD | 1, 3 | TNS (parcial: mutex de stdout) | GAP | PB5-9: nenhuma referência é concorrente |
| `MCPO-REQ-PROTO-013` | notifications/cancelled | SHOULD | 3 | nenhuma | GAP | PB5-9 |
| `MCPO-REQ-PROTO-014` | tools/list determinístico | MUST | 1, 3 | VMW, OMG | Resolvido |  |
| `MCPO-REQ-PROTO-015` | Códigos de saída | MUST | 1 | OMG (EOF) | Parcial |  |
| `MCPO-REQ-PROTO-016` | stdout só JSON-RPC | MUST | 1 | OMG, VMW, TNS | Resolvido |  |
| `MCPO-REQ-PROTO-017` | id preservado | MUST | 1 | OMG, VMW | Resolvido |  |
| `MCPO-REQ-PROTO-018` | params de tools/call | MUST | 1, 3 | OMG (sem teste) | Parcial |  |
| `MCPO-REQ-PROTO-019` | Panic recuperado | MUST | 1, 3 | VMW | Resolvido |  |
| `MCPO-REQ-PROTO-020` | Tipos à mão, sem SDK | MUST | 1 | OMG, VMW, TNS | Resolvido |  |
| `MCPO-REQ-PROTO-021` | instructions verdadeiras | SHOULD | 1 | OMG (texto inadequado) | Parcial |  |
| `MCPO-REQ-PROTO-022` | Modo moderno 2026-07-28 | COULD | 1 | OMG (não verificado) | GAP | PB5-12: spec 2026-07-28 não conferida |

### CFG

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-CFG-001` | Flags com fallback para env | MUST | 1 | TNS, VMW | Resolvido |  |
| `MCPO-REQ-CFG-002` | Regra única de booleano | MUST | 1 | TNS, VMW (inconsistente) | Parcial |  |
| `MCPO-REQ-CFG-003` | Modelo default | MUST | 1, 3 | nenhuma (OMG exige `model`) | GAP | Decisão já tomada no CLAUDE.md |
| `MCPO-REQ-CFG-004` | Modelo de embeddings separado | MUST | 1, 3 | W1 (ideia) | Parcial |  |
| `MCPO-REQ-CFG-005` | Timeout por chamada | MUST | 1, 2, 5 | TNS (espera limitada), W3 | Parcial | PB5-3: default depende de medir o timeout dos clientes |
| `MCPO-REQ-CFG-006` | Validação de flags sem rede | SHOULD | 1, 2 | VMW, TNS | Resolvido |  |
| `MCPO-REQ-CFG-007` | Normalização da URL | SHOULD | 1, 2 | OMG (parcial) | Parcial |  |
| `MCPO-REQ-CFG-008` | Sem default silencioso | SHOULD | 1 | nenhuma | GAP |  |
| `MCPO-REQ-CFG-009` | --allow-root | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-CFG-010` | Tetos configuráveis | MUST | 1, 4 | W1 (`CONTEXT_SIZE`, parcial) | GAP | PB5-1; default ≈ 128.000 caracteres, ~48k tokens (P-6, 19:21) |
| `MCPO-REQ-CFG-011` | Trace e audit opt-in | MUST | 1, 3 | VMW | Resolvido |  |
| `MCPO-REQ-CFG-012` | --help sem segredos | COULD | 1 | nenhuma | GAP |  |

### SEC

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-SEC-001` | Token por env, sem --api-key-file | MUST | 1, 5 | VMW, TNS | Resolvido |  |
| `MCPO-REQ-SEC-002` | Bearer só no cabeçalho | MUST | 2 | nenhuma (TNS manda a chave no **corpo** da mensagem `auth.login_with_api_key`, `MCPNas/truenas-mcp/truenas/client.go:305`; OMG não autentica) | GAP | B6: era "Resolvido" |
| `MCPO-REQ-SEC-003` | Token nunca em log, erro ou audit | MUST | 1, 2 | TNS | Resolvido |  |
| `MCPO-REQ-SEC-004` | URL sanitizada nos erros | MUST | 2 | VMW (parcial) | Parcial |  |
| `MCPO-REQ-SEC-005` | Corpo de erro limitado | MUST | 2 | nenhuma | GAP |  |
| `MCPO-REQ-SEC-006` | TLS: pool do SO mais a CA | MUST | 2 | TNS (ajustar `MCPNas/truenas-mcp/truenas/tls.go:33`) | Parcial | PB5-7: metade internet não validada |
| `MCPO-REQ-SEC-007` | --insecure explícito | MUST | 1, 2 | TNS, VMW (parcial) | Parcial |  |
| `MCPO-REQ-SEC-008` | Dica no erro de certificado | SHOULD | 2 | TNS | Resolvido |  |
| `MCPO-REQ-SEC-009` | Sem Bearer em http:// remoto | SHOULD | 2 | TNS (análogo) | Parcial |  |
| `MCPO-REQ-SEC-010` | Transport com defaults | SHOULD | 2 | nenhuma | GAP |  |
| `MCPO-REQ-SEC-011` | Leitura de arquivo só com raízes | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-SEC-012` | Sem tools administrativas | MUST | 2, 3 | OMG | Resolvido |  |
| `MCPO-REQ-SEC-013` | Saída do modelo é dado | SHOULD | 3, 4 | nenhuma | GAP |  |
| `MCPO-REQ-SEC-014` | Nada em disco sem flag | MUST | 1, 3, 4 | VMW | Resolvido |  |
| `MCPO-REQ-SEC-015` | Audit só com allowlist de campos | MUST | 3, 4 | VMW (anti-padrão) | Parcial |  |
| `MCPO-REQ-SEC-016` | Segredos nos testes ao vivo | MUST | 0, 2, 5 | TNS, VMW | Resolvido |  |

### OLL

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-OLL-001` | Cliente nativo em src/ollama | MUST | 2 | OMG | Parcial |  |
| `MCPO-REQ-OLL-002` | Context em toda requisição | MUST | 2 | nenhuma | GAP | PB5-8 |
| `MCPO-REQ-OLL-003` | Bearer em toda requisição | MUST | 2 | TNS (padrão), nenhuma em OMG | Parcial |  |
| `MCPO-REQ-OLL-004` | Mapa de status HTTP | MUST | 2 | nenhuma | GAP |  |
| `MCPO-REQ-OLL-005` | Conexão derrubada | MUST | 2 | nenhuma | GAP | PB5-10 |
| `MCPO-REQ-OLL-006` | Contadores do Ollama | MUST | 2, 3 | OMG (parcial) | Parcial |  |
| `MCPO-REQ-OLL-007` | Streaming interno robusto | SHOULD | 2 | OMG (sem tratamento de erro) | Parcial | PB5-8: stream × não-stream |
| `MCPO-REQ-OLL-008` | Timeout de inatividade | COULD | 2 | nenhuma | GAP | PB5-8 |
| `MCPO-REQ-OLL-009` | Parâmetros em allowlist | SHOULD | 3 | nenhuma | GAP |  |
| `MCPO-REQ-OLL-010` | Raciocínio não vai ao cliente | MUST | 2, 3 | OMG | Resolvido |  |
| `MCPO-REQ-OLL-011` | embed em lote | MUST | 3 | OMG (1 texto) | Parcial | PB5-6 |
| `MCPO-REQ-OLL-012` | list_models com residência | SHOULD | 3 | OMG (só tags) | Parcial |  |
| `MCPO-REQ-OLL-013` | chat sem estado e validado | MUST | 3 | OMG (sem validar `role` nem tamanho) | Parcial |  |
| `MCPO-REQ-OLL-014` | Sem retry de geração | SHOULD | 2 | nenhuma | GAP |  |
| `MCPO-REQ-OLL-015` | Buffers e corpo com limite | SHOULD | 2 | OMG (parcial) | Parcial |  |
| `MCPO-REQ-OLL-016` | Transport reutilizado | COULD | 2 | OMG, VMW | Resolvido |  |

### TOOL

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-TOOL-001` | Catálogo base | MUST | 3 | OMG | Resolvido |  |
| `MCPO-REQ-TOOL-002` | Tools de economia | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-TOOL-003` | Nome total até 60 | MUST | 3, 5 | TNS | Resolvido |  |
| `MCPO-REQ-TOOL-004` | Schema por construtor | SHOULD | 3 | nenhuma | GAP |  |
| `MCPO-REQ-TOOL-005` | title e annotations | SHOULD | 3, 4 | OMG | Parcial | `openWorldHint:false` decidido (P-9, 19:21) |
| `MCPO-REQ-TOOL-006` | Registry e helper de argumentos | MUST | 3 | VMW (registry) | Parcial |  |
| `MCPO-REQ-TOOL-007` | Tipo errado tem erro próprio | MUST | 3 | WIM | Parcial |  |
| `MCPO-REQ-TOOL-008` | Erros que orientam o modelo | SHOULD | 3 | VMW | Resolvido |  |
| `MCPO-REQ-TOOL-009` | Descrições curtas e estáticas | SHOULD | 3 | VMW (parcial: estáticas e com o pré-requisito no fim, mas longas: "string longa", `mcpvmware/20-Tools_e_Contratos.md:16`; anti-padrão nº 21 do `60`) | Parcial | B6: era "Resolvido" |
| `MCPO-REQ-TOOL-010` | Saída compacta, sem null | SHOULD | 3 | VMW | Parcial |  |
| `MCPO-REQ-TOOL-011` | generate e chat | MUST | 3 | OMG (parcial: `model` obrigatório, `ollama-mcp-go/main.go:458-460`, `487-490`, e sem `temperature`/`num_predict`) | Parcial | B6: era "Resolvido"; contradizia `CFG-003` (GAP) |
| `MCPO-REQ-TOOL-012` | embed enxuto | SHOULD | 3 | OMG (parcial) | GAP | PB5-6 |
| `MCPO-REQ-TOOL-013` | Modo assíncrono (condicional) | COULD | 5 | TNS (desenho, não o código) | Parcial | PB5-4 |
| `MCPO-REQ-TOOL-014` | Ranking por similaridade | COULD | 4 | W1 (conceito) | GAP | PB5-6 |
| `MCPO-REQ-TOOL-015` | Sem meta-tools | COULD | 3 | VMW | Resolvido |  |

### FILE

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-FILE-001` | Sem --allow-root, sem tools de arquivo | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-002` | Forma do caminho | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-003` | Contenção canônica | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-004` | Particularidades do Windows | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-005` | Só arquivo regular | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-006` | Limite de bytes | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-007` | Binário recusado | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-008` | Denylist de sensíveis | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-009` | Chunking por orçamento | MUST | 4 | nenhuma | GAP | PB5-1, PB5-2 |
| `MCPO-REQ-FILE-010` | Sem truncamento silencioso | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-011` | Tetos de custo | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-012` | Conteúdo nunca em log | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-013` | Somente leitura | MUST | 4 | nenhuma | GAP | PB5-13 |
| `MCPO-REQ-FILE-014` | Erro de contenção opaco | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-015` | Conteúdo delimitado | SHOULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-016` | Vários arquivos | COULD | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-FILE-017` | Encodings | COULD | 4 | nenhuma | GAP | PB5-2 |

### TOK

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-TOK-001` | Servidor lê, Claude não | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-TOK-002` | Linha de uso no texto | MUST | 3 | OMG (formato inadequado) | Parcial | PB5-5 |
| `MCPO-REQ-TOK-003` | Contadores também em _meta | SHOULD | 3, 5 | nenhuma | GAP | PB5-5 |
| `MCPO-REQ-TOK-004` | Economia por arquivo | SHOULD | 4, 6 | nenhuma | GAP |  |
| `MCPO-REQ-TOK-005` | tools-verbosity compact | SHOULD | 3 | VMW | Resolvido |  |
| `MCPO-REQ-TOK-006` | Orçamento do tools/list | SHOULD | 3 | OMG, VMW (medidas) | Parcial |  |
| `MCPO-REQ-TOK-007` | Saída com teto | SHOULD | 3, 4 | nenhuma | GAP |  |
| `MCPO-REQ-TOK-008` | Sem vetores por padrão | MUST | 3 | nenhuma | GAP | PB5-6 |
| `MCPO-REQ-TOK-009` | Relatório de economia medido | MUST | 6 | nenhuma | GAP | PB5-11 |

### OBS

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-OBS-001` | Log mínimo por padrão | MUST | 1, 3 | TNS | Resolvido |  |
| `MCPO-REQ-OBS-002` | --debug sem args | MUST | 1, 3 | TNS (parcial) | Parcial |  |
| `MCPO-REQ-OBS-003` | Teto de linha de log | MUST | 1 | TNS | Resolvido |  |
| `MCPO-REQ-OBS-004` | Trace opt-in | MUST | 3 | OMG (anti-padrão) | Parcial |  |
| `MCPO-REQ-OBS-005` | Banner de start | SHOULD | 1 | VMW (parcial) | Parcial |  |
| `MCPO-REQ-OBS-006` | Avisos de risco | SHOULD | 1, 4 | VMW (parcial) | Parcial |  |
| `MCPO-REQ-OBS-007` | Métrica por chamada | SHOULD | 3 | nenhuma | GAP |  |
| `MCPO-REQ-OBS-008` | Falha de stdout encerra | SHOULD | 1 | nenhuma | GAP |  |
| `MCPO-REQ-OBS-009` | Panic com stack em stderr | SHOULD | 1 | VMW (parcial) | Parcial |  |
| `MCPO-REQ-OBS-010` | Correlação por id | COULD | 1 | nenhuma | GAP |  |

### TEST

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-TEST-001` | Núcleo testável | MUST | 1 | OMG (parcial) | Parcial |  |
| `MCPO-REQ-TEST-002` | Mock httptest com NDJSON | MUST | 2 | OMG, TNS | Resolvido |  |
| `MCPO-REQ-TEST-003` | Falhas do Ollama | MUST | 2 | nenhuma (OMG não cobre) | GAP |  |
| `MCPO-REQ-TEST-004` | Casos de protocolo | MUST | 1 | VMW (medição manual) | Parcial |  |
| `MCPO-REQ-TEST-005` | TLS em cinco cenários | MUST | 2 | TNS (parcial: os testes usam `NewCertPool()`, `MCPNas/truenas-mcp/truenas/tls.go:29`, e não `SystemCertPool()` + PEM) | Parcial | B6: era "Resolvido"; mesmo ajuste de `SEC-006` |
| `MCPO-REQ-TEST-006` | Teste de vazamento de segredo | MUST | 2, 3 | TNS | Resolvido |  |
| `MCPO-REQ-TEST-007` | stdout só JSON | MUST | 1 | nenhuma | GAP |  |
| `MCPO-REQ-TEST-008` | Conjunto exato de tools | MUST | 3 | VMW | Resolvido |  |
| `MCPO-REQ-TEST-009` | Testes de arquivo | MUST | 4 | nenhuma | GAP | PB5-2 |
| `MCPO-REQ-TEST-010` | Sincronia do TOOLS.md | MUST | 3 | nenhuma (VMW sem teste) | GAP |  |
| `MCPO-REQ-TEST-011` | Testes que provam a si mesmos | SHOULD | todas | VMW | Resolvido |  |
| `MCPO-REQ-TEST-012` | Gate limpo | MUST | todas | VMW (parcial: `go clean -testcache`, `build`, `vet` e `gofmt -l` no CI, mas sem `-cover`, `MCPVMWare/.github/workflows/ci.yml:47-71`); TNS (anti-padrão: `-cover` de `tools/` quebra por BOM) | Parcial | B6: era "Resolvido" |
| `MCPO-REQ-TEST-013` | Teste de corrida | SHOULD | 1, 3 | VMW | Parcial |  |
| `MCPO-REQ-TEST-014` | Conformidade do tools/list | SHOULD | 3 | nenhuma | GAP |  |
| `MCPO-REQ-TEST-015` | Smoke stdio sem backend | MUST | 1 | VMW | Resolvido |  |
| `MCPO-REQ-TEST-016` | Smoke ao vivo e segunda via | MUST | 2, 3 | nenhuma | GAP |  |
| `MCPO-REQ-TEST-017` | Testes ao vivo atrás de tag | MUST | 2, 3 | nenhuma | GAP |  |
| `MCPO-REQ-TEST-018` | Cancelamento e concorrência | SHOULD | 3 | nenhuma | GAP | PB5-9 |

### BUILD

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-BUILD-001` | Layout src/ | MUST | 1 | VMW, TNS | Resolvido |  |
| `MCPO-REQ-BUILD-002` | Binário na raiz | MUST | 1, 0 | TNS | Resolvido |  |
| `MCPO-REQ-BUILD-003` | Versão por ldflags | MUST | 1 | VMW, TNS | Resolvido |  |
| `MCPO-REQ-BUILD-004` | Script de build e release | SHOULD | 6 | VMW | Resolvido |  |
| `MCPO-REQ-BUILD-005` | Build Linux | SHOULD | 1, 6 | TNS (medido) | Resolvido |  |
| `MCPO-REQ-BUILD-006` | TOOLS.md gerado do registro | MUST | 3, 4 | VMW | Parcial |  |
| `MCPO-REQ-BUILD-007` | Fim de linha e BOM | MUST | 1 | VMW (`MCPVMWare/.gitattributes`: `*.go text eol=lf`); TNS (anti-padrão: sem `.gitattributes` na cópia e `tools/kmip.go` com BOM) | Resolvido |  |
| `MCPO-REQ-BUILD-008` | Deploy sem matar sessões | SHOULD | 5 | VMW, TNS | Resolvido |  |
| `MCPO-REQ-BUILD-009` | gitignore e gate de status | MUST | 0, 6 | VMW (`MCPVMWare/.gitignore:6`); a cópia do TNS não tem `.gitignore` | Resolvido |  |
| `MCPO-REQ-BUILD-010` | README e CLAUDE.md fiéis | MUST | 6 | nenhuma | GAP |  |
| `MCPO-REQ-BUILD-011` | Plano atualizado por fase | MUST | todas | nenhuma | GAP |  |
| `MCPO-REQ-BUILD-012` | CI ou gate local | COULD | 6 | VMW | Resolvido |  |

### INTEG

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-INTEG-001` | Claude Code | MUST | 5 | TNS (padrão) | Parcial |  |
| `MCPO-REQ-INTEG-002` | VS Code | MUST | 5 | TNS | Resolvido |  |
| `MCPO-REQ-INTEG-003` | Cursor (decisão P-3) | MUST | 5 | TNS (formato) | Parcial | P-3 aberta |
| `MCPO-REQ-INTEG-004` | Limite de 60 no cliente real | MUST | 5 | TNS | Resolvido |  |
| `MCPO-REQ-INTEG-005` | Timeout de tool medido | MUST | 5 | TNS (padrão) | GAP | PB5-3 |
| `MCPO-REQ-INTEG-006` | Uma chamada real por cliente | MUST | 5 | nenhuma | GAP |  |
| `MCPO-REQ-INTEG-007` | Campos que chegam ao modelo | SHOULD | 5 | nenhuma | GAP | PB5-5 |
| `MCPO-REQ-INTEG-008` | Reiniciar após rebuild | SHOULD | 5 | TNS | Resolvido |  |
| `MCPO-REQ-INTEG-009` | URL por localização | MUST | 5, 6 | nenhuma | GAP | PB5-7; B6: era "Parcial" com "nenhuma", contra a definição de Parcial |
| `MCPO-REQ-INTEG-010` | Segredo por expansão de env | SHOULD | 5 | TNS (`inputs`) | Parcial |  |
| `MCPO-REQ-INTEG-011` | Nível local (decisão P-4) | SHOULD | 6 | nenhuma | GAP | P-4 aberta; B6: era "Parcial" com "nenhuma" |
| `MCPO-REQ-INTEG-012` | Testes ao vivo em série | MUST | 2, 3, 5 | nenhuma | GAP |  |

### PERF

| ID | Requisito | Prio | Fase(s) | Referência(s) que resolvem | Cobertura | GAP / pendência |
| --- | --- | --- | --- | --- | --- | --- |
| `MCPO-REQ-PERF-001` | Carga a frio não é falha | MUST | 2, 3 | nenhuma | GAP |  |
| `MCPO-REQ-PERF-002` | Fila de GPU no servidor MCP | MUST | 3 | nenhuma | GAP | PB5-4 |
| `MCPO-REQ-PERF-003` | Timeout de fila separado | SHOULD | 3 | nenhuma | GAP | PB5-4 |
| `MCPO-REQ-PERF-004` | Teto de prompt provisório | MUST | 1, 2, 4 | nenhuma | GAP | PB5-1 |
| `MCPO-REQ-PERF-005` | Sem prompt grande ao vivo | MUST | 2, 3, 4 | nenhuma | GAP | PB5-1 |
| `MCPO-REQ-PERF-006` | Sem literal de contexto | MUST | 1, 2, 4 | W1 (`CONTEXT_SIZE`, parcial) | GAP | PB5-1 |
| `MCPO-REQ-PERF-007` | Disjuntor para prompt grande | COULD | 3, 4 | nenhuma | GAP | PB5-10 |
| `MCPO-REQ-PERF-008` | Calibrar tokens por caractere | COULD | 4, 6 | nenhuma | GAP | PB5-1 |
| `MCPO-REQ-PERF-009` | Embeddings em lote | SHOULD | 3 | nenhuma | GAP |  |
| `MCPO-REQ-PERF-010` | Estimar duração | COULD | 3, 6 | nenhuma | GAP |  |
| `MCPO-REQ-PERF-011` | Cancelar libera a fila | SHOULD | 3 | nenhuma | GAP | PB5-9 |
| `MCPO-REQ-PERF-012` | Resposta rápida sem backend | MUST | 1, 3 | TNS | Resolvido |  |

## 3. Requisitos por fase e área

Conta cada requisito em todas as fases em que ele atua (um requisito de duas fases conta duas vezes). `Fase 0` aparece só como referência.

| Área | Fase 1 | Fase 2 | Fase 3 | Fase 4 | Fase 5 | Fase 6 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `PROTO` | 21 | 0 | 7 | 0 | 0 | 0 |
| `CFG` | 11 | 3 | 3 | 2 | 1 | 0 |
| `SEC` | 4 | 11 | 4 | 4 | 2 | 0 |
| `OLL` | 0 | 12 | 6 | 0 | 0 | 0 |
| `TOOL` | 0 | 0 | 12 | 3 | 2 | 0 |
| `FILE` | 0 | 0 | 0 | 17 | 0 | 0 |
| `TOK` | 0 | 0 | 6 | 3 | 1 | 2 |
| `OBS` | 8 | 0 | 4 | 1 | 0 | 0 |
| `TEST` | 7 | 8 | 10 | 3 | 2 | 2 |
| `BUILD` | 6 | 1 | 2 | 2 | 2 | 6 |
| `INTEG` | 0 | 1 | 1 | 0 | 11 | 2 |
| `PERF` | 3 | 4 | 9 | 5 | 0 | 2 |
| **Total** | **60** | **40** | **64** | **40** | **21** | **14** |

### MUST por fase (insumo do gate de cada fase)

- **Fase 1 — Esqueleto do protocolo** (44 MUST): `PROTO-001`, `PROTO-002`, `PROTO-003`, `PROTO-005`, `PROTO-006`, `PROTO-007`, `PROTO-008`, `PROTO-009`, `PROTO-010`, `PROTO-011`, `PROTO-014`, `PROTO-015`, `PROTO-016`, `PROTO-017`, `PROTO-018`, `PROTO-019`, `PROTO-020`, `CFG-001`, `CFG-002`, `CFG-003`, `CFG-004`, `CFG-005`, `CFG-010`, `CFG-011`, `SEC-001`, `SEC-003`, `SEC-007`, `SEC-014`, `OBS-001`, `OBS-002`, `OBS-003`, `TEST-001`, `TEST-004`, `TEST-007`, `TEST-012`, `TEST-015`, `BUILD-001`, `BUILD-002`, `BUILD-003`, `BUILD-007`, `BUILD-011`, `PERF-004`, `PERF-006`, `PERF-012`
- **Fase 2 — Cliente Ollama** (29 MUST): `CFG-005`, `SEC-002`, `SEC-003`, `SEC-004`, `SEC-005`, `SEC-006`, `SEC-007`, `SEC-012`, `SEC-016`, `OLL-001`, `OLL-002`, `OLL-003`, `OLL-004`, `OLL-005`, `OLL-006`, `OLL-010`, `TEST-002`, `TEST-003`, `TEST-005`, `TEST-006`, `TEST-012`, `TEST-016`, `TEST-017`, `BUILD-011`, `INTEG-012`, `PERF-001`, `PERF-004`, `PERF-005`, `PERF-006`
- **Fase 3 — Tools base** (38 MUST): `PROTO-009`, `PROTO-010`, `PROTO-014`, `PROTO-018`, `PROTO-019`, `CFG-003`, `CFG-004`, `CFG-011`, `SEC-012`, `SEC-014`, `SEC-015`, `OLL-006`, `OLL-010`, `OLL-011`, `OLL-013`, `TOOL-001`, `TOOL-003`, `TOOL-006`, `TOOL-007`, `TOOL-011`, `TOK-002`, `TOK-008`, `OBS-001`, `OBS-002`, `OBS-004`, `TEST-006`, `TEST-008`, `TEST-010`, `TEST-012`, `TEST-016`, `TEST-017`, `BUILD-006`, `BUILD-011`, `INTEG-012`, `PERF-001`, `PERF-002`, `PERF-005`, `PERF-012`
- **Fase 4 — Tools de economia (leitura local)** (23 MUST): `CFG-009`, `CFG-010`, `SEC-011`, `SEC-014`, `SEC-015`, `TOOL-002`, `FILE-001`, `FILE-003`, `FILE-004`, `FILE-006`, `FILE-008`, `FILE-009`, `FILE-010`, `FILE-012`, `FILE-013`, `TOK-001`, `TEST-009`, `TEST-012`, `BUILD-006`, `BUILD-011`, `PERF-004`, `PERF-005`, `PERF-006`
- **Fase 5 — Integração com os clientes** (14 MUST): `CFG-005`, `SEC-001`, `SEC-016`, `TOOL-003`, `TEST-012`, `BUILD-011`, `INTEG-001`, `INTEG-002`, `INTEG-003`, `INTEG-004`, `INTEG-005`, `INTEG-006`, `INTEG-009`, `INTEG-012`
- **Fase 6 — Medição e documentação** (6 MUST): `TOK-009`, `TEST-012`, `BUILD-009`, `BUILD-010`, `BUILD-011`, `INTEG-009`

- **Fase 0 (só referência):** `SEC-016`, `BUILD-002`, `BUILD-009`.

## 4. GAPs

O que nenhuma referência resolve. Cada GAP tem decisão proposta; as propostas são **do B5** e dependem de aprovação do owner. Requisitos afetados são os que citam o `PB5-n` na matriz.

| PB5 | GAP | O que as referências dizem | Decisão proposta | Quem fecha | Requisitos afetados |
| --- | --- | --- | --- | --- | --- |
| `PB5-1` | **Limite utilizável de prompt, contexto e orçamento de chunk** | Nenhuma referência trata. `VAL §7`: prompt de 42.540 caracteres derruba o serviço a ~45 s (2×); o limiar é desconhecido; `num_ctx` real é 131072, não 32k. | `--max-prompt-chars` configurável, com default **provisório** e conservador abaixo de 42.540 caracteres; sem literais de contexto no código; chunking pelo teto; nenhum teste ao vivo acima de 42.540 caracteres sem ordem do owner. A Fase 4 pode ser construída e testada com mock, mas só abre ao vivo depois de PB4-2 (limiar medido) ou com o teto provisório. O owner investiga PB4-1/PB4-2. | Owner (PB4-1, PB4-2) | `CFG-010`, `FILE-009`, `PERF-004`, `PERF-005`, `PERF-006`, `PERF-008` |
| `PB5-2` | **Leitura de arquivo: allowlist, symlink/junction, Windows, denylist, chunking** | Nenhuma referência resolve: o `mcpvmware` lê `local_path` livre (`MCPVMWare/src/tools/datastore.go:71-109`), `use_curl` do `W7` não tem limite, e o `memory_get` do `W6` só tem o desenho ("caminhos fora do workspace são recusados"), sem código lido. | Pacote próprio (`src/tools/files` ou `src/files`) implementando MCPO-REQ-FILE-001 a 017 e os testes de TEST-009, com casos Windows (junction, UNC, `\\?\`, ADS, dispositivos). Sem `--allow-root`, as tools não existem. | Fase 4 | `CFG-009`, `SEC-011`, `TOOL-002`, `FILE-001`, `FILE-002`, `FILE-003`, `FILE-004`, `FILE-005`, `FILE-006`, `FILE-007`, `FILE-008`, `FILE-009`, `FILE-010`, `FILE-011`, `FILE-012`, `FILE-014`, `FILE-015`, `FILE-016`, `FILE-017`, `TOK-001`, `TEST-009` |
| `PB5-3` | **Timeout de tool dos clientes MCP desconhecido** | Nenhuma referência mediu. `TNS` usou espera de 100 s sob um limite de 120 s, mas o `120 s` do `truenas-mcp` é do cliente WebSocket (`MCPNas/truenas-mcp/truenas/client.go:815-821`); o limite dos clientes MCP é **inferência** não confirmada. | Medir em Claude Code, Cursor e VS Code na Fase 5 (carga a frio ~36 s e geração longa) antes de fixar o default de `--timeout`; até lá, default provisório e documentado. Se algum cliente cortar antes de uma geração real, ativar o modo assíncrono (PB5-4). | Fase 5 | `CFG-005`, `INTEG-005` |
| `PB5-4` | **Fila de GPU e geração longa (modo assíncrono)** | Nenhuma referência tem fila de GPU. O `truenas-mcp` dá o **desenho** `task_id` (`MCPNas/truenas-mcp/tasks/types.go:8-51`), mas o `Poller` é acoplado ao TrueNAS e o `Result` nunca chega ao cliente (bug-015). | Semáforo N=1 antes do cliente HTTP (PERF-002, PERF-003) já na Fase 3. Modo assíncrono (`task_id`, `tasks_get`, `tasks_list`, `tasks_cancel`) só se PB5-3 mostrar necessidade; reusar `TaskStore` e `Task`, corrigindo `Result`, lock e cancelamento. | Fase 3 (fila); Fase 5 (assíncrono, condicional) | `TOOL-013`, `PERF-002`, `PERF-003` |
| `PB5-5` | **Contadores no formato MCP** | O `ollama-mcp-go` põe `prompt_eval_count` e `eval_count` em campos de topo (`ollama-mcp-go/main.go:86-87`), que um cliente pode ignorar (inferência). Nenhuma referência valida o que chega ao modelo. | Linha final de uso em `content[].text` (sempre funciona) mais `_meta` ou `structuredContent`; descobrir empiricamente, por cliente, na Fase 5 (INTEG-007). | Fase 3 (texto); Fase 5 (validação) | `TOK-002`, `TOK-003`, `INTEG-007` |
| `PB5-6` | **Contrato do `embed` e ranking por similaridade** | `ollama-mcp-go` devolve só dimensões e 16 valores (`ollama-mcp-go/main.go:580-589`); `W1` tem `filter_docs`, sem código lido; `VAL §6` mostra 768 dimensões e custo de recarga. | Default de resumo (dimensões, quantidade, contadores); `include_vectors:true` com teto; `input` em lote; modelo `--embed-model`. `rank_files` fica COULD. | Fase 3 | `OLL-011`, `TOOL-012`, `TOOL-014`, `TOK-008` |
| `PB5-7` | **Caminho internet (Cloudflare) e TLS público não validados** | `VAL §1`: de dentro da LAN, o host sem porta dá timeout; só `:8443` foi medido. `SystemCertPool` mais a CA funciona na LAN; a metade "cadeia pública" é teoria. | Aceitar o desenho (SEC-006); validar de fora da LAN ou com DNS público (PB4-3); documentar as duas URLs (INTEG-009). Não bloqueia as fases. | Owner (PB4-3) | `SEC-006`, `INTEG-009` |
| `PB5-8` | **Streaming × não-streaming e prazos no cliente Ollama** | `VAL §7`: a falha ocorre antes dos cabeçalhos, então o streaming não resolve o defeito. O `ollama-mcp-go` usa stream sem tratar `error`/`done_reason`, e o `Timeout` de 5 min cobre o corpo inteiro. | Fase 2 decide com teste: `stream:true` com tratamento de `error`, EOF sem `done` e `done_reason`, `context` por chamada e timeout de inatividade (COULD); `stream:false` fica como opção se o stream não trouxer ganho medido. | Fase 2 | `OLL-002`, `OLL-007`, `OLL-008` |
| `PB5-9` | **Loop concorrente e `notifications/cancelled`** | As referências com loop próprio (`ollama-mcp-go`, `mcpvmware`, `truenas-mcp`) são sequenciais ou só têm mutex de stdout sem concorrência (`VMW-10 §3`, `TNS-10`, `OMG-10`). | Goroutine por `tools/call` com `context`, mutex de escrita (já existe em `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:286-301`), cancelamento por `requestId` e `-race` nos testes. | Fases 1 e 3 | `PROTO-012`, `PROTO-013`, `TEST-018`, `PERF-011` |
| `PB5-10` | **Resiliência a queda do serviço Ollama** | `VAL §7` e `§8.2`: o servidor derruba a conexão sem corpo e descarrega os modelos; nenhuma referência trata esse sintoma. | Erro de tool específico (OLL-005); disjuntor para prompts grandes (PERF-007) como COULD; mensagem que cita carga a frio de ~36 s. | Fase 2 | `OLL-005`, `PERF-007` |
| `PB5-11` | **Qualidade do modelo local e economia medida** | `SRV` e `CONEXAO-IA.md:133-136` afirmam qualidade equivalente entre os modelos do bake-off; **não foi medido**. A economia de tokens do mcpollama também não foi medida em nenhuma referência. | Relatório da Fase 6 com caso real antes e depois: tokens Anthropic medidos, tempo e avaliação da resposta local contra a do Claude em amostra pequena (TOK-009). | Fase 6 | `TOK-009` |
| `PB5-12` | **Protocolo moderno 2026-07-28** | `OMG-10`: só o `ollama-mcp-go` implementa (`server/discover`, `resultType`, `ttlMs`, `-32022`) e **não foi conferido** com a especificação oficial. | Não implementar nesta versão (PROTO-022, COULD). Se um cliente exigir, conferir a especificação oficial antes. | Pós-Fase 6 | `PROTO-022` |
| `PB5-13` | **Escrita de saída em disco (`output_path`)** | Nenhuma referência discute; `PLN` não prevê. | Proposta: **sem escrita**; resultado só na resposta MCP (FILE-013, SEC-014). Escrever em disco exigiria allowlist de escrita separada e é decisão do owner se um dia for pedido. | Owner (se pedido) | `FILE-013` |

> **Atualização do PB5-1 (2026-10-23 19:18, B7):** o defeito foi diagnosticado (reboot do host por PCIe da GPU 3) e mitigado (3 GPUs, `num_ctx` 65536). A 2ª via passou com 42.540 caracteres = 15.959 tokens em 3,2 s (`VAL §9`). O teto "abaixo de 42.540 caracteres" **deixa de valer**; o default de `--max-prompt-chars` passa a derivar do contexto configurado. O owner sugere ~48k tokens de entrada; a decisão fica na P-6. `--max-prompt-chars` continua configurável, porque o contexto volta a 131072 após o reparo.

Também são GAP, sem número próprio por serem de baixo risco: `CFG` (default sem `localhost`, `--help` sem segredos), `SEC` (Bearer só no cabeçalho, corpo de erro limitado, `Transport` com defaults, saída do modelo como dado), `OBS` (falha de stdout, correlação por id) e `TEST` (núcleo testável, stdout só JSON, testes ao vivo atrás de tag). Todos têm requisito e verificação em `60`.

### Limitações do próprio brief que afetam esta matriz

- `W2` (mcp.so): só a 1ª de 309 páginas foi lida (PB3-1); nada sobre servidores Ollama com leitura de arquivo foi confirmado fora dessa amostra.
- `W7`: o `main.go` do exemplo 06 e do 07 não carregou (PB3-2). `W6`: `flux7-console` e `keys7` não foram lidos (PB3-3).
- `OMG`: o modo 2026-07-28, o `bufio.ErrTooLong` e o trace em `/tmp` no Windows não foram executados; a leitura dos campos de topo pelos clientes não foi testada.
- `TNS`: cobertura de `tools/` não medida (BOM em `tools/kmip.go`), `-race` não rodado (PB2-2); a cópia não tem `go.mod`.
- `VMW`: suíte completa não rodada (PB2-1); divergência 909 × 924 tools não reconciliada (PB2-3).
- `VAL`: o limiar de prompt e o caminho internet **não** foram medidos; a qualidade do modelo é afirmação do guia.

## 5. Impacto nas pendências do plano de implementação

| Pendência | Estado no plano | Estado após o B5 | Insumo do brief |
| --- | --- | --- | --- |
| P-1 (`--api-key-file`) | decidida: não | **mantida: não** | Consistente com `SEC-001`. A contradição do `ollama-mcp-go/30-Configuracao_e_Seguranca.md:33` ("ou de arquivo") foi corrigida no B5; os docs do `truenas-mcp` e do `mcpvmware` já estavam corrigidos (conferido no B6: 0 menções a `--api-key-file` como opção) |
| P-2 (modelo 32k × 64k) | aberta | **SUPERADA** | `VAL §3`: `qwen3-ia` é o MoE `qwen3-coder:30b-a3b`, `num_ctx` 131072; o `qwen3.8:27b-64k` é o modelo do nível `local`, não do `qwen3-ia`. O limite utilizável é desconhecido (PB5-1). O texto do plano que fala em "contexto de 32k" (Objetivo 5, Fase 4 passo 3, Riscos) e a seção do servidor no `CLAUDE.md` e no `README.md` devem ser reescritos (`BUILD-010`) |
| P-3 (config do Cursor) | aberta | **aberta, do owner** | Cursor usa `mcpServers`; limite `servidor + tool <= 60` (`TOOL-003`). `.cursor/` é o SSOT dos espelhos (`CLM`); o token vai por env, então a config por projeto não carrega segredo. Decidir via AskUserQuestion (`INTEG-003`) |
| P-4 (nível `local` global) | aberta (após a Fase 6) | **aberta, do owner** | O `ia-local.ps1` usa `qwen3.8:27b-64k` via OpenClaude (`SRV`); a tool MCP usa `qwen3-ia`. Trocar muda de modelo, não só de transporte (`INTEG-011`) |
| P-5 (1º commit) | aberta | **aberta, do owner** | `referencias/*` está ignorada, exceto o brief (`BUILD-009`); o pack `.cursor/` tem 1101 arquivos (`PLN`, changelog 15:10); `core.symlinks=false` |
| PB4-1, PB4-2, PB4-3 | abertas (plano do brief) | **abertas, do owner** | Alimentam PB5-1 e PB5-7 |

**Novas pendências sugeridas** (a levar ao owner no B7, via AskUserQuestion):

| Nº sugerido | Decisão | Depende de |
| --- | --- | --- |
| P-6 | Valor provisório de `--max-prompt-chars` e política de chunking enquanto o limiar for desconhecido | PB4-1, PB4-2 (PB5-1). **Decidida (19:21): default ≈ 128.000 caracteres, ~48k tokens** |
| P-7 | Default de `--timeout` e adoção (ou não) do modo assíncrono | Medição da Fase 5 (PB5-3, PB5-4) |
| P-8 | Conjunto final de tools: incluir `rank_files` (COULD) e tools assíncronas? | PB5-4, PB5-6 |
| P-9 | `openWorldHint` e anotações das tools para um Ollama remoto e privado | `TOOL-005`. **Decidida (19:21): `false`** |
| P-10 | Política de denylist e de override da denylist (`.env`, `*.pem`...) e regras de caminho no Windows | PB5-2 |

### Ajustes sugeridos às fases do plano (para o B7)

| Fase | Acrescentar ou corrigir |
| --- | --- |
| 1 | Flags `--max-line-bytes`, `--embed-model`, `--max-prompt-chars` (com env); `ping` → `{}`; `-32700` com `id:null`; leitura sem o limite de 64 KB; smoke sem backend; `Run()` testável; stdout só JSON |
| 2 | `SystemCertPool` mais CA (não `NewCertPool`); `context` por chamada; mapa de status 401/403/404/5xx; conexão derrubada como erro; decisão stream × não-stream (PB5-8); corpo de erro limitado |
| 3 | Semáforo de GPU e loop concorrente (PB5-4, PB5-9); linha de uso nos resultados (PB5-5); `embed` com `--embed-model` (PB5-6); `dumptools` com teste de sincronia; teste de conjunto exato de tools |
| 4 | Remover "contexto de 32k" do texto; chunking por `--max-prompt-chars`; casos Windows e denylist (PB5-2); liberar ao vivo só depois de PB4-2 ou com teto provisório |
| 5 | Medir timeout de cada cliente (PB5-3); verificar o limite de 60 no Cursor; descobrir campos que chegam ao modelo (PB5-5); `.mcp.json` sem segredo literal |
| 6 | Relatório de economia com tokens medidos e avaliação de qualidade (PB5-11); corrigir `README.md` e `CLAUDE.md` (servidor: ctx 131072) |

## Changelog

- **1.2.0 (2026-10-23):** B7, com as decisões do owner das 19:21: `PROTO-009` passa a **MUST** (PB6-1), então a Fase 1 fica com **44 MUST** e a Fase 3 com **38 MUST**; a tabela por fase × área não muda, porque conta todas as prioridades; P-6 (default ≈ 128.000 caracteres) e P-9 (`openWorldHint:false`) decididas; nota do PB5-1 sobre a mitigação do servidor (`num_ctx` 65536).
- **1.1.0 (2026-10-23):** B6 (revisão adversarial, `70-Revisao.md`). Cobertura recontada: **48 resolvidos, 47 parciais, 76 GAP** (antes 53/45/73). `SEC-002` vira GAP (o TNS manda a chave no corpo, não em cabeçalho); `TEST-005`, `TEST-012` (CI do VMW sem `-cover`), `TOOL-009` e `TOOL-011` viram Parcial (a referência tem o defeito oposto ou só parte); `INTEG-009` e `INTEG-011` viram GAP (estavam "Parcial" com "nenhuma"). `BUILD-007` e `BUILD-009`: TNS deixa de constar como solução (sem `.gitattributes` nem `.gitignore` na cópia). `PROTO-009` conferido na especificação. §5: a contradição do `OMG-30:33` já estava corrigida. MUST por fase e matriz fase × área inalteradas (não dependem da cobertura).
- **1.0.0 (2026-10-23):** B5. 171 requisitos × Fases 0 a 6; cobertura: 53 resolvidos, 45 parciais, 73 GAP; 13 GAPs numerados (`PB5-1` a `PB5-13`). Gerado do mesmo conjunto de dados de `60-Requisitos_mcpollama.md`.
