# truenas-mcp — visão geral

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.1 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## O que é

Servidor **MCP em Go (stdio, JSON-RPC 2.0, tipos MCP escritos à mão)** que expõe a API JSON-RPC do **TrueNAS 25.10** como tools para clientes MCP (Cursor, VS Code, Claude Code). Módulo Go `github.com/truenas/truenas-mcp` (imports em `MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:14-17`). Diferença estrutural em relação ao mcpollama: o backend **não é HTTP**. O `truenas.Client` fala **WebSocket** (`gorilla/websocket`) com o middleware do TrueNAS: *connect* + `auth.login_with_api_key` e multiplexação de requisições por `id` (`MCPNas/truenas-mcp/truenas/client.go:123-199`, `:203-256`, `:293-325`). Um plano de cobertura de API (110 fases / 680 ondas) foi concluído e gerou **730 tools** (`MCPNas/truenas-mcp/.wolf/STATUS.md:5`; contagem confirmada abaixo).

Para o mcpollama importam as **soluções de infraestrutura** de um servidor MCP stdio que já passou por clientes reais (Cursor, extensão Claude Code do VS Code): TLS por CA explícita, `--insecure` como opt-in, redação de segredos em log, jobs assíncronos com `task_id` e os bugs de handshake/pipe do `.wolf/buglog.json`.

## Fontes e dimensionamento

Medido em 2026-10-23 em `referencias/MCPNas/truenas-mcp/` (linhas **físicas**, como `wc -l`; `docs/` fora das contagens de código).

| Medida | Valor | Como foi medida |
| --- | ---: | --- |
| Arquivos `.go` | **116** | `find . -name '*.go' \| wc -l` |
| Linhas físicas `.go` | **109.126** | `find . -name '*.go' -print0 \| xargs -0 cat \| wc -l` |
| Arquivos de teste | 10 (4.261 linhas) | `find . -name '*_test.go'` fora de `docs/` |
| Tools registradas | **730** | `grep -c '^\s*r\.tools\["' tools/registry.go` = 730; o binário compilado devolveu 730 entradas no `tools/list` |
| Tamanho do `tools/list` | **892.704 bytes** (~872 KiB) | resposta real do binário, `wc -c` da linha |
| Binário na raiz | `TruenasMcp.exe`, 15.545.344 bytes | `ls -la` (o CLAUDE.md do mcpollama proíbe versionar binários) |

Por pasta:

| Pasta | `.go` | Linhas | Papel |
| --- | ---: | ---: | --- |
| `cmd/truenas-mcp/` | 1 | 308 | entrypoint stdio + `StdioHandler` |
| `cmd/fetch-system-info/`, `cmd/fetch-update-info/` | 2 | 112 | utilitários descartáveis de uma chamada |
| `mcp/` | 1 | 71 | tipos JSON-RPC/MCP e interface `ToolRegistry` |
| `proxy/` | 2 | 152 | leitor/gravador stdio + config (**não importado por ninguém**, ver `10-Arquitetura.md`) |
| `tasks/` | 4 | 568 | jobs assíncronos: store com TTL, manager, poller |
| `tools/` | 102 (8 de teste) | 106.053 | `registry.go` (26.107 linhas, **todas** as definições) + handlers por família |
| `truenas/` | 4 (2 de teste) | 1.862 | cliente WebSocket, TLS, redação de log |
| **Total** | **116** | **109.126** | 3+1+2+4+102+4 arquivos; 420+71+152+568+106.053+1.862 linhas |

Outros materiais:

| Item | Tamanho | Observação |
| --- | --- | --- |
| `docs/` | **2.069 arquivos, 220 MB** (`du -sh`; 1.019 `.html`, 1.017 `.txt`, 855 `api_methods_*.html`) | Cópia local da **documentação HTML da API do TrueNAS**, versão 25.10.5 (`MCPNas/truenas-mcp/.wolf/STATUS.md:24`). Não resumida. |
| `.wolf/` | `cerebrum.md` 85 · `buglog.json` 257 (21 bugs) · `STATUS.md` 295 · `memory.md` 253 linhas | fonte principal de `80-Licoes_Aprendidas.md` |
| `.workspace/plans/` | 1 plano de 3.927 linhas + 3 concluídos (101-113) | cobertura da API; os concluídos são governança do pack `.cursor` |
| `.workspace/reports/` | 6 reports lidos (63 a 2.253 linhas) | 2 são referência de tools / comandos de infra (não resumidos) |
| `.workspace/rules/` | 1 regra (387 linhas) | lista de pacotes Delphi do dono; sem relação com este servidor Go |
| `scripts/` | 3 scripts Python | geradores auxiliares (Postman/lista de tools), fora do build |
| `.vscode/` | `MCP-VSCODE.md` (45), `tasks.json` (340, template Delphi) | `mcp.json` **não foi lido** por precaução (o doc diz que a API key fica nele) |

