# ollama-mcp-go — visão geral

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## O que é

Servidor **MCP em Go, stdio, JSON-RPC 2.0**, de um único pacote `main`, sem dependências externas (`ollama-mcp-go/go.mod:1-3` não tem `require`). Expõe o Ollama a clientes MCP por **4 tools**: `list_models`, `generate`, `chat`, `embed` (`ollama-mcp-go/main.go:172-230`). Autor/origem: `github.com/KTCrisis/ollama-mcp-go` (`ollama-mcp-go/go.mod:1`), licença MIT (`ollama-mcp-go/README.md:184-186`), versão do servidor `0.2.0` (`ollama-mcp-go/main.go:59`). Histórico git local: 8 commits, o último `16dfa22 Merge feat/mcp-2026-07-28` (`git log --oneline`, medido em 2026-10-23).

Ideias de design declaradas pelo autor (`ollama-mcp-go/README.md:7-14`): binário único; só tools "de agente" (sem `pull`/`push`/`delete`); funciona atrás de um gateway (agent-mesh); e **trace de tokens em arquivo**, porque o MCP não entrega resultado parcial.

Estado de verificação (rodado nesta onda, 2026-10-23, Go 1.26.5, cache de teste desligado):

- `go test -count=1 -v ./...` → 26 testes `PASS` (contados na saída), `ok ... 0.110s`;
- `go test -count=1 -cover ./...` → **75,2% das instruções**;
- `go vet ./...` → sem saída;
- `git status --short` na pasta → vazio (a execução não alterou a referência).

## Fontes (linhas físicas, `wc -l`)

| Arquivo | Linhas | Papel |
| --- | ---: | --- |
| `ollama-mcp-go/main.go` | 662 | tipos JSON-RPC/MCP/Ollama, tools, streaming, trace, loop stdio |
| `ollama-mcp-go/main_test.go` | 562 | testes das tools com mock `httptest`, streaming, trace, round-trip |
| `ollama-mcp-go/protocol.go` | 109 | negociação de versão, `dispatch`, `server/discover`, `initialize` |
| `ollama-mcp-go/protocol_test.go` | 93 | testes do protocolo "dual-era" |
| **Subtotal Go** | **1.426** | confere com o dimensionamento do brief (`Brief_Referencias/README.md`) |
| `ollama-mcp-go/README.md` | 186 | documentação do autor |
| `ollama-mcp-go/go.mod` | 3 | `module github.com/KTCrisis/ollama-mcp-go`, `go 1.24.2` |
| `ollama-mcp-go/.gitignore` | 3 | ignora `ollama-mcp-go`, `*.exe`, `CLAUDE.md` |

Medição: `wc -l *` na pasta da referência (4 `.go` somam 1.426; com README e `go.mod`, 1.615). `git ls-files` lista exatamente 7 arquivos versionados (os 4 `.go`, `README.md`, `go.mod`, `.gitignore`); não há `Makefile`, CI, `LICENSE` em arquivo, nem pasta `cmd/`.

## Dimensionamento

| Item | Valor | Como medido |
| --- | ---: | --- |
| Arquivos Go | 4 | `ls` |
| Linhas Go | 1.426 | `wc -l` |
| Código de produção (`main.go` + `protocol.go`) | 771 | 662 + 109 |
| Código de teste | 655 | 562 + 93 |
| Tools | 4 | `ollama-mcp-go/main.go:172-230` |
| Funções `Test*` | 27 declarações = 26 testes + `TestMain` | `grep -c "^func Test"`: 21 em `main_test.go` (inclui `TestMain`) e 6 em `protocol_test.go` = 27 declarações; 26 aparecem como `--- PASS` (o `TestMain` não é teste) |
| Cobertura | 75,2% | `go test -cover` |
| `tools/list` completo | 1.901 bytes | saída do servidor para `tools/list` (linha JSON inteira, com `_meta`); medido rodando `go run .` com stdin controlado e `OLLAMA_MCP_TRACE=off` |
| Dependências externas | 0 | `ollama-mcp-go/go.mod:1-3` |

## Índice dos documentos

| Arquivo | Conteúdo |
| --- | --- |
| [10-Arquitetura.md](10-Arquitetura.md) | entrypoint, loop stdio, framing, dispatch, camadas, fluxo ponta a ponta de `tools/call`, negociação de versão, notifications |
| [20-Tools_e_Contratos.md](20-Tools_e_Contratos.md) | catálogo das 4 tools, `inputSchema`, saída, erros, consumo do streaming |
| [30-Configuracao_e_Seguranca.md](30-Configuracao_e_Seguranca.md) | env vars, defaults, auth (ausente), TLS (ausente), timeouts, trace file e risco de vazamento, faltas de segurança |
| [40-Codigo_Arquivo_a_Arquivo.md](40-Codigo_Arquivo_a_Arquivo.md) | ficha por arquivo com `caminho:linha` |
| [50-Testes_Build_Release.md](50-Testes_Build_Release.md) | padrão de testes, o que cobre e o que não cobre, build, versão, release |

