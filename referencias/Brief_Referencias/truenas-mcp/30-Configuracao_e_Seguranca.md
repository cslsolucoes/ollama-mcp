# truenas-mcp — Configuração e segurança

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

## Flags → variáveis de ambiente

Binário principal (`MCPNas/truenas-mcp/cmd/truenas-mcp/main.go:20-27`, fallback em `:41-69`). Valores de credencial: `[omitido]`.

| Flag | Env | Default | Observação |
| --- | --- | --- | --- |
| `--truenas-url` | `TRUENAS_URL` | vazio | obrigatório; host ou `wss://` (`ws://` é rejeitado) |
| `--api-key` | `TRUENAS_API_KEY` | `[omitido]` | obrigatório; `log.Fatal` se faltar um dos dois (`main.go:49-51`) |
| `--insecure` | `TRUENAS_INSECURE` | `false` | env: `""`, `0`, `false`, `no`, `off` = falso; qualquer outro valor = verdadeiro (`main.go:63-69`) |
| `--tls-ca` | `TRUENAS_TLS_CA` | vazio | caminho de um PEM a confiar |
| `--debug` | `TRUENAS_MCP_DEBUG` | `false` | env lido em `init()` do pacote `truenas` (`client.go:24-30`) e de novo no `main` (`:55-58`) |
| `--version` | — | — | imprime `truenas-mcp version <Version>` e sai (`:36-39`) |

Padrão: **flag vence, env é fallback** (`if *x == "" { *x = os.Getenv(...) }`), validado depois. O proxy (não usado) repete o padrão com `--server-url`/`--api-key`/`--timeout` 30 s (`proxy/config.go:26-63`). **Não há** `--model`, `--timeout` do cliente WebSocket (fixo em 120 s, `client.go:815`) nem `--tools-verbosity`.

Diferenças para o conjunto do mcpollama (CLAUDE.md): o mcpollama acrescenta `--model`, `--tools-verbosity` e `--tls-ca`/`--insecure` com env próprio; a referência não tem `--api-key-file`. A linha de comando fica visível a outros processos (lista de processos), por isso o token deve ir pela env `OLLAMA_API_KEY` na config do cliente. `--api-key-file` foi considerado e **descartado pelo owner** (P-1 = não, 2026-10-23 15:50). O `.vscode/MCP-VSCODE.md:25-28` recomenda `inputs` + `env` para não deixar a chave no JSON.

## `--tls-ca` e `--insecure` em detalhe

Função única: `NewTLSConfig(insecure bool, caPath string) (*tls.Config, error)` em `MCPNas/truenas-mcp/truenas/tls.go:18-34`.

1. `insecure` → `&tls.Config{InsecureSkipVerify: true}` (`:19-21`). Tem precedência sobre a CA (se os dois forem dados, a CA é ignorada **sem aviso**; o `main` só avisa do `--insecure`, `main.go:78-80`).
2. `caPath == ""` → `&tls.Config{}`: validação padrão (cadeia + hostname) contra o pool do SO (`:22-24`).
3. `caPath != ""` → `os.ReadFile`, `x509.NewCertPool()`, `AppendCertsFromPEM`; sem certificado válido → erro `no PEM certificates found in <caminho>` (`:25-31`); resultado `&tls.Config{RootCAs: pool}` (`:33`).

Como entra no transporte: o `*tls.Config` é guardado em `Client.tlsConfig` (`client.go:41`) e passado ao `websocket.Dialer{TLSClientConfig: c.tlsConfig}` (`client.go:133-138`). **No mcpollama o ponto equivalente é `http.Transport{TLSClientConfig: cfg}`**; a função `NewTLSConfig` é reaproveitável como está (não depende de WebSocket).

Pontos finos (leitura do código; marcados quando são inferência):

- **`RootCAs: pool` substitui o pool do SO**, não soma. Com `--tls-ca` apontando só para a CA de origem do Cloudflare, o acesso **pela internet** (cadeia pública do Cloudflare na borda) falharia, porque a CA pública não está no pool. Para o mcpollama, que usa o mesmo host na LAN (`:8443`, Origin CA) e pela internet (sem porta), usar `x509.SystemCertPool()` (com *fallback* para `NewCertPool()` se retornar erro) e então `AppendCertsFromPEM`. É um **achado desta análise**, não registrado pelo projeto de origem; validar contra o servidor real numa onda própria.
- A verificação inclui **hostname**: o certificado precisa ter SAN para o nome usado (`ia.cslsolucoes.com.br`); `ServerName` não é sobrescrito (**inferência**: não há campo `ServerName` em `tls.go`).
- Sem `MinVersion` (o padrão do Go moderno é TLS 1.2). Sem *pinning* de certificado: a "CA fixada" aceita qualquer cert emitido por aquela CA; o teste `TestTLSCAPinDoesNotDisableVerification` prova que outro cert **continua rejeitado** (`truenas/tls_test.go:83-117`).
- A mensagem de falha ensina o usuário: se o erro contém `x509:` ou `certificate`, é anexado `(... pass --tls-ca <cert.pem> to trust it, or --insecure ...)` (`client.go:145-147`). O teste exige que a mensagem cite `--tls-ca` (`tls_test.go:43-57`).
- `--insecure` imprime `WARNING: TLS certificate verification disabled ...` no log (stderr) (`main.go:78-80`).
- Esquema obrigatório: `ws://` é recusado com `SECURITY ERROR`, pois o TrueNAS revoga chaves usadas sem TLS (`client.go:273-276`). Equivalente no mcpollama: recusar `http://` para host que não seja loopback quando houver Bearer (**sugestão**).
- Os utilitários `cmd/fetch-*` só implementam `-insecure` (sem `--tls-ca`), e criam `tls.Config` à mão (`cmd/fetch-system-info/main.go:32-35`).

