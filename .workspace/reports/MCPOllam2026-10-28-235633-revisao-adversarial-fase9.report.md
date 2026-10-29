# Revisão adversarial da Fase 9 (gateway: fila, proxy, monitor, store) — etapa 9.5

- **Projeto:** mcpollama (`E:\MCPOllam`), servidor MCP em Go + gateway `mcpollama-gateway`
- **Alvo:** `src/gateway/{queue,proxy,monitor,store,gpu}`, `src/cmd/mcpollama-gateway` e as mudanças da 9.4 em `src/ollama`, `src/tools` e `src/mcpollama-mcp`, no estado do commit `315f996` (faixa `ced50f8..315f996`, commits 418b028, 75a1999, b47d763, f6a9b9c e 315f996)
- **Onde:** worktree `E:\MCPOllam\.claude\worktrees\agent-a5589ffef1a3fdd1b`, branch `worktree-agent-a5589ffef1a3fdd1b` (o repositório principal não foi tocado)
- **Revisor:** subagente adversarial, modelo Opus 5.5 (etapa 9.5 do plano, nível opus por ser verificação crítica de segurança)
- **Data:** 2026-10-28 23:24 a 2026-10-29 00:15 (hora local, `Get-Date`)
- **Ambiente:** Windows 11 IoT Enterprise 10.0.28000, go1.27.0 windows/amd64, 16 CPUs
- **Restrições respeitadas:** nenhum acesso ao servidor, SSH ou Ollama real (todo upstream é `httptest`); `D:\MCP` só lido (DLL e tessdata do OCR); `.mcp.json`, `referencias/`, `src/files`, `src/doctext` e `src/ocr` não foram alterados; sem push nem merge.

---

## 1. Veredito

**Nenhum achado Alto.** Os controles centrais aguentaram todos os ataques: `X-Real-IP` forjado, smuggling (CL/TE duplicados, `chunked, identity`, header com espaço), 17 variantes de rota bloqueada, contadores enganados pelo texto do modelo, vazamento de slot (Ollama que cai antes e depois dos headers, cliente que desiste no escalonamento), XSS, CSRF, fixação de sessão, injeção SQL no `/history` e a sentinela nos caminhos de erro. O `error_class` só carrega texto fixo do gateway (`upstream_500`, `stream_error`, …), nunca o corpo do Ollama.

Há **5 achados Médios**, todos de disponibilidade ou de controle de acesso ao `/monitor`, e **7 Baixos**. **Todos foram corrigidos**, cada um com teste de regressão no pacote:

1. **M1** (do coordenador): TOCTOU no limite de login. 40 tentativas paralelas do mesmo IP → **40 senhas avaliadas** (limite 5), cada uma com PBKDF2.
2. **M2** (do coordenador): bloqueio global. 40 falhas anônimas de 40 IPs → o administrador, de IP limpo e com a senha certa, recebe **429**.
3. **M3** (pendência 4 da 9.4): SSE com teto só global. **1 IP abre os 20 streams** e o resto da LAN recebe 503.
4. **P6** (pendências 6/7): sem `--run-timeout`. Um stream sem fim **prende o slot para sempre**.
5. **S2:** histórico sem teto de linhas. A retenção é só por idade, e um cliente em laço na taxa do nginx grava ~864 mil linhas/dia por 90 dias. Prova: 3.150.000 linhas recentes continuam 3.150.000 depois do `Maintain`.

## 2. Números