## O que tirar para o mcpollama

Cada lição tem ID `LIC-OMG-nnn` (convenção do brief); o requisito formal sai na onda B5.

1. **LIC-OMG-001 — Camadas mínimas e sem SDK.** Tipos JSON-RPC e MCP escritos à mão, `dispatch` por `switch` em `Method`, loop de leitura de linhas (`ollama-mcp-go/main.go:16-88`, `ollama-mcp-go/protocol.go:53-80`). Confirma que 1 pacote e 0 dependências bastam para o núcleo; o mcpollama mantém o layout `src/mcp/`, `src/ollama/`, `src/tools/` do molde CSL.
2. **LIC-OMG-002 — Streaming interno com resposta inteira.** Pedir `stream:true` ao Ollama e juntar os tokens em um `strings.Builder`, porque o MCP não entrega parcial (`ollama-mcp-go/main.go:382-421`, `314-318`). Útil para observar geração longa na fila de 4 GPUs; o custo é código extra. Alternativa a avaliar: `stream:false` e sem trace.
3. **LIC-OMG-003 — Contadores do Ollama pegos do chunk `done:true`** (`ollama-mcp-go/main.go:413-415`). O mcpollama precisa exatamente disso, mas o repositório os põe em campos de topo fora do padrão (`prompt_eval_count`, `eval_count`, `ollama-mcp-go/main.go:86-87`); **um cliente MCP pode ignorá-los** (inferência). Para o mcpollama, repetir os contadores também no `text` ou em `_meta`.
4. **LIC-OMG-004 — Padrão de erro de tool:** falha de negócio/rede vira resultado com `isError:true` e texto; só método desconhecido ou `params` inválido vira erro JSON-RPC (`ollama-mcp-go/main.go:600-613`, `ollama-mcp-go/protocol.go:78-79`).
5. **LIC-OMG-005 — Mock do Ollama com `httptest` que fala NDJSON de verdade** (`writeNDJSON`, `mockOllama`, `ollama-mcp-go/main_test.go:83-159`) e chamada de `tools/call` sem processo (`handleToolsCall` direto). Reaproveitar o desenho; ampliar com os casos que faltam (doc 50).
6. **LIC-OMG-006 — Anotações e `title` por tool** (`readOnlyHint`, `destructiveHint`, `openWorldHint`, `ollama-mcp-go/main.go:51-56`, `172-230`). As tools de leitura de arquivo do mcpollama devem marcar `openWorldHint:false` e `readOnlyHint:true`.
7. **LIC-OMG-007 — Cuidado com o trace.** Gravar prompt e resposta em `/tmp/ollama-mcp-trace.log` com permissão `0644`, ligado por padrão (`ollama-mcp-go/main.go:298-309`, `329`), é o oposto do que o mcpollama precisa: ele lerá dados sensíveis e o stdout/disco devem ficar limpos. Se houver trace, deve ser **desligado por padrão** (flag), com permissão `0600`, e sem conteúdo de arquivo.
8. **LIC-OMG-008 — O que NÃO copiar do loop:** JSON inválido é só logado e a linha é descartada, sem resposta `-32700` (`ollama-mcp-go/main.go:644-648`); linha maior que 4 MB derruba o loop (`ollama-mcp-go/main.go:635`, `659`); o loop é sequencial, então uma chamada de até 5 min bloqueia `ping` e cancelamentos (`ollama-mcp-go/main.go:167`, `656`). Reproduzimos o comportamento com stdin controlado (doc 10).
9. **LIC-OMG-009 — O que falta e o mcpollama tem de trazer:** flags, auth Bearer, TLS/CA, modelo padrão, `options`/`num_ctx`, `--tools-verbosity`, tools que leem arquivos, limites de leitura (ver doc 30). O `embed` do repositório devolve só `dimensions` e 16 valores (`ollama-mcp-go/main.go:580-589`), não o vetor.
10. **LIC-OMG-010 — Negociação de versão simples e testada.** `negotiate` devolve a versão pedida se estiver na lista, senão a mais nova da lista (`ollama-mcp-go/protocol.go:97-109`, teste `ollama-mcp-go/protocol_test.go:64-72`). O modo "moderno" (2026-07-28: `server/discover`, `resultType`, `ttlMs`) é implementado só por este repositório e **não foi verificado contra a especificação** nesta onda; tratar como opcional.

## Lacunas (o que esta referência não resolve)

Resumo; o detalhe está nos docs 20, 30 e 50. Autenticação Bearer, TLS, flags, `--version`, `--tools-verbosity`, leitura de arquivo no servidor, chunking, `--debug`, release/CI e testes de erro HTTP/stream não existem aqui.

## Changelog

- **1.0.0 (2026-10-23):** B1. Criado a partir da leitura integral dos 4 arquivos Go, do `README.md`, de `go.mod` e de `.gitignore`, mais execução de `go test`, `go vet` e de um smoke de stdio.
