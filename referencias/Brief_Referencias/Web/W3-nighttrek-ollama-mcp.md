# W3 — NightTrek/Ollama-mcp

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://mcpservers.org/pt-BR/servers/NightTrek/Ollama-mcp>; <https://github.com/NightTrek/Ollama-mcp>; <https://raw.githubusercontent.com/NightTrek/Ollama-mcp/main/README.md>; <https://raw.githubusercontent.com/NightTrek/Ollama-mcp/master/README.md> |
| **Status da coleta** | mcpservers.org: **HTTP 403** (não carregou; nada foi descrito a partir dessa página). github.com/NightTrek/Ollama-mcp: **OK** (a seção About não tem descrição). README em `main`: **OK**. README em `master`: **OK** (conteúdo equivalente). |

## O que é

Servidor MCP que liga o Ollama ao Model Context Protocol, para usar modelos locais em clientes compatíveis com MCP (README). A página do repositório não tem descrição na seção About. Nos prerequisitos, o README cita Node.js e npm/pnpm, e a lista de arquivos inclui `package.json` e `tsconfig.json`. Por isso a linguagem é **Node.js/TypeScript**, mas essa leitura é inferência: a página não declara a linguagem.

## Tools expostas

Fonte: exemplos de uso do README. Nenhuma tool tem saída documentada.

| Tool | Parâmetros | Saída |
| --- | --- | --- |
| `pull` | `name` (baixa um modelo) | não documentada |
| `run` | `name`, `prompt` (executa um modelo) | não documentada |
| `chat_completion` | `model`, `messages` (objetos com `role` e `content`), `temperature`; descrito como compatível com a API de chat da OpenAI | não documentada |
| `create` | `name`, `modelfile` (caminho para um Modelfile) | não documentada |

O README também cita, sem nomes de tool, capacidades de push, listagem, cópia, remoção de modelos, visualização de detalhes e controle do servidor Ollama.

## Transporte e linguagem

- Transporte: **não declarado** no README. O exemplo do Claude Desktop usa `command: "node"` com `args` apontando para `build/index.js`, o que sugere stdio (inferência, não verificado).
- Linguagem: Node.js/TypeScript (inferência, ver acima).

## Configuração (flags/env, auth, TLS)

- Build: `pnpm install` e depois `pnpm run build`.
- Variável de ambiente: `OLLAMA_HOST`, opcional, padrão `http://127.0.0.1:11434`.
- Sem flags de linha de comando documentadas.
- Timeout de execução de modelos: 60 s. Faixa de temperatura: 0 a 2.
- Auth e TLS: não documentados.
- Caminho do Claude Desktop: `%APPDATA%/Claude/claude_desktop_config.json` (Windows), citado no README.
- Licença: MIT.

## Ideias aproveitáveis para o mcpollama

1. **Lista enxuta de tools por modelo.** `chat_completion` com `messages` e `temperature` é o mesmo formato de chat que o mcpollama vai precisar. Serve como referência de nomes e de argumentos, sem o restante.
2. **Timeout explícito de 60 s.** Um valor documentado para a execução de modelos é útil para calibrar o `PERF` do mcpollama, em que a GPU é uma fila única.

## Limitações / o que não serve

- `pull` e `create` são operações administrativas (baixar modelo, criar a partir de Modelfile). O mcpollama proíbe essas tools: o nginx responde 403 em `/api/pull|push|create|delete|copy|blobs`. Não devem ser copiadas.
- Não há tool que leia arquivo do lado do servidor, que é o princípio central do mcpollama.
- Saídas não documentadas; sem auth nem TLS.
- O README lista `push`, `copy` e `delete` como capacidades, o que amplia a superfície além do que o mcpollama aceita.

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada na página nem no README.

## Fontes

- <https://mcpservers.org/pt-BR/servers/NightTrek/Ollama-mcp> (coleta 2026-10-23; HTTP 403)
- <https://github.com/NightTrek/Ollama-mcp> (coleta 2026-10-23; OK)
- <https://raw.githubusercontent.com/NightTrek/Ollama-mcp/main/README.md> (coleta 2026-10-23; OK)
- <https://raw.githubusercontent.com/NightTrek/Ollama-mcp/master/README.md> (coleta 2026-10-23; OK)