| Execução | Vetores (`VEC`) | OK | FALHA | Suíte |
| --- | --- | --- | --- | --- |
| Código `315f996` + testes de ataque (`antes.log`, `-short`, sem M8 e C5, que vieram depois) | 27 | 18 | **9** (M1, M2, M3, P2, P6, Q1, S1, S3, C1) | só os testes de ataque falham |
| Vetor S2 no código `315f996` (sem `-short`, 115 s) | 1 | 0 | **1** (S2: 3.150.000 linhas após o `Maintain`, teto 3.000.000) | saída capturada na sessão (`--- FAIL: TestAdvRequestsGrowthWithoutCap (114.85s)`), **não gravada em arquivo**; o `s2-depois.log` é a rodada corrigida |
| Vetor M8 no código `315f996` | 1 | 0 | **1, por inspeção** (o vetor usa o contador da correção; ver §3.6) | — |
| Vetor C5 no código `315f996` (escrito depois do `antes.log`; os outros arquivos já estavam corrigidos, mas o C5 depende só do `main.go` e do `monitor.go` sem `Close`) | 1 | 0 | **1** (não encerra em 10 s) | — |
| Código final + testes de ataque (`depois.log`, `-short`) | 29 | **29** | 0 | todos os pacotes `ok` |
| Vetor S2 no código corrigido (`s2-depois.log`, 138 s) | 1 | **1** | 0 | 3.150.000 → 3.000.000 linhas |
| **Total desta revisão** | **30** | 18 OK desde o início | **12 achados** (5 Médios, 7 Baixos), todos corrigidos | — |

