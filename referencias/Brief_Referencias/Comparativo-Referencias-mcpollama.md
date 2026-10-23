# Comparativo das referências do mcpollama

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.1.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B5 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Compara as referências nas dimensões que decidem a construção do **mcpollama**, diz o que tirar de cada uma e o que não copiar, e fecha com a conclusão. Os números vêm dos docs de cada referência (medidos nas ondas B1 a B4); o que é inferência está marcado. IDs `MCPO-REQ-*` (abreviados, sem o prefixo) estão em [60-Requisitos_mcpollama.md](60-Requisitos_mcpollama.md); a cobertura por fase e os GAPs estão em [61-Matriz_Cobertura.md](61-Matriz_Cobertura.md). Chaves de fonte (`OMG-10`, `VMW-30`, `W6`...) são as de `60`.

## 1. Tabela comparativa

Referências: **OMG** = `ollama-mcp-go`; **WIM** = `04-what-is-mcp`; **VMW** = `MCPVMWare`; **TNS** = `MCPNas/truenas-mcp`; **Web** = as 7 fichas (só o que traz dado técnico).

| Dimensão | OMG | WIM | VMW | TNS | Web |
| --- | --- | --- | --- | --- | --- |
| **Tamanho e papel** | 4 arquivos Go, 1.426 linhas, 4 tools (`OMG-README`). Servidor de Ollama, a referência mais próxima | 1 arquivo Go, 47 linhas, 1 tool `hello_world` (`WIM-README`). Exemplo didático | 176 arquivos Go, 85.397 linhas, 1.047 tools (`VMW-README`). Domínio VMware; vale pelos mecanismos | 116 arquivos Go, 109.126 linhas, 730 tools (`TNS-README`). Domínio TrueNAS, backend WebSocket | `W1` Go, 3 tools (`reason_task`, `embed_text`, `filter_docs`); `W3` Node/TS, 4 tools; `W7` Go, 1 tool `use_curl` |
| **Transporte e framing** | stdio, 1 JSON por linha, `Scanner` de 4 MB (`OMG-10`) | stdio pelo SDK (`ServeStdio`); framing interno do SDK não lido (inferência) (`WIM-10`) | stdio, 1 JSON por linha, `Scanner` de 64 KB (`VMW-10 §3`) | stdio, 1 JSON por linha, `Scanner` de 64 KB (`TNS-10`) | `W1`, `W6`, `W7`: stdio. `W3`: não declarado. `flux7-memory` (`W6`) também HTTP |
| **SDK ou tipos à mão** | À mão, 0 dependências (`OMG-README`) | SDK de terceiros `mark3labs/mcp-go` v0.8.2 (`WIM-10`) | À mão (`mcp/types.go`); só `govmomi` de domínio (`VMW-README`) | À mão (`mcp/types.go`); `gorilla/websocket` e `uuid` (`TNS-README`) | `W7` usa o SDK `mark3labs`; `W1` não verificado |
| **Configuração** | 2 env, 0 flags, sem `--version` (`OMG-30`) | Nenhuma (`WIM-10`) | 16 flags; env só para parte (assimetria) (`VMW-30 §1`) | 6 flags, 5 delas com env (`--version` não tem), flag vence (`TNS-30`) | `W1`: 4 env (3 obrigatórias). `W7`/06: `OLLAMA_HOST`, `TOOLS_LLM`, `CHAT_LLM`. `W3`: `OLLAMA_HOST` |
| **Autenticação** | Nenhuma: sem `Authorization` (`OMG-30`) | Nenhuma | Login SOAP, Basic e refresh token, **no boot**, com `log.Fatalf` (`VMW-10 §8`) | Chave de API por sessão WebSocket, **lazy** (`TNS-30`) | `W1`, `W3`: não documentada. `flux7-memory`: Bearer `MEM7_TOKEN` (`W6`) |
| **TLS e CA** | Pool do SO, sem CA nem `--insecure` (`OMG-30`) | Não se aplica | Só `--insecure` (`VMW-30 §2`) | `--tls-ca` e `--insecure`; o pool do SO é **substituído** (`MCPNas/truenas-mcp/truenas/tls.go:33`) (`TNS-30`) | `W4` recomenda TLS e proxy com token na frente do Ollama, sem detalhe |
| **Segredos em log e erro** | Erro com URL inteira; trace com prompt, resposta e raciocínio, ligado por padrão (`OMG-30`) | Não se aplica | **Vaza**: `args` no audit (medido) e URL no erro (medido) (`VMW-30 §5, §6`) | Redação em camadas: valor exato, nome de campo, log mínimo; teto de 4.000 bytes (`TNS-30`) | Sem dado |
| **Leitura de arquivo local** | Nenhuma tool (`OMG-30`) | Nenhuma | `vmware_datastore_upload_file` com `local_path` livre, sem allowlist, symlink ou limite (`VMW-30 §7`) | Não tratada no brief | `W7` `use_curl` sem limite; `W6` `memory_get` recusa caminho fora do workspace (só README, sem código lido) |
| **Testes** | 26 testes, 75,2%; `httptest` com NDJSON; `main()` não testado (`OMG-50`) | Nenhum | 86 arquivos de teste, 485 `Test*`, `vcsim`/`httptest`; loop stdio sem teste (`VMW-50 §1`) | 64 testes (27 + 37); 70,6% em `truenas/`; `cmd/`, `mcp/`, `tasks/` sem teste (`TNS-50`) | `W3`, `W6`, `W7`: nenhum descrito |
| **Build, versão e release** | `go build`; versão constante `0.2.0`; sem release (`OMG-50`) | Nenhum; versão fixa no código (`WIM-10 §6`) | `Build-Release.ps1`, `-ldflags`, `SHA256SUMS`, CI no GitHub, Windows-only (`VMW-50 §3-5`) | `-ldflags -X main.Version`; `Makefile` só citado (fora da cópia); cross-compile medido (`TNS-50`) | `W7`: Docker em 2 estágios (não se aplica) |
| **Economia de tokens** | Contadores em campos de topo; raciocínio só no trace; `tools/list` de 1.901 bytes (`OMG-20`, `OMG-README`) | A descrição entra no `tools/list` (`WIM-README`) | `compact` corta -63% a -67% do `tools/list`; JSON indentado custa tokens (`VMW-20 §3, §5`) | `tools/list` de 892.704 bytes (730 tools); respostas simplificadas (inferência) (`TNS-README`, `TNS-20`) | `W1` `filter_docs` ranqueia no servidor; `W6` raciocínio fora da resposta; `W5` `--quiet` |
| **Robustez: buffer, timeout, concorrência** | Linha acima de 4 MB mata (inferência); `Timeout` de 5 min cobre o corpo; sequencial, sem `context` (`OMG-10`, `OMG-30`) | Não se aplica | Linha acima de 64 KB **mata** (medido); sem timeout; sequencial (`VMW-10 §3, §8`) | Linha acima de 64 KB mata; 120 s no cliente de backend; sequencial, mas com mutex de stdout (`TNS-10`) | `W3` timeout de 60 s; `W7`/06 contexto de 30 s |
| **Protocolo** | Negocia versões; `ping` na versão legada; modo 2026-07-28 não verificado (`OMG-10`) | Pelo SDK | Versão fixa `2024-11-05`; `ping` devolve `-32601` (medido) (`VMW-10 §3`) | Versão fixa `2024-11-05`; `ping` devolve `-32601` (medido) (`TNS-10`) | `W6`: negociação do `ollama-mcp-go` (mesma referência) |
| **Erros de tool** | `isError`; tool desconhecida vira `-32602` (`OMG-20`) | `NewToolResultError` (`WIM-20`) | `isError` com `Error:` e texto para o modelo agir (`VMW-20 §4`) | `isError` com `Error:` e contexto, com parâmetros redigidos (`TNS-20`) | `W7`: erro explícito para tipo errado |

