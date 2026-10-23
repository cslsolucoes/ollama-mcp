# W5 — Dolphin MCP com Ollama (artigo da Apidog)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://apidog.com/pt/blog/mcp-ollama/> |
| **Status da coleta** | **OK** |

## O que é

Artigo em português sobre o Dolphin MCP, um cliente de linha de comando e biblioteca Python que conversa com um modelo e chama servidores MCP. O Ollama aparece como **um dos provedores de modelo** suportados. O artigo **não mostra o Ollama como cliente MCP**, que era a pergunta da onda B3.

## Tools expostas

Não há servidor MCP nomeado para o Ollama. O único recurso concreto é um banco SQLite de demonstração criado por `setup_db.py`, com dados de exemplo sobre espécies de golfinhos.

## Transporte e linguagem

- Linguagem: Python (`pip install dolphin-mcp`, script `asyncio`).
- Transporte: não descrito. Os servidores são configurados por `command` e `args` em `mcp_config.json`, o que indica stdio (inferência).

## Configuração (flags/env, auth, TLS)

- CLI: `dolphin-mcp-cli --model <nome> "<pergunta>"`. Exemplo com Ollama: `dolphin-mcp-cli --model ollama "Defina física quântica"`. O artigo não diz se `ollama` é um nome de modelo válido ou um alias.
- Outras opções: `--quiet` (oculta a saída intermediária) e `--config <arquivo>`.
- API Python: `run_interaction(user_query=..., model_name=..., quiet_mode=...)`.
- `.env`: mostrado só para OpenAI (`OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_ENDPOINT` opcional). Sem variáveis de Ollama.
- `mcp_config.json`: bloco `mcpServers`, cada entrada com `command`, `args` e `env` opcional.
- Ollama: não há passos de instalação, de download de modelo, nem de endpoint ou porta.
- Auth e TLS: não tratados.

## Ideias aproveitáveis para o mcpollama

1. **Padrão `mcpServers` com `command`/`args`/`env`.** É o formato de configuração que os clientes MCP (Claude Code, Cursor) já usam. Vale para o exemplo de `.mcp.json` do README do mcpollama, sem segredo no arquivo.
2. **`--quiet` para suprimir o trecho intermediário.** Reduz o texto devolvido ao chamador. Ideia alinhada à economia de tokens, mas aqui é uma flag de CLI, não de servidor.

## Limitações / o que não serve

- O artigo não ensina o Ollama a atuar como cliente MCP. Não há uso do Ollama como cliente com servidores MCP do lado do Ollama. A premissa da onda B3 não se confirma neste artigo.
- Não traz configuração de Ollama (endpoint, modelo, porta).
- Não trata de auth, TLS nem de arquivos locais.
- É um guia introdutório de um produto de terceiros (Dolphin MCP).

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada no artigo.

## Fontes

- <https://apidog.com/pt/blog/mcp-ollama/> (coleta 2026-10-23; OK)
