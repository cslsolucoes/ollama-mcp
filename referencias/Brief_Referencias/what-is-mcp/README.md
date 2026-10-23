# what-is-mcp: brief da referência 04

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## O que é

A pasta `referencias/04-what-is-mcp/` é o exemplo didático **04** da série `ollama-tlms-golang`. O clone vem de `https://github.com/ollama-tlms-golang/04-what-is-mcp.git` (`04-what-is-mcp/.git/config`, linha de `url`). O `LICENSE` é o Unlicense (domínio público) (`04-what-is-mcp/LICENSE:1`).

O README é uma explicação do protocolo MCP (hosts, clientes, servidores, transportes STDIO e SSE, JSON-RPC 2.0) com demonstrações:

- Claude Desktop usando o servidor SQLite publicado no Docker Hub (`mcp/sqlite`);
- `mcphost` (Go) falando com o Ollama local (`qwen2.5:3b`), também com `mcp/sqlite`.

Essas demonstrações usam um servidor **externo** (Docker Hub), que não está no repositório. O código local é um servidor mínimo com **uma única tool**, `hello_world`, em `mcp-server/main.go` (47 linhas). Por isso o valor para o mcpollama é o **esqueleto mínimo** e não as demonstrações.

## Fontes

Contagens em linhas físicas, medidas com `wc -l` em 2026-10-23 (o arquivo com quebra final sem `\n` aparece com +1 no leitor).

| Arquivo | Linhas (`wc -l`) | Papel |
| --- | --- | --- |
| `04-what-is-mcp/mcp-server/main.go` | 47 | **O único `.go` do repositório** (`find . -name '*.go'`). Servidor MCP completo. CRLF, UTF-8. |
| `04-what-is-mcp/README.md` | 305 | Explicação do MCP e demos. Exibido pelo leitor como 306 linhas. |
| `04-what-is-mcp/mcp-server/go.mod` | 7 | Módulo `hello-mcp-server`, `go 1.23.4`, `mark3labs/mcp-go v0.8.2`. |
| `04-what-is-mcp/mcp-server/go.sum` | 12 | Somas de verificação (mcp-go, uuid, testify, go-spew, yaml.v3). |
| `04-what-is-mcp/go.mod` (raiz) | 3 | Módulo `01-json-output`: resto de outro exemplo, **não** é o servidor. |
| `04-what-is-mcp/go.work` | 5 | Workspace com `use ( mcp-server )`. |
| `04-what-is-mcp/go.work.sum` | 120 | Somas do workspace. Não contém entrada de `mark3labs` (`grep -c` = 0). |
| `04-what-is-mcp/mcp-server/compose.yml` | 1 | Conteúdo `# in progress`. Vazio de fato. |
| `04-what-is-mcp/mcp-server/.gitignore` | 2 | Ignora `hello-mcp-server`. |
| `04-what-is-mcp/.gitignore` | 4 | Ignora `README.fr.md`, `tmp`, `mcp-gitlab`, `mcphost`. |
| `04-what-is-mcp/.devcontainer/devcontainer.json` | 39 | Ambiente de desenvolvimento (VS Code). Ver `10-Arquitetura.md`. |
| `04-what-is-mcp/.devcontainer/compose.yml` | 13 | Serviço `ai-workspace`. |
| `04-what-is-mcp/.devcontainer/Dockerfile` | 92 | Ubuntu 22.04, Go, ferramentas, usuário. |
| `04-what-is-mcp/.vscode/settings.json` | 52 | Preferências do editor (JSONC). Sem relevância para o MCP. |
| `04-what-is-mcp/.vscode/extensions.json` | 12 | Recomendações de extensões. |
| `04-what-is-mcp/git.sh` | 111 | Script de commit com gitmoji. Não relacionado ao MCP. |
| `04-what-is-mcp/LICENSE` | (Unlicense) | Domínio público. |
| `04-what-is-mcp/imgs/*.png` | 7 imagens | `claude-ai-00/01.png`, `mcphost-01..05.png` (0,5 a 1,1 MB cada). **Não inspecionadas visualmente nesta onda** (ver pendências). |

Arquivos ignorados por regra de segredos: nenhum `.env`, `.pem`, `.key` ou `.mcp.json` no repositório (`find` completo). Nenhum valor de credencial foi encontrado nas fontes lidas.

## Índice

1. `README.md` (este arquivo): o que é, fontes, índice e o que levar para o mcpollama.
2. [`10-Arquitetura.md`](10-Arquitetura.md): SDK (`mark3labs/mcp-go` v0.8.2), transporte STDIO, registro da tool, fluxo de uma chamada, `.devcontainer`.
3. [`20-Tools_e_Contratos.md`](20-Tools_e_Contratos.md): a tool `hello_world` (nome, descrição, parâmetro, saída, erro).

## O que tirar para o mcpollama

- **Stdout só para JSON-RPC.** O exemplo imprime com `fmt` no stdout antes e depois de `server.ServeStdio` (`04-what-is-mcp/mcp-server/main.go:32`, `:35`, `:37`). No transporte STDIO isso mistura texto não JSON no canal do protocolo (inferência: clientes podem rejeitar a linha). O `CLAUDE.md` do mcpollama exige log só em stderr. Não copiar este padrão.
- **Esqueleto mínimo com SDK.** `NewMCPServer` → `NewTool` → `AddTool` → `ServeStdio` (`main.go:13-16`, `:21-30`, `:34`). Serve de comparação com a decisão do mcpollama de escrever `mcp/types.go` à mão, sem SDK externo. Essa escolha fica para a onda de arquitetura, com o `10-Arquitetura.md` como insumo.
- **Validar o tipo no handler e devolver erro como resultado.** O parâmetro é declarado `Required()` (`main.go:23-26`), mas o handler refaz a checagem com `type assertion` e devolve `NewToolResultError` (`main.go:42-45`), sem erro de protocolo. Padrão a copiar nas tools do Ollama (erro de domínio como resultado).
- **Descrição é custo de token.** A descrição da tool e de cada parâmetro (`main.go:22`, `:25`) entra no `tools/list`. O mcpollama já prevê `--tools-verbosity compact` por esse motivo; este exemplo é o mínimo possível.
- **Não copiar a higiene do repositório.** `go.mod` da raiz com módulo de outro exemplo (`04-what-is-mcp/go.mod:1`), `compose.yml` vazio (`mcp-server/compose.yml:1`), README com resíduo de conversa no fim (`README.md:306`) e `[!Claude Desktop](...)` no lugar da sintaxe de imagem (`README.md:231`, `:235`).

## Pendências

1. As 7 imagens em `imgs/` não foram inspecionadas visualmente. São capturas das demos (Claude Desktop e `mcphost`) e não alteram o achado técnico. Se o owner quiser, uma leitura pontual fecha a lacuna.
2. O código-fonte do `mark3labs/mcp-go` v0.8.2 não está na pasta e não foi baixado nesta onda. Os detalhes internos do SDK em `10-Arquitetura.md` são inferências marcadas como tais.
3. O README cita a série no blog do autor (`k33g.hashnode.dev`, `README.md:5`). Não foi coletado nesta onda, portanto não há ficha web com data.