O servidor Ollama da CSL (`SRV`, `VAL`) não é uma referência de código, mas fixa os limites que as colunas acima não têm: fila única de GPU, carga a frio de ~32 a 36 s, 401 sem token, 403 em endpoint administrativo e o defeito de prompt grande (`VAL §5, §7`).

## 2. O que tirar de cada referência

### `ollama-mcp-go` (base do núcleo)

- **Copiar o desenho:** tipos JSON-RPC e MCP à mão, `dispatch` por `switch` (`PROTO-020`); `negotiate` de versão (`PROTO-005`) e `ping` (`PROTO-007`); erro de domínio como `isError` (`PROTO-010`); stderr só para log (`PROTO-016`); contadores do chunk final (`OLL-006`); raciocínio fora da resposta (`OLL-010`); `title` e anotações (`TOOL-005`); mock `httptest` com NDJSON real (`TEST-002`).
- **Não copiar:** trace ligado por padrão em `/tmp` (`OBS-004`); JSON inválido sem resposta (`PROTO-003`); URL inteira no erro (`SEC-004`); ausência de auth e de TLS; `embed` que devolve 16 valores (`TOOL-012`); buffer de 4 MB com loop sequencial (`PROTO-002`, `PROTO-012`); texto "Nothing leaves the machine" (`PROTO-021`); `model` obrigatório sem default (`CFG-003`).