Evidência bruta em `…-revisao-adversarial-fase9.testes\`: `antes.log`, `depois.log`, `s1-depois.log`, `s2-depois.log`, `fuzz\*.log`, `gate-*.log`.

### Medida da pendência 5 (Stats com 200 mil linhas, banco sintético de 90 dias)

207.000 linhas espalhadas em 90 dias (~2,3 mil/dia), gravadas por CTE recursiva em 3,7 s:

| Chamada | Antes (sem índice em `finished_at`) | Depois (`idx_requests_fin`) |
| --- | --- | --- |
| `Stats` 15 min (24 linhas) | **48,2 ms**, 0,4 MiB, `SCAN requests` | **0,56 ms**, 0,0 MiB, `SEARCH … USING INDEX idx_requests_fin` |
| `Stats` 12 h (1.151) | 48,7 ms, 0,6 MiB | 2,3 ms, 0,2 MiB |
| `Stats` 24 h (2.301) | 51,0 ms, 0,9 MiB | 4,4 ms, 0,5 MiB |
| `Stats` 7 d (16.101) | 70,8 ms, 4,0 MiB | 33,6 ms, 3,6 MiB |
| `Stats` 90 d (não exposto na API) | 305,6 ms, 49,2 MiB, **truncado em 200.000 sem aviso** | 422,5 ms, 49,2 MiB, agora com `"truncated": true` |
| `History` 50 | < 1 ms | 0,5 ms |
| `History` com `offset` no fim | 347 ms | 348 ms (varredura do OFFSET; limitada ao tamanho da tabela) |

O monitor chama `Stats` duas vezes a cada 2 s enquanto houver um painel aberto ("hoje" e "15m"). Antes eram ~100 ms de CPU a cada 2 s na máquina do Ollama; agora são ~3 ms.

## 3. Método

1. Li todo o código novo (`queue.go`, `proxy.go`, `monitor.go`, `admin.go`, `store.go`, `main.go`, `config.go`, `index.html`) e o diff da 9.4 em `src/ollama`/`src/tools`, antes de atacar.
2. Escrevi um arquivo de ataque por pacote (`fase9_adv_test.go`), como na Fase 7. Cada vetor imprime `VEC|id|severidade|arquivo:linha|resultado`, e os de achado **falham** no código atual.
   - `queue`: corrida Cancel × abandono, inanição de `baixa`, desistência no escalonamento, corrida das ações × início/fim com `-race`;
   - `proxy`: `X-Real-IP` (8 casos), rotação IPv6, smuggling por TCP cru (7 formas), 17 variantes de rota bloqueada pelo mesmo mux do entrypoint, contadores enganados, resposta sem fim, vazamento de slot;
   - `store`: banco sintético de 90 dias (medida), crescimento sem teto, eventos de descarte, injeção SQL, sentinela nos caminhos de erro;
   - `monitor`: TOCTOU do login, bloqueio global, SSE por IP, XSS, CSRF (11 casos), fixação de sessão, exposição de segredo, `/stats` sem cache;
   - `cmd/mcpollama-gateway`: `--priority-ips` mapeado e malformado, `hash-password`, ponta a ponta com `--debug` (sentinelas de token, prompt e resposta), encerramento com painel aberto;
   - `tools`: nova tentativa só em 503 + `Retry-After` (9 casos).
3. Rodei os vetores no código `315f996` (`antes.log`), corrigi, rodei de novo (`depois.log`) e escrevi um teste de regressão por achado, no próprio pacote (`fase95_regress_test.go`).
4. **Prova de que a regressão falha antes:** copiei `src` do commit `315f996` (`git archive`, sem tocar o repositório principal) para o scratchpad, copiei os `fase95_regress_test.go` e rodei. O de `queue` (Q1) **falha por asserção** (`records []`). Os de `proxy`, `store`, `monitor` e `cmd` **não compilam**, porque usam a API da correção (`RunTimeout`, `fairIP`, `MaxRequests`, `Truncated`, `tryBegin`, `statsComputed`). Para esses, a prova de comportamento é o vetor de ataque correspondente, que compila contra o código antigo e falha no `antes.log`.
5. Fuzz de 10 min por alvo (§5), em paralelo, com `-parallel 3` cada.
6. Os arquivos de ataque foram movidos para `…fase9.testes\<pacote>\` antes do gate final. Para rodá-los de novo, copie-os de volta para o pacote: eles usam os auxiliares dos `fase95_regress_test.go`.

### 3.1 Hipótese refutada (registro honesto)

A 1ª hipótese do C1 era que `--priority-ips ::ffff:10.0.0.5` virava `::ffff:10.0.0.5/32` e **casava com todo IPv4**, ou seja, prioridade `alta` para qualquer cliente. O spike (`net.ParseCIDR` no scratchpad) **refutou**: a rede vira `::/32` e **não casa com nada**, nem com 10.0.0.5. O defeito real é a **falha silenciosa** (a entrada é aceita e não tem efeito), Baixa. O vetor foi reescrito para o defeito real antes de corrigir.

### 3.2 Erros de processo meus, corrigidos antes dos números finais

- **(a)** O 1º vetor Q4 (corrida das ações) contou 77 registros a menos. A causa era o próprio teste: um contexto com prazo de 0 ms já nasce morto, e o `Acquire` recusa esse pedido sem registro, o que é o desenho. Troquei para prazos ≥ 1 ms. Ficou 1.800 pedidos = 1.800 registros em 3 rodadas com `-race`. O caso "contexto morto antes de entrar não gera registro" ficou como INFO (pendência 4).
- **(b)** O 1º teste de regressão do Q1 chamava `Release` no fim, e o `Release` antigo já emitia o registro. Por isso **passava no código antigo**: a cópia do `315f996` mostrou isso. Tirei o `Release`, porque no fluxo real o proxy não recebe ticket, e agora o teste falha antes e passa depois.
- **(c)** O 1º teste de regressão de SSE travou a suíte por 400 s: o `defer srv.Close()` rodava antes do `t.Cleanup` que fechava os streams, e o `httptest.Server.Close` espera os pedidos ativos. Matei o processo e corrigi a ordem.
- **(d)** O 1º vetor O1 falhou em falso: o nome do caso "409 sem administrador" continha a palavra que o próprio teste procurava. Renomeei para "409 generico".
- **(e)** Um `Set-Content -Encoding utf8` do PowerShell 5.1 gravou BOM num arquivo de teste. Removi com `sed` e passei a editar só com Python ou com a ferramenta de edição.
- **(f)** O teste de varredura do limitador esperava 2 IPs depois da varredura. O certo são até 101: a varredura roda ao passar de 4.096, e os IPs que chegam depois ficam. O teste errava, não o código.

### 3.6 M8 sem "antes" executável

O vetor M8 conta os cálculos com `statsComputed()`, que só existe na correção. O "antes" é por inspeção: `monitor.go:491` (`315f996`) chamava `Store.Stats` a cada GET, sem cache. O custo de cada chamada está medido em §2 (janela 7d: 71 ms / 4 MiB com 16 mil linhas; com a janela cheia de 200 mil linhas, ~300 ms / 49 MiB).

## 4. Achados

| Id | Sev. | Vetor (antes → depois) | Correção | Regressão |
| --- | --- | --- | --- | --- |
| **M1** | Média | `monitor.go:587`: 40 logins paralelos errados do mesmo IP → **40 PBKDF2** (`401:40`) → **5** (`401:5 429:35`) | Reserva atômica `limiter.tryBegin` sob lock, antes do PBKDF2; a tentativa conta enquanto roda; `end` registra o resultado (sucesso limpa o IP); `abort` devolve a reserva sem contar. Semáforo de **2** `Verify` simultâneos (espera até 10 s; depois 503 + `Retry-After: 5`) | `TestRegressLoginReservationIsAtomic`, `FuzzLoginLimiter` |
| **M2** | Média | `admin.go:226`: 40 falhas de 40 IPs → administrador limpo recebe **429** → **200** | **Teto global retirado.** Ficam 5 tentativas por IP e **backoff exponencial por IP**: 15 min, 30, 60… até 24 h; um dia sem atividade zera os bloqueios; mapa limitado (varredura acima de 4.096 IPs). Não foi preciso `--admin-ips`, porque não há teto global | `TestRegressNoGlobalLockout`, `TestRegressLoginBackoff`, `TestRegressLimiterSweeps` |
| **M3** | Média | `monitor.go:301`: 1 IP abre 20 streams, outro IP recebe **503** → 1 IP limitado a **4**, outro recebe **200** | `MaxSSEPerIP` (padrão 4) além do teto global (padrão agora 32), contados sob mutex pelo IP do cliente | `TestRegressSSEPerIP` |
| **P6** | Média | `proxy.go:340`: stream sem fim → **preso após 3 s** (e o pedido seguinte só saía por 504 de espera na fila) → corte em **~300 ms** (`MaxWait` do teste): o stream recebe a linha final `{"error":"tempo máximo de execução no servidor esgotado"}`; um Ollama que nunca manda headers → **504** | `--run-timeout` (env `MCPGW_RUN_TIMEOUT`), **padrão = `--max-wait` (10 min)**; `context.WithTimeoutCause(tk.Ctx, …, ErrRunTimeout)`; registro `erro/run_timeout`; o slot volta. Justificativa no §4.1 | `TestRegressRunTimeout`, `TestRegressRunTimeoutDefaultsToMaxWait`, `TestRegressRunTimeoutAndRowCapFlags` |
| **S2** | Média | `store.go:621`: 3.150.000 linhas recentes → `Maintain` → **3.150.000** → **3.000.000** | `--max-request-rows` (env `MCPGW_MAX_REQUEST_ROWS`, padrão **3.000.000**, mínimo 1.000); `Store.Cap` apaga por `rowid` as mais antigas acima do teto **a cada `Maintain` (de hora em hora)**, com o evento `teto_pedidos` | `TestRegressRequestsCap` |
| **P2** | Baixa | `proxy.go:309`: 10 endereços do mesmo /64 com teto por IP 2 → **10** em espera → **2** em espera e 8×503 | `fairIP`: IPv6 agrupado por /64 nos limites e na justiça (o IPv4, inclusive o mapeado, fica igual). A prioridade `alta` continua avaliada no IP real | `TestRegressFairIP`, `TestRegressIPv6RotationCountsAsOneClient`, `FuzzFairIP` |
| **C1** | Baixa | `config.go:245`: `--priority-ips ::ffff:10.0.0.5` aceito e **não casa nem com 10.0.0.5** → casa com 10.0.0.5 | `parseNets` com `net/netip`: o IP mapeado vira o IPv4; o prefixo mapeado vira o prefixo IPv4 (`/104` → `/8`); prefixo mapeado < /96 e zona são recusados | `TestRegressParseNetsMapped`, `FuzzParseNets` |
| **Q1** | Baixa | `queue.go:445`: `Acquire` acorda pelo `ctx.Done` no instante da partida e o administrador cancela antes do `abandon` → **0 registros** → 1 `cancelado_admin` | `abandon` emite o registro uma vez quando encontra `adminCx && !recorded` | `TestRegressAbandonAfterAdminCancelRecordsOnce` |
| **S1** | Baixa | `store.go:511`: `SCAN requests` a cada `Stats` (48 ms com 207 mil linhas, 2 vezes a cada 2 s com painel aberto); 200 mil linhas truncadas sem aviso → `SEARCH … idx_requests_fin` (0,56 ms) | Índice `requests(finished_at)`; `ORDER BY finished_at DESC` (o truncamento guarda as mais novas); campo `truncated` | `TestRegressStatsUsesIndex` |
| **S3** | Baixa | `store.go:321`: 20 lotes seguidos com descarte → **20** eventos `registros_descartados` (a tabela `events` nunca expira) → **1** | No máximo 1 evento por minuto; a contagem acumula e é gravada no próximo relato ou no `Close` | `TestRegressDropEventsCoalesced` |
| **M8** | Baixa | `monitor.go:491`: `GET /stats` aberto e sem cache (por inspeção, §3.6) → 30 GET paralelos = **1** cálculo | Cache de 5 s por janela + mutex: N pedidos paralelos custam 1 cálculo | `TestRegressStatsCached` |
| **C5** | Baixa | `main.go:183`: com 1 painel SSE aberto, o SIGTERM **não encerra em 10 s** (espera a tolerância de 30 s) → **6 ms** | `monitor.Handler.Close` fecha os streams; `srv.RegisterOnShutdown(mon.Close)` | vetor C5 (fica no `.testes`, porque sobe o `run` inteiro) |
| H1 | INFO (endurecimento) | `admin.go:108`: `ConstantTimeCompare` do usuário retorna antes quando os tamanhos diferem; diferença de ns diante de PBKDF2 de centenas de ms, não mensurável | Compara os SHA-256 do usuário (mesmo tamanho) | coberto pelos testes de login existentes |

### Vetores sem achado (OK)

| Id | Sev. do alvo | Resultado |
| --- | --- | --- |
| P1 `X-Real-IP` | Alta | só vale de par loopback e com 1 valor; forjado de fora, duplicado, lista e zona caem no IP do par; `::ffff:` normalizado; `::1` e loopback mapeado aceitos |
| P3 smuggling | Alta | CL+CL, CL negativo e CL/TE com espaço → 400; TE `chunked, identity` e TE duplicado → 501; CL+TE → o Go usa o chunked, e o `/api/pull` contrabandeado vira um 2º pedido do mesmo cliente → **403**. O Ollama só viu `/api/generate` com o corpo exato. O gateway reserializa o corpo (`ContentLength` fixo, sem TE), então não há dessincronia com o Ollama |
| P4 rotas bloqueadas | Alta | `/api/pull/`, `/api/%70ull`, `/api/create%2F`, `/api/blobs/…`, `/api/copy?x=1`, `OPTIONS`/`HEAD` → 403; `//api/pull`, `/API/pull`, `/api/pull%00` → 404; `/v1/../api/pull`, `/./api/push`, `/api/tags/../pull` → 307 do mux para o caminho limpo (que dá 403); 0 de 17 chegou ao Ollama |
| P5 contadores | Baixa | `"done":true` e contadores falsos no texto, `{"error"` no texto e argumentos de `tool_call` com chave `eval_count`: nenhum engana; só a última linha conta |
| P7 vazamento de slot | Alta | Ollama que fecha antes dos headers (502, `upstream_unreachable`), no meio do corpo (`upstream_dropped`) e cliente que desiste no escalonamento (`cancelado`): 0 slots presos |
| Q2 inanição | Média | `baixa` envelhecido começa apesar de 8 goroutines mandando `normal` sem parar de outro IP |
| Q3 escalonamento | Média | quem desiste nos 300 ms sai da fila; o timer dispara sem nada para começar; slots 1 → 0 |
| Q4 corrida | Alta | 12 goroutines × 150 pedidos com promote/demote/remove/cancel aleatórios e `-race`: 1.800 pedidos = 1.800 registros, 0 slots presos, invariantes ok (3 rodadas) |
| S4 SQLi | Alta | `' OR 1=1 --`, `UNION`, `DROP` nos filtros: 0 linhas, tabela intacta (tudo parametrizado) |
| S5 sentinela nos erros | Alta | 500 do Ollama ecoando o prompt, linha de erro no stream e 400 com o prompt: 0 ocorrências em `.db`/`-wal`/`-shm`; `error_class` ∈ {`upstream_500`, `stream_error`, `upstream_400`} |
| M4 XSS | Alta | label, tool, modelo, endpoint e `error_class` com `</script><img onerror>`: JSON escapa `<`, `>` e `&`; `nosniff`; página só com `textContent`; CSP `default-src 'none'` com hash |
| M5 CSRF | Alta | sem token, token errado, `Origin: null`, subdomínio, sufixo (`…br.evil.com`), `Sec-Fetch-Site: same-site/cross-site` → 403; `text/plain` → 415; sem cookie → 401; `Origin` ausente com tudo certo → segue (409 do id inexistente) |
| M6 sessão | Alta | o cookie plantado não vira sessão; o login sempre gera token novo de 256 bits; `HttpOnly; Secure; SameSite=Strict` |
| M7 exposição | Alta | nenhuma rota GET anônima devolve cookie, CSRF ou hash |
| C2 CIDR malformado | Média | 9 formas recusadas; a partida falha |
| C3 `hash-password` | Média | a senha só entra pelo stdin e não sai em stdout/stderr (sucesso e 2 erros). Risco residual: o terminal ecoa o que se digita (pendência 1) |
| C4 ponta a ponta | Alta | com `--debug`: o `Authorization` não chega ao Ollama; token, prompt e resposta ausentes do log, de todas as rotas `/monitor` e do `.db`/`-wal` |
| O1 nova tentativa | Média | 503 com `Retry-After` inválido (`0`, `-3`, data HTTP, `1.5`), 429, 502, 409 genérico e 409 do administrador → 1 tentativa; 503 + `Retry-After: 1` sempre → exatamente 2. O `TOOLS.md` e as descrições não mudaram (`git diff ced50f8..HEAD -- TOOLS.md` vazio; `TestToolsListBudget*` e `TestCatalogInSync` PASS) |

