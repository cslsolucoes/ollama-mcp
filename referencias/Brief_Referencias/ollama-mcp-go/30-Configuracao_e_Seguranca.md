# ollama-mcp-go — configuração e segurança

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Busca feita em `*.go` por `Authorization`, `Bearer`, `tls`/`TLS`, `flag.`: **0 ocorrências** (`grep -n`, executado em 2026-10-23).

## Configuração: só 2 variáveis de ambiente, 0 flags

| Item | Tipo | Default | Efeito | Fonte |
| --- | --- | --- | --- | --- |
| `OLLAMA_HOST` | env | `http://localhost:11434` | URL base do Ollama; `/` final removido por `strings.TrimRight(host, "/")` | `ollama-mcp-go/main.go:159-165`; `ollama-mcp-go/README.md:140` |
| `OLLAMA_MCP_TRACE` | env | `/tmp/ollama-mcp-trace.log` | caminho do trace; `off` desliga; vazio = padrão | `ollama-mcp-go/main.go:298-309`; `ollama-mcp-go/README.md:141` |

Não existem: flags de linha de comando (nem `--version`, nem `--help`), arquivo de configuração, modelo padrão, token, CA, `insecure`, timeout configurável, `--debug`. O único "debug" é o log em stderr com `log.Ltime` (`ollama-mcp-go/main.go:628-629`). Detalhes que valem atenção:

- `OLLAMA_HOST` é lido **na criação do servidor**, uma vez (`ollama-mcp-go/main.go:160`). A API real do Ollama também usa `OLLAMA_HOST`, o que casa com o nome; o valor é um URL completo (com esquema). Um valor no formato `host:porta` do Ollama (sem `http://`) quebraria o `http.Client` (inferência; não executado).
- O servidor loga o URL no start (`ollama-mcp-go/main.go:632`): `ollama-mcp-go starting (ollama: <url>)`. O log vai a stderr; se o URL embutir credenciais (`https://user:pass@host`) elas vazariam no log (inferência; o código não filtra).
- O README só mostra `env` no bloco do cliente MCP (`ollama-mcp-go/README.md:64-77`).

## URL do Ollama

- Default local sem TLS, `http://localhost:11434` (`ollama-mcp-go/main.go:162`). O servidor da CSL (`https://ia.cslsolucoes.com.br:8443`, CA de origem Cloudflare) **não funciona** com este código sem alteração: não há como passar CA nem token (ver abaixo).
- A mensagem de erro de rede inclui o URL completo (executado: `Post "http://localhost:11434/api/generate": dial tcp ...`), `ollama-mcp-go/main.go:384`, `428`, `561`.

## Autenticação

- **Não existe Bearer, nem qualquer cabeçalho de auth.** As chamadas usam `s.client.Post(...)`/`s.client.Get(...)` diretos, sem `http.NewRequest`, portanto sem possibilidade de cabeçalho (`ollama-mcp-go/main.go:383`, `426`, `559`).
- Consequência: contra o nginx da CSL a resposta seria `401` (token ausente). O corpo do `401` seria repassado como texto de erro `Ollama returned 401: ...` (`ollama-mcp-go/main.go:389-392`).
- Para o mcpollama: trocar para `http.NewRequestWithContext` e enviar o cabeçalho `Authorization` com esquema Bearer (valor `[omitido]`), com o token vindo de `OLLAMA_API_KEY` (env, preferido) ou `--api-key`; `--api-key-file` foi descartado pelo owner (P-1 = não, 2026-10-23 15:50); nunca em log nem em mensagem de erro.

## TLS

- `http.Client{Timeout: 5*time.Minute}` **sem `Transport`** (`ollama-mcp-go/main.go:166-168`): usa o armazém de CAs do sistema, sem `RootCAs` customizado, sem `InsecureSkipVerify`. Não há `--tls-ca` nem `--insecure`.
- Para o mcpollama: precisa de `tls.Config{RootCAs: <pem>}` via `--tls-ca`/`OLLAMA_TLS_CA` e, só como exceção, `--insecure`. O molde está em `MCPNas/truenas-mcp` (a ser detalhado na onda B2).

## Timeouts e limites

| Limite | Valor | Fonte |
| --- | --- | --- |
| `http.Client.Timeout` | 5 min, **para a requisição inteira, inclusive a leitura do corpo/stream** | `ollama-mcp-go/main.go:167` |
| Buffer de linha de stdin | 4 MB | `ollama-mcp-go/main.go:635` |
| Buffer de linha do stream NDJSON | 1 MiB | `ollama-mcp-go/main.go:395` |
| Corpo de erro HTTP lido | **sem limite** (`io.ReadAll`) | `ollama-mcp-go/main.go:390`, `433`, `566` |
| Tamanho de prompt, `messages`, `text` | **sem limite** no servidor | `ollama-mcp-go/main.go:453-590` |
| Concorrência | 1 requisição por vez (loop sequencial) | `ollama-mcp-go/main.go:656` |
| Retry/backoff | nenhum | (ausência em `main.go`) |
| Contexto/cancelamento (`context.Context`) | nenhum; não há `context` importado | `ollama-mcp-go/main.go:3-14` |