### `04-what-is-mcp` (contraste)

- **Aproveitar pouco:** erro de domínio devolvido como resultado (`PROTO-010`) e validação de tipo no handler (`TOOL-007`).
- **Não copiar:** `fmt` no stdout (`PROTO-016`); erro sem código de saída (`PROTO-015`); SDK de terceiros, que o CLAUDE.md recusa (`PROTO-020`); versão fixa no código (`BUILD-003`); higiene do repositório (`go.mod` da raiz de outro exemplo, `compose.yml` vazio).

### `MCPVMWare` (molde CSL: mecanismos e armadilhas pagas)

- **Copiar:** registry com definição e handler (`TOOL-006`); ordenação do `tools/list` (`PROTO-014`); `compact` com a regra "poda prosa, nunca estrutura" (`TOK-005`); `recover` por chamada (`PROTO-019`); `dumptools` para o `TOOLS.md` (`BUILD-006`); `Build-Release.ps1` e versão por `-ldflags` (`BUILD-003`, `BUILD-004`); teste de conjunto exato (`TEST-008`); regra única de booleano na env, corrigindo a assimetria (`CFG-002`); testes que provam a si mesmos e CI provado por teste negativo (`TEST-011`, `BUILD-012`); marshal único sem `null` (`TOOL-010`); deploy por renomeação do `.exe` (`BUILD-008`); `.gitattributes` com `*.go text eol=lf` (`BUILD-007`); a regra de não tocar a rede antes do loop (`PROTO-011`).
- **Não copiar (medido):** `Scanner` de 64 KB (`PROTO-002`); audit com `args` em texto puro (`SEC-015`); `--debug` que loga argumentos (`OBS-002`); URL com segredo no erro (`SEC-004`); `ping` sem resposta e versão fixa; conexão ao backend no boot com `log.Fatalf`; ausência de timeout por chamada; tool de arquivo sem allowlist (`FILE-001`); `CLAUDE.md` que era o template Delphi (`VMW-80`, LIC-VMW-035).

### `MCPNas/truenas-mcp` (molde CSL: infraestrutura de cliente real)

- **Copiar:** `--tls-ca` e `--insecure` com a dica no erro x509 (`SEC-006`, `SEC-007`, `SEC-008`) e os 4+1 testes de TLS (`TEST-005`); autenticação lazy (`PROTO-011`); teto de 4.000 bytes por linha de log (`OBS-003`); redação do valor exato do segredo (`SEC-003`) com teste de captura de log (`TEST-006`); o desenho de `task_id` para gerações longas, se a Fase 5 o exigir (`TOOL-013`); o limite de 60 caracteres do Cursor (`TOOL-003`); formatos `mcpServers` e `servers` (`INTEG-002`, `INTEG-003`); a lição de CRLF e BOM (`BUILD-007`, LIC-TNS-021/022). A cópia do TNS **não** tem `.gitattributes` e o `tools/kmip.go` tem BOM: o `.gitattributes` a copiar é o do `MCPVMWare`.
- **Ajustar ao copiar:** o `tls.go:33` substitui o pool do SO, então o mcpollama usa `SystemCertPool()` mais a CA de origem (`SEC-006`); o `Result` da task tem `json:"-"`, então precisa ser exposto.
- **Não copiar:** `proxy/` (esqueleto morto, ninguém importa); `tools/list` em ordem aleatória por `map`; `ping` sem resposta; ausência de teste do loop stdio; o `MCP-VSCODE.md` que fala em "Content-Length" (herdado do Delphi, contradiz o código).

### Web

