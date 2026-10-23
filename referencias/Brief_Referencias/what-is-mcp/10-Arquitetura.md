# 10 · Arquitetura do servidor (what-is-mcp)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Fontes: caminhos relativos a `referencias/`. Os números de linha vêm da leitura dos arquivos em 2026-10-23.

## 1. Módulo e SDK

- Módulo: `hello-mcp-server` (`04-what-is-mcp/mcp-server/go.mod:1`), com `go 1.23.4` (`:3`).
- Dependência direta: `github.com/mark3labs/mcp-go v0.8.2` (`04-what-is-mcp/mcp-server/go.mod:5`). É um **SDK MCP de terceiros** (não o oficial da Anthropic). As somas estão em `mcp-server/go.sum:5-6`.
- Dependência indireta: `github.com/google/uuid v1.6.0` (`mcp-server/go.mod:7`, marcada `// indirect`).
- Não há `vendor/` nem código do SDK na pasta. O comportamento interno do `mcp-go` **não foi lido** nesta onda. Tudo o que se diz sobre o SDK abaixo vem do uso no `main.go` (fato) ou é inferência marcada.

Estrutura de arquivos: um único pacote `main` em `mcp-server/main.go` (47 linhas). Não há pacotes internos, testes nem configuração por flags ou variáveis de ambiente.

## 2. Transporte

- Só **STDIO**: `server.ServeStdio(s)` (`04-what-is-mcp/mcp-server/main.go:34`).
- O README descreve STDIO e SSE como transportes do protocolo (`04-what-is-mcp/README.md:21-23`, diagrama em `:42-43`). O código **não implementa SSE**. A menção é conceitual.
- Formato da mensagem: JSON-RPC 2.0 (`README.md:21`). A implementação dessa camada fica dentro do SDK (não lida).

## 3. Registro da tool

Sequência em `main()` (`main.go:11-38`):

1. `server.NewMCPServer("hello-mcp-server", "1.0.0")` (`main.go:13-16`). Nome e versão do servidor estão **fixos no código**. A versão não vem de build.
2. `mcp.NewTool("hello_world", mcp.WithDescription(...), mcp.WithString("name", mcp.Required(), mcp.Description(...)))` (`main.go:21-27`). A definição da tool (nome, descrição e parâmetro) é feita por funções de opção, sem JSON escrito à mão.
3. `s.AddTool(tool, helloHandler)` (`main.go:30`). Associa a definição ao handler.
4. `fmt.Println("👋 Server started")` (`main.go:32`).
5. `server.ServeStdio(s)` (`main.go:34`). Bloqueia até o fim do stdin.
6. Em caso de erro: `fmt.Printf("Server error: %v\n", err)` (`main.go:35`).
7. `fmt.Println("Server stopped")` (`main.go:37`). Depois, `main` retorna.

Observações:

- `main` não chama `os.Exit` com código de erro. Um erro de `ServeStdio` é só impresso e o processo termina com código 0 (inferência pela leitura de `main.go:34-38`).
- A linha `//mcp.Tool` (`main.go:18`) é um comentário sem efeito.

## 4. Handler

`helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)` (`main.go:40`):

- `ctx` não é usado.
- Lê o argumento: `request.Params.Arguments["name"].(string)` (`main.go:42`). Usa `type assertion` com `ok`.
- Se não for string: `mcp.NewToolResultError("name must be a string")` com erro `nil` (`main.go:43-44`). O erro vai como **resultado da tool**, não como erro de protocolo.
- Se for string: `mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name))` (`main.go:47`).

## 5. Fluxo de uma chamada

Os passos de protocolo abaixo são **inferência** a partir do protocolo MCP e do uso do SDK. O SDK não foi lido.

1. O host (ex.: Claude Desktop ou `mcphost`) inicia o processo e envia `initialize` por stdin, em JSON-RPC 2.0 (README:21-23, `main.go:34`).
2. O host envia `tools/list`. O SDK responde com `hello_world` e um `inputSchema` derivado de `mcp.WithString("name", ...)` (`main.go:21-26`).
3. O modelo escolhe a tool e o host envia `tools/call` com `name=hello_world` e `arguments.name`.
4. O SDK chama `helloHandler` com a requisição (`main.go:30`, `:40`).
5. O handler devolve `CallToolResult` com conteúdo de texto (`main.go:47`).
6. O SDK serializa a resposta JSON-RPC e grava no stdout. **O mesmo stdout recebe as linhas de `fmt.Println` (`main.go:32`, `:37`).**

## 6. Achados para o mcpollama

