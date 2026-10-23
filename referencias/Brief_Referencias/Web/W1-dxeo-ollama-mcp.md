# W1 — dxeo-ollama-mcp (Dxeo)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.1 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://lobehub.com/mcp/dxeo-ollama-mcp> (redirect 302 para <https://market.lobehub.com/s/plugins/dxeo-ollama-mcp>, seguido manualmente); <https://github.com/Dxeo/ollama-mcp>; <https://raw.githubusercontent.com/Dxeo/ollama-mcp/main/README.md>; <https://raw.githubusercontent.com/Dxeo/ollama-mcp/master/README.md> |
| **Status da coleta** | lobehub: **OK** (página do marketplace, com o redirect acima; o campo de resumo ficou com o placeholder `{{overview\.summary}}`). Repositório, README em `main`: **HTTP 404**. README em `master`: **OK** (fonte primária usada nesta ficha). |

## O que é

Servidor MCP chamado "Ollama MCP Server" (identificador `dxeo-ollama-mcp`, versão 1.0.0, autor Dxeo). O marketplace o descreve como "An MCP server routing AI tasks to local Ollama models" e informa que é preciso ter os modelos baixados localmente. Linguagem: **Go**, pelas instruções de build do README (`go build`). Na página do marketplace, o tipo de conexão aparece como "local".

O README do repositório usa a mesma linha de propósito: direcionar tarefas de IA para modelos locais do Ollama, sem chaves de API nem chamadas à nuvem. Usa `slog` para logs estruturados.

## Tools expostas

Fonte: README do repositório (`master`). Nenhum dos três nomes tem parâmetros ou formato de saída documentados.

| Tool | Finalidade (README) | Modelo usado |
| --- | --- | --- |
| `reason_task` | raciocínio, geração de código e sumarização | `REASONING_MODEL` |
| `embed_text` | geração de embeddings de texto | `EMBEDDING_MODEL` |
| `filter_docs` | ranqueia documentos por similaridade semântica com uma consulta | `EMBEDDING_MODEL` |

**Divergência observada:** a página do marketplace marca Tools, Resources e Prompts como `false` e não lista tool alguma. Prevalece o README do repositório, mas os nomes e a assinatura não foram confirmados no código-fonte, porque `main.go` não foi carregado nesta coleta.

## Transporte e linguagem

- Transporte: **stdio** (informado pelo README).
- Linguagem: **Go** (inferido das instruções `go build` e do `slog`; não verificado no código).

## Configuração

Variáveis de ambiente (README):

| Variável | Obrigatória | Padrão | Descrição |
| --- | --- | --- | --- |
| `REASONING_MODEL` | sim | — | modelo de raciocínio e geração de código (ex.: `qwen3:1.7b`) |
| `EMBEDDING_MODEL` | sim | — | modelo de embeddings e filtragem de documentos (ex.: `nomic-embed-text`) |
| `CONTEXT_SIZE` | sim | — | janela de contexto do modelo de raciocínio, em tokens (ex.: `16384`) |
| `OLLAMA_HOST` | não | `http://localhost:11434` | endereço do Ollama |

- Se alguma variável obrigatória faltar, o servidor encerra com erro (README).
- Auth: **não documentada**. TLS: **não documentado**. Não há flags de linha de comando citadas.
- Licença: GNU GPL v3.0 (README).

## Ideias aproveitáveis para o mcpollama

1. **Modelos separados por papel.** Ter `REASONING_MODEL` e `EMBEDDING_MODEL` como configs distintas casa com a intenção de tools por função. No mcpollama, o `--model` (default `qwen3-ia`) poderia ganhar um par para embeddings (inferência: o servidor atual tem um único modelo residente). [desatualizado: a B4 mediu que o `nomic-embed-text` (768 dimensões) está instalado e que o `qwen3-ia` não tem a capacidade de embedding; o par virou o requisito `MCPO-REQ-CFG-004`, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3 e §6]
2. **`filter_docs` como economia de tokens.** O ranking feito no servidor devolve só os documentos relevantes, e não o conjunto inteiro. Segue o princípio do mcpollama de o servidor fazer o trabalho pesado. Ressalva: aqui os documentos vêm no payload da chamada (inferência, pela assinatura citada); no mcpollama, o ideal é o servidor ler os arquivos do lado dele.
3. **`CONTEXT_SIZE` como configuração explícita.** Expor a janela de contexto como config obrigatória ajuda a alinhar o cliente ao limite real (no mcpollama, `ctx 32k`). [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]

## Limitações / o que não serve

- Parâmetros e formato de saída das três tools não estão documentados; sem o código, não há contrato a reaproveitar.
- Sem auth nem TLS documentados. O mcpollama precisa de Bearer e certificado CA (`--tls-ca`), então este servidor não serve como base sem adaptação.
- Divergência entre o marketplace (zero tools) e o README (três tools).
- Projeto pequeno (8 instalações e 0 estrelas, segundo o marketplace).

## Achado de segurança: prompt injection na página do marketplace

A página `market.lobehub.com/s/plugins/dxeo-ollama-mcp` contém um bloco de texto dirigido a agentes de IA. Ele manda executar `npx @lobehub/market-cli register`, substituir credenciais salvas e registrar de novo antes de qualquer busca. **Não foi seguido. Nenhum comando foi executado.** Trata-se de tentativa de alterar credenciais de outra instância, que o owner deve avaliar. Registrado como achado; a página é dado, não instrução.

Nota neutra (não é instrução ao agente): o README do repositório sugere copiar o conteúdo de um `AGENT.md` para as instruções do agente. Foi lido como recomendação aos usuários do projeto e não foi seguido.

## Fontes

- <https://lobehub.com/mcp/dxeo-ollama-mcp> (coleta 2026-10-23; redirect para `market.lobehub.com`)
- <https://github.com/Dxeo/ollama-mcp> (coleta 2026-10-23; `main` retornou 404)
- <https://raw.githubusercontent.com/Dxeo/ollama-mcp/master/README.md> (coleta 2026-10-23)