### 4.1 `--run-timeout`: por que o padrão é o `--max-wait`

- **Simetria:** um pedido não deve segurar um slot por mais tempo do que qualquer um pode esperar por ele. Com os dois em 10 min, quem espera nunca é vencido por um pedido preso.
- **Folga para o uso legítimo:** o `--timeout` do mcpollama é de 120 s, e o `ia-local` tem 900 s para a sessão inteira, que faz várias chamadas. As extrações medidas na Fase 6 levaram até ~87 s.
- **O limite físico:** um contexto de 65.536 tokens gerados a ~97 tok/s leva ~11 min. Ou seja, só uma geração patológica do contexto inteiro passaria de 10 min, e é exatamente essa que se quer cortar.
- **É flag independente:** se a 9.7 mostrar algum uso legítimo acima disso, sobe-se só ele.
- **Comportamento do corte:**
  - antes dos headers, o cliente recebe 504 com mensagem clara;
  - no meio do stream, recebe a linha NDJSON de erro, que o `streamNDJSON` do mcpollama mostra como "o servidor reportou erro durante a geração";
  - o registro fica `erro/run_timeout` e o slot volta na hora.

## 5. Gate final (dentro de `src/` do worktree, árvore sem os testes de ataque)

Logs em `…fase9.testes\gate-*.log`. A árvore é a dos commits de correção, sem os arquivos de ataque.

