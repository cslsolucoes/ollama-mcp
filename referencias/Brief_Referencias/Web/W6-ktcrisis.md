# W6 — KTCrisis (ollama-mcp-go e flux7-*)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://github.com/KTCrisis>; <https://raw.githubusercontent.com/KTCrisis/ollama-mcp-go/main/README.md>; <https://raw.githubusercontent.com/KTCrisis/flux7-mesh/main/README.md>; <https://raw.githubusercontent.com/KTCrisis/flux7-memory/main/README.md>; <https://raw.githubusercontent.com/KTCrisis/flux7-supervisor/main/README.md> |
| **Status da coleta** | github.com/KTCrisis: **OK** (5 repositórios fixados visíveis; a página indica 28 no total, e só os fixados foram vistos). ollama-mcp-go README: **OK**. flux7-mesh: **OK**. flux7-memory: **OK**. flux7-supervisor: **OK**. flux7-console e keys7: não lidos. |

## O que é

- **ollama-mcp-go** (Go): servidor MCP para Ollama, distribuído como binário único, para Claude Code e Cursor. É a referência mais próxima do mcpollama.
- **flux7-mesh** (Go): proxy de governança entre agentes de IA e suas ferramentas. Aplica política, aprovação humana e trace assinado em cada chamada.
- **flux7-memory** (Go): memória compartilhada entre agentes, com busca híbrida (BM25 e vetorial) e reranking por LLM opcional.
- **flux7-supervisor** (linguagem não declarada; inferido Python pelos arquivos `src/sup7/*.py` e `pip`): agente L1 que resolve aprovações pendentes do flux7-mesh com regras e um LLM plugável.

## Tools expostas

### ollama-mcp-go (Go, stdio)

| Tool | Parâmetros | Saída |
| --- | --- | --- |
| `list_models` | não detalhados | lista dos modelos do Ollama |
| `generate` | `model`, `prompt`, `system` (opcional) | resposta completa, com `prompt_eval_count` e `eval_count` |
| `chat` | `model`, `messages` (cada item com `role` e `content`) | resposta completa, com os mesmos contadores |
| `embed` | `model`, `text` | embeddings (formato não descrito) |

Cada tool tem `title` e anotações (somente leitura, não destrutiva, mundo fechado, idempotência em `list_models` e `embed`).

### flux7-memory (Go)

`memory_store`, `memory_recall`, `memory_search`, `memory_context`, `memory_get` (lê um trecho de arquivo do workspace; caminhos fora dele são recusados), `memory_list`, `memory_forget`, `memory_history`. Saídas parcialmente documentadas (`memory_context` devolve um array JSON de `{key, value, tags, agent, updated}`).

### flux7-mesh e flux7-supervisor

Não são servidores de tools de modelo. O mesh é um proxy que intermedeia tools de terceiros. O supervisor é um agente de avaliação que faz poll de aprovações pendentes e devolve approve/deny com justificativa.

## Transporte e linguagem

- ollama-mcp-go: **stdio**, JSON-RPC 2.0, Go 1.22+ para compilar.
- flux7-mesh: Go 1.24+; agentes conectam por MCP stdio ou HTTP; upstream pode ser MCP stdio, API REST (OpenAPI) ou CLI; traces em JSONL ou OTLP.
- flux7-memory: Go 1.25+, sem cgo; stdio (padrão), HTTP JSON-RPC (`POST /rpc`), MCP Streamable HTTP (`POST /mcp`) e HTTP+SSE legado.
- flux7-supervisor: `pip install`, `pytest`; LLM plugável (`ollama`, `anthropic`, `claude-code` ainda não funcional, `jev`, `none`).

## Configuração (flags/env, auth, TLS)

### ollama-mcp-go

- `OLLAMA_HOST`: URL do Ollama. Padrão `http://localhost:11434`.
- `OLLAMA_MCP_TRACE`: caminho do arquivo de trace. Padrão `/tmp/ollama-mcp-trace.log`. `off` desativa.
- Sem flags de linha de comando.
- Negociação de `protocolVersion`: aceita clientes modernos (2026-07-28, com `server/discover` e `_meta`) e legados (2025-11-25, 2025-06-18, 2025-03-26, 2024-11-05). Versão desconhecida gera `UnsupportedProtocolVersionError` (-32022).
- Auth e TLS: não tratados.