## Autenticação

- O TrueNAS usa chave de API por sessão WebSocket: `auth.login_with_api_key` com a chave como parâmetro posicional (`client.go:293-325`). O estado `authenticated` é por conexão e é refeito em reconexão (`client.go:187-189`, `:706`).
- Autenticação **lazy** no primeiro `Call` (`client.go:327-344`); a ordem dos eventos na inicialização: `initialize` não toca a rede (`main.go:89-97`).
- Equivalente do mcpollama: `Authorization: Bearer` em cada requisição HTTP (CLAUDE.md); sem estado de sessão, então nada a refazer.

## Segredos em log e em erro

Três camadas, todas em `truenas/client.go`:

1. **Log mínimo por padrão.** Sem `--debug` o log mostra só metadados (`Sending request: id=%s method=%s`, `Received response: id=... result=N bytes`, `client.go:247`, `:759`). Com `--debug`, corpos completos, mas redigidos e truncados (`:234-245`, `:753-757`).
2. **Máscara por nome de campo.** `sensitiveKeyFragments` (`:511`, 19 fragmentos: `password`, `passwd`, `bindpw`, `secret`, `api_key`, `apikey`, `token`, `credential`, `privatekey`, `private_key`, `passphrase`, `key`, `salt`, `hash`, `provisioning_uri`, `community`, `pass`, `registration_details`, `monpwd`) com *substring match* (`isSensitiveKey`, `:513-521`) aplicado recursivamente (`redactValue`, `:524-545`) a requisições (`redactParams`, `:676-689`; para `auth.*` **todos** os parâmetros viram `[REDACTED]`) e a mensagens stdio de debug (`RedactJSONForLog`, `:551-561`). A lista cresceu a cada vazamento descoberto (ver `80-Licoes_Aprendidas.md`, LIC-TNS-005): é uma lista negra, sempre atrasada.
3. **Remoção do valor exato.** `scrubSecrets` troca a API key por `[REDACTED]` em mensagens e *traces* devolvidos pelo servidor (`:665-670`; usado em `:245`, `:857`, `:869`, `:872`); o teste `TestServerEchoedKeyIsScrubbed` simula o servidor ecoando a chave (`truenas/client_test.go:772-802`).
4. **Resposta com segredo escalar.** `redactResponseForLog` troca o `result` inteiro por `[REDACTED]` para `auth.*` e para métodos listados em `bareScalarSecretResultMethods` (`:611-661`).

O que **não** é protegido: o resultado devolvido ao cliente MCP vai verbatim (política explícita, comentários `client.go:477-481`); a redação cobre só o espelho de debug no stderr. Prévias de `dry_run` mascaram campos secretos localmente (bug-001/002/013).

Para o mcpollama: o segredo é um só (Bearer). A camada 3 (valor exato) cobre `Authorization` ecoado e é barata; a camada 2 é desnecessária **desde que nenhum log de payload seja ligado por padrão**. Atenção específica: as tools que **leem arquivos** podem colocar conteúdo sensível em prompts/respostas; o log de `--debug` deve registrar tamanhos e contadores, não o texto (e sempre com teto de linha, LIC-TNS-001).

## Teto de linha de log

`maxDebugLogLineBytes = 4000` e `TruncateForLog` (`client.go:581-592`), aplicado em `main.go:143,296` e `client.go:245,757`. Motivo: deadlock de pipe stderr (bug-016). Redação **antes** do truncamento.

## Gate destrutivo

Não há flag global tipo `--allow-destructive`. O controle é **por tool**: `dry_run` (`tools/dryrun.go`), `confirm=true` para exposição de segredo/desligamento (`tools/system_advanced.go:383-395`, `registry.go:454-475`) e pré-checagens que recusam a operação (`tools/boot.go:217-231`). Regra fixada no `.wolf/cerebrum.md:69`: nunca apagar/resetar a chave de API que o próprio servidor usa (autocorte). O mcpollama não tem escrita remota; o gate relevante é sobre **leitura de arquivos** (allowlist de diretórios, teto de bytes), matéria de `MCPO-REQ-FILE-*`.

## Processo e credenciais

- `.wolf/cerebrum.md:30` e `:34`: o ambiente bloqueia subagentes que passam a chave como argumento literal; o padrão aceito é `cmd.Env` dentro de um programa Go descartável. Vale para os testes ao vivo do mcpollama (usar o caminho do arquivo, nunca o conteúdo).
- `.wolf/cerebrum.md:67`: validações ao vivo de consultas com credenciais devem filtrar campos (`options.select`) para não levar hashes/segredos ao transcript.
- **Arquivos excluídos (nunca abertos):** `.mcp.json`, `settings.local.json`, `*.env`, `*.pem`, `*.key`, `reports/*servidor-teste-config*`. Este documento não depende deles.