- **`W1` (dxeo):** modelos separados por papel (`CFG-004`); `filter_docs` como ideia de ranking no servidor (`TOOL-014`); `CONTEXT_SIZE` explícito como conceito (`PERF-006`). Projeto sem auth nem TLS documentados; a página do marketplace tem **prompt injection** dirigida a agentes (não seguida), prova de que conteúdo externo é dado (`SEC-013`).
- **`W3` (NightTrek):** formato `chat_completion` com `messages` e `temperature` (`OLL-013`); timeout de 60 s como ponto de partida. **Não copiar** `pull`, `create`, `push`, `copy`, `delete` (`SEC-012`). O `mcpservers.org` deu 403; só o README do GitHub foi lido.
- **`W4`:** corrobora TLS mais proxy com token na frente do Ollama (`SEC-006`); sem detalhe de implementação.
- **`W5`:** formato `mcpServers` com `command`/`args`/`env` (`INTEG-001`). A premissa "Ollama como cliente MCP" **não** se confirma: lá o Ollama é provedor de modelo.
- **`W6` (KTCrisis):** raciocínio fora da resposta e trace opt-in (`OLL-010`, `OBS-004`); Bearer mais escopo de leitura por identidade como desenho para a leitura de arquivo (`FILE-001`). `flux7-*` são governança e memória, fora do escopo.
- **`W7` (ollama-tlms-golang):** contrato mínimo de um parâmetro e erro explícito (`TOOL-011`); o anti-exemplo da `use_curl` sem limite de bytes reforça (`FILE-006`). Os exemplos 06 e 07 não carregaram.
- **`W2` (mcp.so):** 1 de 309 páginas; nenhum cliente Ollama com leitura de arquivo no servidor foi achado nessa amostra, sem valor de conclusão.

### Servidor Ollama da CSL (`SRV`, `VAL`)

- **Aplicar:** `nomic-embed-text` para embeddings (`CFG-004`); `SystemCertPool()` mais a CA (`SEC-006`); conexão derrubada vira erro claro (`OLL-005`); carga a frio de ~32 a 36 s não é falha (`PERF-001`); teto de prompt provisório (`PERF-004`); nenhum literal de contexto (`PERF-006`).
- **Evitar:** assumir 32k (o `qwen3-ia` hoje tem `num_ctx` 131072) ou 131072 (o limite utilizável é menor e desconhecido); chamadas paralelas (fila única de GPU).

## 3. O que nenhuma referência resolve (GAPs)

Resumo; o detalhe, a decisão proposta e os requisitos afetados estão em `61-Matriz_Cobertura.md` §4.

| PB5 | GAP |
| --- | --- |
| `PB5-1` | Limite utilizável de prompt, contexto e orçamento de chunk |
| `PB5-2` | Leitura de arquivo: allowlist, symlink/junction, Windows, denylist, chunking |
| `PB5-3` | Timeout de tool dos clientes MCP desconhecido |
| `PB5-4` | Fila de GPU e geração longa (modo assíncrono) |
| `PB5-5` | Contadores no formato MCP |
| `PB5-6` | Contrato do `embed` e ranking por similaridade |
| `PB5-7` | Caminho internet (Cloudflare) e TLS público não validados |
| `PB5-8` | Streaming × não-streaming e prazos no cliente Ollama |
| `PB5-9` | Loop concorrente e `notifications/cancelled` |
| `PB5-10` | Resiliência a queda do serviço Ollama |
| `PB5-11` | Qualidade do modelo local e economia medida |
| `PB5-12` | Protocolo moderno 2026-07-28 |
| `PB5-13` | Escrita de saída em disco (`output_path`) |

## 4. Limitações deste comparativo

- As células marcadas "(inferência)" não têm leitura ou medição direta no brief.
- `WIM`: o SDK `mark3labs/mcp-go` não foi lido (pendência do `what-is-mcp`).
- `OMG`: comportamento com linha acima de 4 MB e o trace no Windows não foram executados; o modo 2026-07-28 não foi conferido com a especificação.
- `TNS`: a cópia não tem `go.mod`; a cobertura de `tools/` não foi medida; `-race` não rodado.
- `VMW`: suíte completa não rodada; 909 × 924 tools não reconciliado.
- `W2`, `W6`, `W7`: cobertura parcial (1 de 309 páginas; 2 repositórios não lidos; 2 exemplos não carregaram).
- Nenhuma referência foi medida contra o servidor da CSL; só o servidor foi (`VAL`).

## 5. Conclusão em uma linha

**O mcpollama é o núcleo mínimo do `ollama-mcp-go` com a configuração, a segurança e o build do molde CSL (`mcpvmware`, `truenas-mcp`), corrigindo as armadilhas medidas (`Scanner` de 64 KB, `ping`, boot com rede, audit e erro vazando segredo) e inventando o que nenhuma referência tem: leitura de arquivo com allowlist e chunking, fila de GPU e os limites reais de prompt do servidor.**

## Changelog

- **1.1.0 (2026-10-23):** B6 (`70-Revisao.md`). O `.gitattributes` saiu de "copiar do TNS" (a cópia não tem o arquivo e tem BOM em `tools/kmip.go`) e entrou em "copiar do `MCPVMWare`".
- **1.0.0 (2026-10-23):** B5. Tabela de 14 dimensões, "o que tirar" por referência com IDs de requisito e conclusão em uma linha.
