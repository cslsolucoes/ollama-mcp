# W2 — mcp.so (diretório de servidores MCP)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://mcp.so/servers> |
| **Status da coleta** | <https://mcp.so/servers>: **OK**, só a primeira página. A própria página indica 309 páginas no total; as demais não foram coletadas. |

## O que é

Diretório público de servidores MCP. A página de listagem traz nome, descrição curta e link de cada item. Nesta coleta, só a primeira página foi lida, por isso a lista abaixo não é exaustiva.

## Servidores relacionados a Ollama (primeira página)

| Servidor | Autor (como exibido) | O que faz (conforme a página) | Link | Relevância para o mcpollama |
| --- | --- | --- | --- | --- |
| CrawlForge MCP Server | mysleekdesigns | 28 ferramentas de web scraping, crawling e extração de dados, com saída em Markdown e JSON estruturado. Inclui suporte a LLM local via Ollama. | <https://mcp.so/servers/crawlforge-mcp> | Baixa: é um servidor de scraping que usa o Ollama, não um cliente Ollama para o Claude. |
| OpenZiti LLM Gateway | (não extraído) | proxy para vários provedores de LLM | (não extraído) | Nenhuma: o texto exibido não cita Ollama. |

## Tools expostas

Não aplicável: é um diretório. As tools do CrawlForge não foram documentadas, porque a página do diretório não as detalha.

## Transporte e linguagem

Não informados na listagem.

## Configuração (flags/env, auth, TLS)

Não informados na listagem.

## Ideias aproveitáveis para o mcpollama

1. Usar o diretório como fonte para a próxima rodada: as páginas 2 a 309 podem conter servidores Ollama de verdade. Isso exige coleta paginada, que não foi feita aqui.
2. Pelo que foi lido, nenhum item da primeira página é um cliente Ollama com leitura de arquivo do lado do servidor, que é o alvo do mcpollama. Esse achado é limitado à primeira página.

## Limitações / o que não serve

- Apenas 1 de 309 páginas foi lida. A conclusão "poucos resultados relevantes" vale só para essa amostra.
- A busca por Ollama não foi feita no diretório (o filtro não foi usado).
- Nenhum detalhe técnico (linguagem, transporte, config) foi extraído.

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada na página.

## Fontes

- <https://mcp.so/servers> (coleta 2026-10-23, primeira página de 309)
- <https://mcp.so/servers/crawlforge-mcp> (link citado na página; não aberto)