### flux7-memory

- Variáveis: `MEM7_DIR` (padrão `~/.mem7`), `MEM7_LISTEN` (`:9070`), `MEM7_TOKEN` (token bearer para rotas protegidas, vazio por padrão), `MEM7_SCOPES` (arquivo JSON de escopos de leitura por agente), `MEM7_CHAIN_KEY` (chave HMAC da cadeia), `MEM7_CHAIN_KEYS_RETIRED`, `MEM7_MAX_ENTRIES` (10000), `MEM7_EMBED_URL` (ativa busca híbrida), `MEM7_EMBED_MODEL` (`nomic-embed-text`), `MEM7_EMBED_PROVIDER` (`ollama` ou `openai`), `MEM7_EMBED_KEY`, `MEM7_RERANK_URL`, `MEM7_RERANK_MODEL` (`gemma4:e4b`).
- Flags de `mem7 serve`: `--listen`, `--token`, `--scopes`.
- Valores de segredo não foram lidos nem copiados; só os nomes.

### flux7-mesh

- Flags: `--mcp`, `--config config.yaml`; `discover --config --generate-policy`; instalador com `--service` e `--system`.
- Auth: JWT emitido pelo IdP, incluindo o usuário humano representado pelo agente. TLS: não detalhado.

### flux7-supervisor

- YAML `sup7.yaml` (seções `mesh`, `memory`, `evaluator`, `poll`, `rules`, `project_dirs`, `admin`, `bench`). Flag `-c sup7.yaml`; subcomandos `start`, `status`, `bench replay`.
- Depende de uma instância de `mesh7 serve`.

## Ideias aproveitáveis para o mcpollama

1. **Raciocínio separado do que volta ao cliente.** ollama-mcp-go envia o bloco de raciocínio do modelo para a seção "--- pense ---" do arquivo de trace e nunca o devolve ao cliente MCP. Isso poupa tokens do Claude sem perder a informação para depuração. Encaixa no `--debug` (stderr) previsto no mcpollama.
2. **Trace em arquivo que espelha o stream.** O trace grava cabeçalho, prompt, resposta em tempo real e rodapé com contagem de tokens, e se desativa sozinho se o arquivo não abrir (a geração continua). É um bom modelo para a observabilidade do mcpollama, que não deve gravar em disco sem flag. Ressalva: o default `/tmp/...` não serve ao Windows; o mcpollama precisaria de caminho explícito (inferência).
3. **Token bearer e escopo de leitura por agente (flux7-memory).** `MEM7_TOKEN` e `MEM7_SCOPES` mostram um padrão de Bearer simples mais restrição de leitura por identidade. É relevante para a superfície de leitura de arquivos do mcpollama (`FILE`/`SEC`). Não copiar o código; só o desenho.

## Limitações / o que não serve

- ollama-mcp-go: sem flags, sem testes descritos no README, saída de `embed` não documentada, sem auth nem TLS.
- O default de trace em `/tmp` depende do sistema operacional.
- flux7-mesh, flux7-memory e flux7-supervisor são de governança e memória, fora do escopo de "tools de modelo". Servem de referência de desenho, não de código a reaproveitar.
- flux7-supervisor: o próprio README avisa que `claude-code` ainda não funciona, e que `contains` pode aprovar por engano ferramentas de escrita.
- flux7-console e keys7 não foram lidos; keys7 é um nó MIDI sem relação com o tema.

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada nos READMEs lidos.

## Fontes

- <https://github.com/KTCrisis> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/KTCrisis/ollama-mcp-go/main/README.md> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/KTCrisis/flux7-mesh/main/README.md> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/KTCrisis/flux7-memory/main/README.md> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/KTCrisis/flux7-supervisor/main/README.md> (coleta 2026-10-23)