Para o Ollama da CSL (ctx 32k, fila de 4 GPUs por um modelo residente): 5 min cobre `qwen3-ia` em tarefas normais, mas é um valor fixo, sem flag (inferência de adequação; não medido aqui). [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]

## Trace file: o que grava, onde, risco

- **Onde:** caminho de `OLLAMA_MCP_TRACE`; sem a variável, **`/tmp/ollama-mcp-trace.log`** (`ollama-mcp-go/main.go:298-309`). **Ligado por padrão.** Em Windows, `/tmp/...` é resolvido como `\tmp\...` na unidade corrente; se a pasta não existir, `os.OpenFile` falha e o trace se desliga com `log.Printf("trace disabled: ...")` (`ollama-mcp-go/main.go:329-333`), então no Windows o padrão tende a não gravar (inferência; não executado em Windows nesta onda).
- **Como abre:** `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0o644)` (`ollama-mcp-go/main.go:329`): **legível por todos os usuários da máquina** (permissão `0644` em Unix), e o arquivo **só cresce** (append, sem rotação nem limite).
- **O que grava** (`ollama-mcp-go/main.go:335-336`, `343-375`):
  - cabeçalho `=== HH:MM:SS  <kind>  <model> ===`;
  - **o prompt inteiro** (`> <prompt>`): para `generate`, o `prompt`; para `chat`, **só o conteúdo da última mensagem** (`ollama-mcp-go/main.go:472`, `529`). O `system` e as mensagens anteriores **não** entram;
  - **a resposta inteira**, token a token, no momento em que chega;
  - **o raciocínio do modelo** (`thinking`), separado por `--- pense ---` / `--- repond ---` (`ollama-mcp-go/main.go:358-367`, `343-353`), que **nunca** vai ao cliente MCP (`ollama-mcp-go/README.md:167-170`);
  - rodapé `[done: N prompt / M eval]`.
- **Risco de vazamento:** alto para o mcpollama. Prompts e respostas (inclusive o raciocínio, que o cliente não vê) ficam em texto puro, permanentemente, em um caminho previsível e legível por outros usuários. Quando o mcpollama ler arquivos sensíveis no servidor (cofre, dados de clientes) e devolver só o resumo, o trace gravaria o **conteúdo do arquivo** se ele fosse colocado no prompt, anulando o objetivo de "dados sensíveis não saem" (`CLAUDE.md` do projeto, seção "O projeto"). Também há **condição de corrida** entre processos: duas instâncias do servidor intercalam linhas no mesmo arquivo (inferência: `O_APPEND` sem trava por chamada).
- **Falha no trace não é fatal** (`ollama-mcp-go/main.go:320-322`, `ollama-mcp-go/README.md:172-173`): bom princípio de design; manter.
- **Testes** isolam o trace com `TestMain` (`OLLAMA_MCP_TRACE=off`, `ollama-mcp-go/main_test.go:73-76`), para não poluir o `/tmp` real; `TestTraceFile` reativa com `t.TempDir()` (`ollama-mcp-go/main_test.go:397-422`).

## O que falta em segurança para o mcpollama

Cada item é lacuna desta referência (não é defeito do código dela, que é um servidor local).

| # | Falta | Observação |
| --- | --- | --- |
| 1 | Auth Bearer, token por arquivo, nunca logado | `Authorization` ausente (grep: 0) |
| 2 | TLS com CA customizada e `--insecure` explícito | nenhum `Transport` |
| 3 | Trace desligado por padrão; permissão `0600`; sem conteúdo de arquivo; sem raciocínio persistido | `ollama-mcp-go/main.go:298-309`, `329` |
| 4 | Mascarar URL e corpo de erro HTTP em mensagens ao cliente (limitar `io.ReadAll`) | `ollama-mcp-go/main.go:390-391`, `428` |
| 5 | Limites de entrada: tamanho de linha stdin, de prompt, de `messages` | `ollama-mcp-go/main.go:635` |
| 6 | **Superfície de leitura de arquivos:** allowlist de raízes, resolução de `..` e symlinks, limite de bytes, bloqueio de `.env`/`*.pem`/`*.key`/`.mcp.json` | não existe nenhuma tool que leia arquivo aqui |
| 7 | Gate para ações destrutivas (`--allow-destructive` + `confirm`) | sem tools administrativas, então só vale se surgirem; `MCPVMWare` é o molde |
| 8 | `-32700` para JSON inválido e erro para requisição sem `id` | `ollama-mcp-go/main.go:644-654` |
| 9 | Logs: não imprimir segredos; `--debug` para detalhe | só `log.Printf` com `Ltime` |
| 10 | Cancelamento (`context`) ao receber `notifications/cancelled` ou EOF | sem `context` |
| 11 | Validar `role` de `chat` e tamanho do histórico (custo de tokens e contexto de 32k [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]) | `ollama-mcp-go/main.go:509-514` |
| 12 | Conteúdo devolvido pelo modelo é texto não confiável: não executar nem repassar como instrução | o servidor devolve a resposta crua (`ollama-mcp-go/main.go:480`) |