### Lacunas desta cópia (GAP)

- **Não há `go.mod`, `go.sum`, `Makefile`, `README.md` nem `.gitignore`**, embora `MCPNas/truenas-mcp/.wolf/anatomy.md:10-22` os liste. A cópia não compila como está. Para medir, esta onda copiou `cmd mcp proxy tasks tools truenas` para o scratchpad e criou um `go.mod` mínimo (`go mod tidy` resolveu `gorilla/websocket v1.5.3` e `google/uuid v1.6.0`). Nada foi gravado em `referencias/`.
- O `Makefile` (convenção de versão com `git describe`) só aparece citado em `.wolf/buglog.json` (bug-021, campo `fix`); a receita de release é só a descrita nesse texto.

## Medições de verificação (na cópia do scratchpad)

| Comando | Resultado |
| --- | --- |
| `go build ./...` e `go vet ./...` (Go 1.26.5, Windows) | sem erro / sem saída |
| `go test -count=1 -v ./truenas ./tools` | **64** testes de nível raiz `--- PASS`, 0 `--- FAIL` (27 em `truenas/`, 37 em `tools/`) |
| `go test -count=1 -cover ./truenas` | **70,6%** das instruções |
| `go test -count=1 -cover ./tools` | **falha**: `tools\kmip.go:1:1: invalid BOM in the middle of the file`. `tools/kmip.go` é o único dos 116 arquivos que começa com BOM `EF BB BF` (checagem dos 3 primeiros bytes de cada um). Sem `-cover` passa. |
| `GOOS=linux GOARCH=amd64 go build ./cmd/truenas-mcp` | binário de 15.416.372 bytes |
| `-ldflags "-X main.Version=probe-1"` + `--version` | imprime `truenas-mcp version probe-1` |
| Sessão stdio com URL/chave **fictícias** (`initialize`, `notifications/initialized`, `ping`, `tools/list`, tool inexistente, linha inválida) | `initialize` responde `protocolVersion` `2024-11-05` fixo; `ping` devolve `-32601`; `tools/list` devolve 730; tool inexistente devolve `isError:true`; linha inválida devolve `-32700`. Nenhuma conexão ao TrueNAS (auth lazy). |

## Índice

| Documento | Conteúdo |
| --- | --- |
| [10-Arquitetura.md](10-Arquitetura.md) | `cmd/`, `mcp/`, `proxy/` (o que é de fato), `tasks/` e gerações longas do Ollama, fluxo de um `tools/call` |
| [20-Tools_e_Contratos.md](20-Tools_e_Contratos.md) | declaração de tools, schema/saída/erro, famílias, 3 famílias em detalhe |
| [30-Configuracao_e_Seguranca.md](30-Configuracao_e_Seguranca.md) | flags→env, `--tls-ca`/`--insecure`, auth, redação de segredos, gates |
| [40-Codigo_Arquivo_a_Arquivo.md](40-Codigo_Arquivo_a_Arquivo.md) | fichas dos arquivos de núcleo com `caminho:linha` |
| [50-Testes_Build_Release.md](50-Testes_Build_Release.md) | testes, mocks, build, cross-compile, versão, release |
| [80-Licoes_Aprendidas.md](80-Licoes_Aprendidas.md) | `LIC-TNS-nnn` |

## O que tirar para o mcpollama

