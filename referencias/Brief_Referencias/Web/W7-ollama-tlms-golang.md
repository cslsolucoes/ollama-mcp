# W7 — ollama-tlms-golang (repositórios 05, 06 e 07)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://github.com/ollama-tlms-golang>; <https://github.com/ollama-tlms-golang/05-make-your-mcp-server>; <https://github.com/ollama-tlms-golang/07-make-your-mcp-server-> (página do repositório); <https://raw.githubusercontent.com/ollama-tlms-golang/05-make-your-mcp-server/main/README.md>; <https://raw.githubusercontent.com/ollama-tlms-golang/05-make-your-mcp-server/main/main.go>; <https://raw.githubusercontent.com/ollama-tlms-golang/06-mcp-host-genaiapp/main/README.md>; <https://raw.githubusercontent.com/ollama-tlms-golang/07-make-your-mcp-server-/main/README.md> |
| **Status da coleta** | Organização: **OK** (a página indica 14 repositórios; o extrator listou 12 nomes, sem descrições, e não confirmou a contagem). **05**: página do repositório **OK**; `main.go` **OK**; README **OK**, mas o arquivo em `README.md` é um post de blog, não um README. **06**: README **OK** (o arquivo é um post de blog). **07**: README **parcial**, só o título ("Creating an MCP Server in Go and Serving it with Docker (part 2)"); a página do repositório também não trouxe o corpo. **main.go de 06 e 07**: não carregados. |

## O que é

Repositórios de apoio a uma série de posts do autor k33g sobre MCP em Go com Ollama. Cada pasta é um exemplo didático:

- **05-make-your-mcp-server**: servidor MCP em Go com uma tool que chama `curl`. Licença MIT; 11 estrelas (página do repositório).
- **06-mcp-host-genaiapp**: cliente MCP em Go que se conecta ao servidor da pasta 05 via Docker e usa o Ollama para escolher a tool e analisar o código buscado.
- **07-make-your-mcp-server-**: título sugere a parte 2 da série sobre servidor MCP com Docker; conteúdo não coletado.

## Tools expostas

**05** (`main.go`, único arquivo lido em código):

- Nome: `use_curl`. Descrição: "fetch this webpage".
- Parâmetro: `url` (string, obrigatório), "url of the webpage to fetch".
- Saída: texto do `curl -s <url>`. Se `url` não for string, erro "url must be a string". Se o `curl` falhar, erro com a mensagem da exceção.

**06**: não é servidor; consome a tool `use_curl`.

**07**: não extraída.

## Transporte e linguagem

- Linguagem: Go (`go 1.23.4`), com a biblioteca `github.com/mark3labs/mcp-go` v0.8.2 (05 e 06).
- Transporte: **stdio** (05, via `server.ServeStdio(s)`; 06, via `client.NewStdioMCPClient`).

## Configuração (flags/env, auth, TLS)

**05**: nenhuma flag e nenhuma variável de ambiente lida. Único valor fixo: `curl -s`. Nome do servidor `mcp-curl`, versão `1.0.0`. Dockerfile em dois estágios (`golang:1.23.4-alpine` para build; `curlimages/curl:8.6.0` na imagem final). `mcp.json` roda `docker run --rm -i mcp-curl`. Execução com o `mcphost` e o modelo `ollama:qwen2.5-coder:14b`.

**06** (README, texto de apoio):

- `OLLAMA_HOST`: padrão `http://localhost:11434`.
- `TOOLS_LLM`: modelo para seleção de tools (padrão `qwen2.5:0.5b`, que precisa suportar tools).
- `CHAT_LLM`: modelo para análise de código (padrão `qwen2.5-coder:3b`).
- Timeout de contexto: 30 s.
- Execução: `go run main.go`.

Auth e TLS: não tratados em nenhuma das pastas lidas.

## Ideias aproveitáveis para o mcpollama

1. **Ponte de dois modelos com seleção por LLM pequeno.** A pasta 06 usa um modelo pequeno (`qwen2.5:0.5b`) só para escolher a tool e outro maior para analisar o resultado. No mcpollama, o modelo residente é um só (`qwen3-ia`), então isso não se aplica diretamente. Vale como padrão de roteamento para uma eventual tool "roteadora".
2. **Parâmetro único e saída de texto simples.** O contrato da `use_curl` (um parâmetro, saída em texto, erro explícito para tipo errado) é o formato mínimo de tool que o mcpollama deve manter. Ponto de atenção: a tool busca conteúdo com `curl` sem limite de tamanho, o que pode estourar o contexto de 32k; o mcpollama precisa de limite de bytes e de chunking. [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]
3. **Empacotamento com Docker de dois estágios.** A imagem final mínima é um modelo para o build do mcpollama em Windows e Linux (`GOOS`), se o empacotamento em container for necessário. Ressalva: o mcpollama distribui binário, não imagem (inferência sobre o alvo).

## Limitações / o que não serve

- A `use_curl` faz um GET arbitrário sem allowlist, sem limite de tamanho e sem TLS próprio. É o oposto do que o mcpollama precisa em `SEC` e `FILE`. Não copiar o desenho de segurança.
- Usa `curl` do sistema via `exec`, o que depende de um binário externo e não é portátil no Windows sem configuração.
- 06 e 07: sem código-fonte lido; o que se sabe vem de posts de blog.
- Sem testes descritos. Sem auth.

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada nas páginas lidas.

## Fontes

- <https://github.com/ollama-tlms-golang> (coleta 2026-10-23; 12 nomes extraídos de 14 indicados)
- <https://github.com/ollama-tlms-golang/05-make-your-mcp-server> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/ollama-tlms-golang/05-make-your-mcp-server/main/main.go> (coleta 2026-10-23)
- <https://raw.githubusercontent.com/ollama-tlms-golang/05-make-your-mcp-server/main/README.md> (coleta 2026-10-23; post de blog)
- <https://raw.githubusercontent.com/ollama-tlms-golang/06-mcp-host-genaiapp/main/README.md> (coleta 2026-10-23; post de blog)
- <https://raw.githubusercontent.com/ollama-tlms-golang/07-make-your-mcp-server-/main/README.md> (coleta 2026-10-23; só título)