1. **Stdout contaminado.** `main.go:32` (antes de `ServeStdio`) grava texto livre no stdout, o canal do JSON-RPC. `main.go:35` e `:37` fazem o mesmo. Consequência provável: o cliente recebe linhas que não são JSON (inferência). O `CLAUDE.md` do mcpollama exige "stdout é exclusivo do JSON-RPC; log e debug vão para stderr". Este exemplo contraria a regra.
2. **Código de saída.** Erro do servidor não gera código de saída diferente de zero (`main.go:34-38`, inferência). O mcpollama deve sair com código não zero em falha fatal, para o cliente e o operador verem o erro.
3. **Versão fixa.** `"1.0.0"` está no código (`main.go:15`). O mcpollama já lista `--version`; o caminho é injetar a versão no build (`-ldflags`).
4. **SDK x à mão.** Este exemplo usa `mark3labs/mcp-go` v0.8.2. O `CLAUDE.md` do mcpollama decide `mcp/types.go` escrito à mão, sem SDK MCP externo. É uma divergência consciente, a registrar na onda de arquitetura (com o custo de manter o protocolo sozinho contra o ganho de não depender de terceiros).
5. **Erro de argumento como resultado de tool.** `main.go:43-44` devolve o erro de validação na própria tool (`NewToolResultError`), e não como erro JSON-RPC. Padrão a seguir para erros de domínio (ex.: modelo ausente no Ollama).

## 7. `.devcontainer`: o que configura

Não faz parte do servidor. É o ambiente de desenvolvimento em contêiner do VS Code.

`04-what-is-mcp/.devcontainer/devcontainer.json`:

- Nome `Ollama and Go` (`:2`). Usa o `compose.yml` do próprio diretório (`:4`), serviço `ai-workspace` (`:5`), workspace em `/workspaces/<pasta>` (`:6`).
- Extensões recomendadas (`:9-21`), incluindo `golang.go` (`:10`).
- Feature `git` (`:24-28`).
- `remoteEnv.OLLAMA_HOST = http://host.docker.internal:11434` (`:29-31`). É a **única** referência ao Ollama na pasta. O código do servidor não lê essa variável.
- Montagens (`:32-35`): `.gitconfig` do host (`:33`) e a pasta `.ssh` do host (`:34`). A montagem de `.ssh` dá ao contêiner as chaves SSH do host (risco; não copiar sem necessidade).
- `postCreateCommand` marca o workspace como `safe.directory` (`:36`) e `remoteUser` usa `${localEnv:USER}` (`:37`).
- Há vírgula final no array `mounts` (`:34`, `]` em `:35`). É aceita pelo JSONC do devcontainer (inferência).

`04-what-is-mcp/.devcontainer/compose.yml`:

- Serviço `ai-workspace` (`:3`), build com `Dockerfile` (`:4-6`).
- Args `GO_VERSION=1.23.1` (`:8`) e `USER_NAME=${USER}` (`:9`).
- Volume `../..:/workspaces:cached` (`:11`). Comando `sleep infinity` (`:12`).
- **Divergência de versão:** o contêiner recebe Go `1.23.1` (`compose.yml:8`), mas o `go.mod` pede `go 1.23.4` (`mcp-server/go.mod:3`). Com toolchain automática, o Go tentaria baixar 1.23.4 (inferência, não testada).

`04-what-is-mcp/.devcontainer/Dockerfile` (92 linhas):

- Base `ubuntu:22.04` (`:1`).
- `ARG GO_VERSION=${GO_VERSION}` (`:8`) é uma referência a si mesmo. Só funciona porque o compose passa o valor.
- Pacotes (`:22-30`), incluindo `sshpass` (`:24`). Ferramenta para senha de SSH; não é necessária ao servidor MCP.
- Go instalado por `wget` do `golang.org/dl` (`:37`), com a versão pelo `ARG`. Ferramentas de Go instaladas com `@latest` (`:52-55`): `gopls`, `go-outline`, `gocode@v1.0.0`, `revive@v1.3.2`.
- Usuário criado (`:63`), com `sudo` sem senha (`:70`, `NOPASSWD:ALL`).
- Oh My Bash por `curl ... | bash` a partir do branch `master` (`:88`). Risco de cadeia de suprimento; não reproduzir.

## 8. Pendências

1. Ler o código-fonte de `mark3labs/mcp-go` v0.8.2 (de preferência em um módulo baixado para pasta isolada, ou no cache do Go) para confirmar o fluxo da seção 5 e o formato de `IsError` em `NewToolResultError`.
2. Confirmar empiricamente se o stdout contaminado quebra algum cliente (teste de `initialize` contra o binário, com captura de stdout). Não foi feito nesta onda.