1. **`--tls-ca` + `--insecure` com fallback para env, e erro com dica.** Copiar o desenho de `MCPNas/truenas-mcp/truenas/tls.go:18-34` (insecure / CA própria / padrão do SO) e a mensagem de ajuda quando o `x509:` falha (`truenas/client.go:145-147`). **Ajuste:** `tls.go:33` troca o pool inteiro (`RootCAs: pool`); como o mcpollama também é acessado pela internet (certificado público do Cloudflare), usar `x509.SystemCertPool()` e **acrescentar** a CA de origem (`30-Configuracao_e_Seguranca.md`).
2. **Responder `initialize`/`tools/list` sem tocar o backend.** O bug-021 custou ~1,4 s de handshake e derrubou duas tentativas do cliente; correção: autenticar de forma lazy (`MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:89-97`). No mcpollama: nada de `GET /api/tags` no arranque.
3. **Limitar toda linha de log de payload.** Uma linha de ~716 KB travou o loop stdio por deadlock de pipe (bug-016); correção `TruncateForLog`, teto de 4.000 bytes, aplicada **depois** da redação (`truenas/client.go:581-592`).
4. **O loop stdio sequencial é um risco.** `main.go:135-173` trata uma requisição por vez; um `tools/call` de 120 s bloqueia até o `ping`. No mcpollama: goroutine por requisição com `stdoutMutex` (já existe em `main.go:123,286-301`) e fila própria para a GPU.
5. **O padrão `task_id` + `tasks_get` serve, com adaptação, para gerações longas.** Transfere o desenho (`tasks/types.go:8-51`, tools em `tools/registry.go:4880,4902`); **não** transfere o `Poller` (consulta `core.get_jobs` do TrueNAS, `tasks/poller.go:58-82`). Para o Ollama a tarefa roda numa goroutine local que grava o texto no store.
6. **Não repetir a dívida do `Task.Result`.** `tasks/types.go:42` usa `json:"-"`, então `tasks_get` nunca devolve o resultado (bug-015, não corrigido na raiz). Numa geração, o resultado é o produto: expô-lo.
7. **"Espera limitada + fallback para tarefa".** `handleCoreJobWait` espera até 100 s (`tools/core.go:510-516`, abaixo do timeout de 120 s por chamada do **cliente WebSocket do TrueNAS**, `truenas/client.go:815-821`; não é o timeout do cliente MCP, que nenhuma referência mediu); `audit_export` idem (`tools/audit.go:810-830`). Mapear para `generate`: aguardar X s abaixo do timeout do cliente MCP e, se estourar, devolver `task_id`.
8. **Redação de segredos por mecanismos complementares:** valor exato removido de erros do servidor (`client.go:665-670`), máscara por nome de campo (`client.go:511-545`) e log só de metadados por padrão (`client.go:234-248`, `:753-760`). Para o mcpollama (um único segredo, o Bearer) o primeiro e o terceiro bastam.
9. **Teste de TLS em 4 cenários com `httptest.NewTLSServer`** (`truenas/tls_test.go:43-117`): cert não confiável rejeitado (mensagem cita `--tls-ca`), `--insecure` aceita, CA fixada aceita, **CA fixada não desliga a verificação** de outro cert.
10. **Capturar o log num buffer e afirmar que o segredo não aparece**, nos modos padrão e debug (`truenas/client_test.go:700-770`), mais "servidor ecoa a chave no erro" (`:772-802`).
11. **Cuidar do tamanho do `tools/list`.** Medido: 892.704 bytes para 730 tools (~220 mil tokens a 4 bytes/token, **inferência**). O mcpollama terá ~7 tools, mas descrições longas pesam: manter `--tools-verbosity compact`.
12. **Limite de nome de tool do Cursor:** `len(server_id) + len(tool_name) <= 60` (bug-017, `.wolf/cerebrum.md:36`). O identificador do servidor entra na conta.
13. **Formato do `mcp.json`:** Cursor usa `mcpServers`; VS Code usa `servers` com `"type": "stdio"` (`.vscode/MCP-VSCODE.md:36-45`). O mesmo doc diz "stdio + Content-Length" (`:45`), o que **contradiz o código** (uma mensagem JSON por linha, `main.go:140-141,299`); é texto herdado do projeto Delphi.
14. **Checar BOM e fim de linha no scaffold.** `tools/kmip.go` com BOM quebra `-cover` (medido); o repo em CRLF gerou 5 regressões (bug-003/006/011/012). Definir `.gitattributes` desde o início.