| Item | Resultado |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `gofmt -l .` | vazio |
| `go test -count=1 ./...` | **16 pacotes `ok`**, 1 sem testes (`mcp`); gateway: cmd 5,7 s, monitor 1,1 s, proxy 1,2 s, queue 1,9 s, store 0,5 s (`gate-test.log`, 00:06:20–00:06:40) |
| `go test -race -count=1 ./...` (gcc de `deps\`) | **16 pacotes `ok`**, exit 0; doctext 127,0 s, ocr 14,1 s, cmd 6,6 s, monitor 4,5 s, store 4,1 s, queue 3,1 s, proxy 2,3 s (`gate-race.log`) |
| OCR com a DLL real (`-tags ocr`, `libtesseract64.dll` SHA-256 `964B0972…E71669` conferido, `tessdataest`) | `ocr` 36,4 s, `doctext` 17,1 s, `tools` 3,6 s, `mcpollama-mcp` 13,2 s, todos `ok`; **385 PASS, 0 SKIP, 0 FAIL** (`gate-ocr.log`). O `golang.org/x/sys` v0.49.0 não quebrou nada no OCR do Windows |
| Build Linux `GOOS=linux CGO_ENABLED=0 go build ./cmd/mcpollama-gateway` | exit 0, 18.077.009 bytes; `go vet` Linux de `./gateway/...` e `./cmd/mcpollama-gateway` exit 0 |
| `go mod tidy` | **sem diff** em `go.mod`/`go.sum`. O `modernc.org/sqlite` já está no bloco direto; o aviso do editor estava desatualizado |
| `TOOLS.md` e descrições das tools | sem mudança (`git diff ced50f8..HEAD -- TOOLS.md` vazio; `TestToolsListBudget`, `TestToolsListBudgetWithFileTools` e `TestCatalogInSync` PASS) |

### Fuzz de 10 min por alvo (5 alvos em paralelo, `-parallel 3` cada)

De 2026-10-28 23:55:32 a 2026-10-29 00:05:36, todos `PASS` (`…fase9.testesuzz\*.log`):

| Alvo | Pacote | Execuções | Corpus novo | O que verifica |
| --- | --- | --- | --- | --- |
| `FuzzConcurrentActions` (existente, 9.1) | `gateway/queue` | 213.938 | 86 (total 236) | ações aleatórias de 4 goroutines; invariantes durante a corrida e dreno no fim; já com a correção Q1 |
| `FuzzFairIP` (novo) | `gateway/proxy` | 6.972.864 | 14 (total 92) | nunca entra em pânico; IPv4 e mapeado preservados; todo endereço do mesmo /64 dá a mesma chave |
| `FuzzCountersFromModelText` (novo) | `gateway/proxy` | 3.282.757 | 50 (total 133) | qualquer texto do modelo, codificado como o Ollama faz, em stream e não stream, com o corte de 16 KiB: os contadores vêm só do último chunk e nunca aparece `errorLine` falso |
| `FuzzParseNets` (novo) | `cmd/mcpollama-gateway` | 3.729.010 | 153 (total 334) | nunca entra em pânico; cada entrada aceita contém exatamente o que o `netip.Prefix` equivalente contém, com o mapeado lido como IPv4 |
| `FuzzLoginLimiter` (novo) | `gateway/monitor` | 3.165.797 | 16 (total 99) | roteiros de `tryBegin`/`end`/`abort` com relógio andando: reservadas + falhas na janela ≤ 5 por IP, `pending` ≥ 0, sucesso limpa, `retry` em (0, 24 h] |

## 6. Commits (branch `worktree-agent-a5589ffef1a3fdd1b`, sem push)

| Commit | Conteúdo |
| --- | --- |
| `3146afd` | **fix:** `/monitor`: M1, M2, M3, M8, o `Close` dos streams (C5) e o endurecimento H1 + `fase95_regress_test.go` e `FuzzLoginLimiter` |
| `8814d7a` | **fix:** P6 (`--run-timeout`), S2 (`--max-request-rows`), P2, C1, Q1, S1, S3 e a ligação do C5 no `main.go` + regressões e `FuzzFairIP`, `FuzzCountersFromModelText`, `FuzzParseNets` |
| (este) | **docs:** este report, a pasta `.testes` (ataques, logs, fuzz) e o changelog do plano |

**Verificação em árvore limpa** (lição da Fase 7): `git archive <commit> src TOOLS.md` de cada commit de correção para o scratchpad, e então `go build ./...`, `go vet ./...`, `gofmt -l` vazio e `go test -count=1` de `gateway/...`, `cmd/...`, `tools`, `ollama` e `mcpollama-mcp`: **tudo `ok` nos dois** (`gate-commits-limpos.log`).

**Erro de processo meu, corrigido:** a 1ª exportação levou só `src` e esqueceu o `TOOLS.md` da raiz. Com isso, `TestCatalogInSync` e `TestToolsMdMatchesTheRealToolsList` falharam por arquivo ausente, um artefato da exportação. Refiz com o `TOOLS.md` e tudo passou.

## 7. Pendências (numeradas)

1. **`hash-password` ecoa a senha no terminal** (lê o stdin sem desligar o eco; não há `golang.org/x/term` no módulo). Documentar na 9.6 o uso com `read -rs PW; printf '%s\n' "$PW" | mcpollama-gateway hash-password -user admin; unset PW`. Também não colocar a senha em `echo` (fica no histórico do shell).
2. **O gateway não policia `options` do corpo.** Um cliente com Bearer pode mandar `options.num_ctx` alto, `num_gpu` ou `keep_alive: 0`. Isso força recarga ou descarga do modelo e pode repetir o prefill gigante do incidente de PCIe de 23/10. Está fora do escopo pedido na 9.5. **Decisão do owner:** limitar `num_ctx` e recusar `keep_alive`/`num_gpu` no proxy, ou aceitar.
3. **O limite de login e o de SSE usam o IP completo, não o /64.** Hoje isso não importa: o `/monitor` só é servido à LAN IPv4 (`allow 10.100.0.0/16` no nginx). Se o `/monitor` passar a aceitar IPv6, usar o `fairIP`.
4. INFO: um pedido cujo cliente já desistiu **antes** de entrar na fila (contexto morto) não gera registro. É coerente (nunca entrou), mas o histórico não conta essas desistências instantâneas.
5. **Corpo lento depois dos headers:** o gateway não tem prazo de leitura do corpo (só `ReadHeaderTimeout` de 10 s) e depende do `proxy_request_buffering on` (padrão) do nginx. **Confirmar na 9.6** que nenhum `location` desliga o buffering.
6. **Um pedido sem `X-Real-IP` vindo do loopback conta como 127.0.0.1.** Não pôr `127.0.0.1` em `--priority-ips`, e confirmar na 9.6 que todo `location` do nginx que aponta ao gateway faz `proxy_set_header X-Real-IP $remote_addr`.
7. **Teto de 3 milhões de linhas e `--run-timeout` de 10 min:** valores de partida, a confirmar com o volume real na 9.7.
8. **Pendência 8 da 9.4 (encerramento lento), diagnosticada:**
   - Com painel aberto, o encerramento esperava 30 s (corrigido, C5).
   - Sem painel, o encerramento do `run` leva **5 ms** (medido isolado).
   - O pacote `cmd/mcpollama-gateway` leva **0,35–0,38 s** nas exportações limpas e **5,7–6,2 s** dentro do worktree, com o mesmo código.
   - Ou seja, os ~5 s são do ambiente de teste (caminho ou disco do worktree), não do gateway. Não vale perseguir.
9. **Pendências herdadas que continuam abertas** (não eram do escopo da 9.5):
   - **9.1:**
     - (1) `Retry-After` fixo em 5 s;
     - (2) a justiça não considera quantos pedidos o cliente já tem em execução.
   - **9.2:**
     - (2) `X-Queue-Position` é a posição na entrada;
     - (3) 502 sem `Retry-After` quando o Ollama reinicia;
     - (4) rotas `/v1/*` no nginx (9.6).
   - **9.3:**
     - (2) descarte por buffer cheio não provado sob carga real;
     - (3) `nvidia-smi` sob `ProtectSystem=strict` (9.6);
     - (4) ruído do AER ausente.
   - **9.4:**
     - (1) ponta a ponta mcpollama → gateway real (9.7);
     - (2) cookie `Secure` exige HTTPS;
     - (3) a página ainda não foi aberta num navegador real (9.7).
10. **Pendências da 9.4 resolvidas aqui:**
    - (4) SSE por IP (M3);
    - (5) `Stats`/`History` medidos (§2);
    - (7) `--run-timeout` (P6);
    - (9) OCR com `x/sys` v0.49 (§5).

    **Da 9.2:** (1) `--run-timeout` e (5) vetor de smuggling (P3). **Da 9.1:** (4) fuzz de 10 min. **Da 9.3:** (1) `Stats` medido.
