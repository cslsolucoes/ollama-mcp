# Revisão adversarial da etapa 10.5 (pool de backends da 10.4)

- **Projeto:** mcpollama (`E:\MCPOllam`), gateway `mcpollama-gateway` e cliente `mcpollama-mcp`
- **Alvo:** ramo `worktree-agent-a9532e3414358d795`, commits `5a77853`, `73a9f30`, `f7c5aed`, `54cc580` e `ed57add` (10.4), sobre o `master` `94a0785`. Arquivos: `src/gateway/pool/{registry,pool}.go`, `queue/{queue,poolstatus}.go`, `proxy/proxy.go`, `monitor/{backends,monitor}.go` e `index.html`, `store/backends.go`, `models/models.go`, `cmd/mcpollama-gateway/{main,config}.go`, `ollama/{api,client}.go`, `mcpollama-mcp/config.go`
- **Onde:** só no worktree `E:\MCPOllam\.claude\worktrees\agent-a9532e3414358d795`; sem push e sem merge
- **Revisor:** subagente adversarial, Opus 5.5 (verificação crítica de segurança do pool: um secundário é outra máquina da LAN, com token próprio, e recebe os documentos jurídicos dos clientes)
- **Data:** 2026-10-30, das 19:02 às 19:45 (hora local, `date`)
- **Ambiente:** Windows 11 IoT Enterprise 10.0.28000, go1.27.0 windows/amd64. Sem WSL: os testes de modo POSIX ficam para o Linux (pendência 1)
- **Restrições respeitadas:** sem servidor, sem SSH, sem `D:\MCP`, sem `.mcp.json` e sem tocar no repositório principal. Todo Ollama é `httptest` ou um socket cru local. Os binários Linux foram para o scratchpad

---

## 1. Veredito

**Nenhum achado Alto.** As promessas centrais da 10.4 aguentaram os ataques:

- o **token do cliente nunca chega a backend algum**: `authorization` em minúsculas, `Authorization` duplicado, `Proxy-Authorization`, `Cookie` e até o `Authorization` mandado como **trailer** de um corpo chunked, em `/api/generate`, `/api/chat` e `/v1/chat/completions`, no primário e no secundário (T3);
- o **token do secundário** não aparece no log `--debug`, nas 5 rotas abertas do monitor, no SSE, no export (aberto e do admin), no `gw.db`, no `content.db` nem nos `-wal` (T1, M2), e **nunca vai para o primário** (T3);
- **nenhum padrão que o cliente controla abre o disjuntor**: 500, 404 de modelo, `run_timeout`, `keep_alive:0` com `num_gpu`/`main_gpu`/`low_vram` (removidos pelo proxy) e corte do cliente, 25 pedidos hostis, e o primário segue fechado e `ok` (B2). A correção `54cc580` da 10.4 se confirma;
- um pedido **em execução nunca muda de backend** (R1), `alta` **nunca vai para a cpu** mesmo com a classe listada no arquivo (R2), a justiça da fila única vale no pool (R4) e não há inanição do primário pela histerese (B4);
- `drain`/`undrain` exigem sessão, CSRF e mesma origem; 9 ids maliciosos (`<script>`, `%00`, CRLF, `..%2F`, 33 caracteres) não drenam nada (M1);
- o fallback do mcpollama não tem laço, respeita o timeout total e separa os tokens nos dois sentidos (FB2–FB4).

Há **2 achados Médios** e **6 Baixos**. **Todos foram corrigidos** no commit **`b1d8418`**, cada um com teste de regressão no pacote (`fase105_regress_test.go`), e as regressões **falham no `ed57add`** (`regressao-no-codigo-antigo.log`).

| # | Sev. | Achado | Prova antes | Correção |
| --- | --- | --- | --- | --- |
| **B1** | **Média** | O disjuntor de um **secundário** abria (3 checagens falhas, p. ex. 15 s de rede fora) e **nunca mais fechava**: depois do tempo aberto ele fica meio-aberto, um secundário meio-aberto não é elegível (`unhealthy()` inclui `br != brClosed`) e, sem pedido, a prova nunca chega. O secundário saía do pool **até reiniciar o gateway**, e o quadro mostrava "disjuntor aberto" para sempre | vetor B1: backend de volta, S2 ligado, nenhum pedido em 1,5 s (`estado=disjuntor_aberto disjuntor=meio_aberto`) | a **checagem ativa** que dá certo **depois** do tempo aberto fecha o disjuntor (ela também é uma prova: chegou a `/api/ps`, `/api/tags` e `/api/version`). O tempo aberto é respeitado (teste) |
| **N1** | **Média** (não regressão sem `--backends`) | Com o Ollama fora, o **quadro "Modelos"** ficava parado no último retrato bom (modelo "carregado" com o Ollama morto) e o evento `modelo_descarregado` não saía. Antes da 10.4 o `RunLoop` do coletor gravava o erro a cada 5 s; a 10.4 passou o coletor para o pool, e o `CheckOnce` parava no `/api/version` | vetor N1: 3 s com o Ollama fora, `1 carregado, erro=""`; no `94a0785` o mesmo teste mostra o erro (`TestOld94…`, 5 s) | o `CheckOnce` lê **sempre** `/api/ps` e `/api/tags` (como o `RunLoop`) e só depois `/api/version` |
| B3 | Baixa | Erro do proxy **depois** de a conexão ser feita (o backend fecha antes dos cabeçalhos) virava `upstream_unreachable` e **contava** no disjuntor. Um corpo de pedido que derrube a conexão (um defeito do Ollama) deixaria **um cliente abrir o disjuntor do primário com 3 pedidos** (e, no código antigo, o 4º pedido já ia para o secundário) | vetor B3: `DERRUBA` ×3 → `aberto` | `httptrace.GotConn` marca a troca; o erro depois da conexão é **neutro** (`Result.AfterConnect`). A classe do registro segue `upstream_unreachable` (sem mudança no histórico); a recusa de conexão ×3 continua abrindo |
| S2 | Baixa | Um secundário que responde **3xx** e `Set-Cookie`: o `ReverseProxy` não segue (bom), mas **repassava** o 3xx e o cookie ao cliente. Um SDK que segue redirect mandaria o prompt para o `Location`, e o cookie cairia na origem do monitor | vetor S2: cliente recebe `307 Location=http://127.0.0.1:…/roubar` e `Set-Cookie: mcpgw_session=forjado; Path=/monitor` | só para secundários: 3xx vira **502 `upstream_redirect`** (neutro para o disjuntor) e o `Set-Cookie` é removido. O primário segue como antes. A checagem ativa já não seguia redirect (`CheckRedirect`), confirmado |
| FB1 | Baixa | O fallback do mcpollama disparava em **qualquer** `ErrConnect`, inclusive quando o primário **já tinha lido o pedido** (resposta malformada; no HTTP/2, `GOAWAY` ou reset de stream). O mesmo trabalho rodava **duas vezes** | vetor FB1: primário leu 1, fallback rodou 1 | o fallback só dispara em erro de **dial/proxyconnect** (`neverSent`): o pedido não saiu. Recusa de conexão continua indo ao fallback (teste) |
| S1 | Baixa | `url.Parse` só confere que a porta é feita de dígitos: `:0`, `:65536`, `:99999` e `?` vazio eram aceitos no `backends.json` (backend "fora" para sempre, sem erro claro na partida) | vetor S1: 3 URLs aceitas | porta 1..65535 e `ForceQuery` recusados, com mensagem |
| S4 | Baixa | `checkPerm` (`os.Stat`) e a leitura (`os.Open`) eram **duas** resoluções do nome: um `backends.json` ou `token_file` trocado (rename, symlink) entre as duas era lido sem checagem (TOCTOU). Exige escrever na pasta, mas é barato fechar | por inspeção (a corrida não é determinística sem gancho) | `readSecret`: checa pelo nome (um FIFO nunca trava o `open`), abre, confere `os.SameFile`, "arquivo comum" e modo **no descritor**. O teste Linux de modo/symlink está pronto (pendência 1) |
| T2 | Baixa | Texto livre do secundário ia **cru** ao estado **aberto** do monitor: `version` (do `/api/version`), nome do modelo e processador (`/api/ps`), e o "último erro" com o nome do modelo. Um secundário que **ecoe o `Authorization`** (servidor de depuração, proxy mal configurado) poria o **token** no `/monitor/api/state`; e um nome de 5 KB ia inteiro | vetor T2: token no quadro, `version` 54 B, modelo 5.025 B | `Backend.scrub` troca o token próprio por `[redigido]` e corta (modelo 96, processador 32, erro 160); `version` só com `[0-9A-Za-z.+_-]`, até 32 |

## 2. Números

| Execução | Vetores (`VEC`) | OK | FALHA | Arquivo |
| --- | --- | --- | --- | --- |
| `ed57add` + vetores 10.5 | 25 | 14 OK + 3 MEDIDA + 1 ACEITO | **7** (B1, B3, S1, S2, T2, FB1, N1) | `…pool.testes\antes.log` |
| código corrigido + vetores 10.5 | 26 (S4 novo) | 21 OK + 3 MEDIDA + 1 ACEITO + 1 N/A (S4 no Windows) | **0** | `…pool.testes\depois.log` |
| regressões 10.5 no `ed57add` (`git archive`) | — | — | **7 de 7 falham por asserção** (pool 5, ollama 1, cmd 1) | `…pool.testes\regressao-no-codigo-antigo.log` |
| N1 no `94a0785` (antes da 10.4) | — | o quadro mostra o erro | — | idem (prova de que é regressão da 10.4) |
| **ataques 9.5 + 10.L + 10.5** no código corrigido, `-short` | **75** (9.5: 29; 10.L: 20; 10.5: 26) | **75** | 0 | `…pool.testes\ataques-short.log` |
| **ataques 9.5 + 10.L + 10.5**, completo (inclui o S2 da 9.5) | **79** (9.5: 30; 10.L: 23, com o **A3 atualizado**; 10.5: 26) | **79** | 0 | `…pool.testes\ataques-completo.log` |
| `-race` nos vetores da 10.5 | — | 4 pacotes ok | 0 (depois de corrigir a corrida do **meu** vetor R3, ver 3.1) | `…pool.testes\race-vetores-10-5.log` |

Contagem: linhas `VEC|` por arquivo de ataque (`fase9_*` 30, `fase10l_` 23, `fase105_` 26). Os logs têm ainda **18 linhas `VEC|` da suíte normal do `tools`** (`filetools_docs_test.go`, Fase 7, todas `BLOQUEADO`), que não são ataques desta rodada; por isso o total bruto dos logs é 93 e 97. O 10.L tem 20 no `-short` e 23 no completo; a 10.5 não usa `-short`. Os pulados são os de Linux: `TestAdvLPermissions`, `TestAdvLKillChild`, `TestAdv5S4FileModesLinux` e os já pulados da suíte (`TestRegistryWrongModeIsRefused`, RAPL).

### Medidas (MEDIDA / ACEITO)

| Vetor | Resultado |
| --- | --- |
| **B5** flapping | 40 checagens alternando falha/ok: disjuntor **fechado** (nunca 3 seguidas) e **40 eventos** `backend_down`/`backend_up` (1 por troca). A 5 s isso daria até **17.280 eventos/dia por backend** oscilando (pendência 5) |
| **B6** custo da checagem | **3 GET por rodada** (`ps`, `tags`, `version`). No primário isso é **+1 GET** (o `/api/version`) sobre o que o coletor de modelos já fazia a cada 5 s desde a Fase 9. Backend **mudo**: a rodada leva **2 × timeout** (ps e tags; 401 ms com 200 ms de timeout no teste; **6 s** com os 3 s de produção), e o tique vencido é descartado (sem empilhar). O custo real com o Ollama ocupado fica para a 10.7 (pendência 4) |
| **R5** estimativa de tokens | corpo de 3.032 B → estimativa **1.011** tokens; os 3.000 dígitos podem dar **3.000** tokens (o Qwen faz 1 token por dígito): **3,0×**. Um `max_prompt_tokens=1000` da cpu aceita o pedido. Efeito: prefill mais longo naquele backend, limitado pelo `--run-timeout`; a cota segura é bytes ÷ 1 (decisão do owner, pendência 2) |
| **R6** digest forjado | **ACEITO (modelo de ameaça):** o digest vem do `/api/tags` do **próprio** secundário. Um secundário mal-intencionado declara o digest do primário e serve outro modelo, e recebe o repasse (HTTP 200). Isso **não** é uma fronteira de segurança: o secundário está no `backends.json` do root (0600) e **já recebe o prompt**; quem controla o secundário controla a resposta, com ou sem digest. O digest protege contra **engano de configuração** (modelo diferente com o mesmo nome). A proteção real é confiar no host, com TLS e token próprio |
| **R3** corrida drenar × escolha | 3.194 pedidos em 4 goroutines (546 sob `-race`): **0** inícios no drenado **depois** de `SetDrained` + `Poke` (o que o monitor faz). Um início decidido **antes** do drenar ainda pode cair nele (o `SetDrained` só pega o lock do pool, a escolha está sob o lock da fila): é o mesmo que ter começado um instante antes, e "o que está rodando termina" já cobre |

## 3. Método

1. Li o `CLAUDE.md`, a entrada 10.4 e as pendências do plano da Fase 10, a §11.9 do plano da Fase 9, o `git diff 94a0785..HEAD` inteiro (37 arquivos) e o método dos reports 9.5 e 10.L.
2. Escrevi **um arquivo de ataque por pacote** (`fase105_adv_test.go`): `gateway/pool` (18 vetores, com o **proxy de verdade** e Ollamas falsos próprios, porque o `fake` da 10.4 não deixava trocar o handler), `gateway/monitor` (2), `ollama` (4) e `cmd/mcpollama-gateway` (2, o gateway inteiro em processo). Cada um imprime `VEC|id|sev|arquivo:linha|resultado` e falha quando há defeito.
   - **T3** usa **TCP cru** para o cabeçalho chegar como foi escrito (minúsculas, duplicado) e para mandar o `Authorization` como **trailer** de um corpo chunked; o Ollama falso grava cabeçalhos **e** trailers.
   - **B3** e **FB1** usam um backend que lê o pedido inteiro e então fecha a conexão ou responde lixo (`ISTO-NAO-E-HTTP`): é a prova de que o servidor **viu** o pedido.
   - **T1** procura os dois tokens em 8 fontes, inclusive os **bytes** do `gw.db`, `content.db` e `-wal`; confere que o `gw.db` tem linhas do secundário (7 ocorrências do id), senão o vetor não teria exercitado nada.
3. Rodei no `ed57add` (`antes.log`), corrigi, escrevi as **regressões no pacote** e rodei de novo (`depois.log`).
4. Prova de que as regressões falham antes: `git archive ed57add src` no scratchpad, com as regressões copiadas por cima; e `git archive 94a0785` para provar que o N1 é regressão (o código antigo mostrava o erro).
5. Recoloquei os **7 arquivos de ataque da 9.5**, os **4 da 10.L** (com o **A3-ids-csrf atualizado**) e os **4 da 10.5** em `src`, rodei com e sem `-short`, rodei os da 10.5 com `-race` e **removi todos de `src`**. Os da 10.5 ficam em `…pool.testes\<pacote>\`; para rodá-los de novo, copie-os de volta (o `pool` usa o `fake`/`newRig` do `fake_test.go`; o `monitor` usa o `newFixture` do `monitor_test.go`; o `cmd` usa o `startGateway` do `pool_main_test.go`).

### 3.1 Erros de processo meus (corrigidos antes dos números finais)

- **(a)** A 1ª rodada do **B2** acusou "placement chegou ao Ollama" 5 vezes. O meu rig montava o proxy **sem** `StripOptions` (o `main.go` passa `proxy.DefaultStripOptions`; o `proxy.New` não tem padrão). Falso positivo meu; o rig passou a usar a lista do `main.go`.
- **(b)** O 1º **S3** parou num `t.Fatal` sem `VEC`: eu pus `token_file` com `http://10.0.0.5`, que o próprio registro recusa (token só com https ou loopback). Troquei para `https://`.
- **(c)** A 1ª regressão do **B1** presumia que o pedido `b` iria ao secundário; a justiça escolhe `b` **ou** `c` (o mesmo erro que a 10.4 registrou). Passei a aceitar qualquer um dos dois.
- **(d)** O vetor **R3**, sob `-race`, acusou **corrida no próprio teste** (`len(started)` lido fora do mutex para montar o IP). Não era defeito do gateway; corrigi o vetor (contador por goroutine) e o `-race` passou.
- **(e)** A 1ª contagem de CR no `index.html` deu **583** (= o número de linhas): erro meu de aspas no shell (o padrão do `grep` virou vazio). Refeita com `tr -cd '\r' | wc -c`: **0**. O `index.html` não foi alterado nesta revisão.

## 4. O que foi atacado e aguentou (sem correção)

| Superfície | Vetor | Resultado |
| --- | --- | --- |
| 1. SSRF | **S1** (21 URLs): `user:pw@`, `file:`, `unix:`, `gopher:`, `//host`, `http:/host`, `%31%30…`, CRLF, link-local IPv4/IPv6/mapeado, `0.0.0.0`, caminho, query, fragmento | recusados (depois da correção da porta) |
| 1. SSRF | redirect na **checagem ativa** (302 para outro host) | não segue (`CheckRedirect`), destino com 0 pedidos, backend `fora` |
| 1. SSRF | **S3** arquivo e `token_file` trocados depois da partida | sem efeito até reiniciar (lidos uma vez) |
| 2. Token | **T1**, **T3**, **M2** | ver o veredito |
| 3. Disjuntor | **B2** (25 pedidos hostis), **B4** (histerese), **B5** (flapping) | fechado; primário livre leva o trabalho sob `hist`; flapping não abre |
| 4. Repasse | **R1**, **R2**, **R3**, **R4** | ver o veredito e as medidas |
| 5. Monitor | **M1**, **M2** (`index.html` sem `innerHTML`/`outerHTML`/`insertAdjacentHTML`/`document.write`, 0 CR) | ok |
| 6. Fallback | **FB2** (400, 401, 403, 404, 409, 413, 429, 500, 502, 503 sem `Retry-After`, 504, erro no stream, timeout, corte no meio → 0 pedidos ao fallback), **FB3** (503+503: primário 2, fallback 1, sem laço; fallback sem token não recebe `Authorization`), **FB4** (primário fora + fallback mudo → 300 ms com `--timeout 300ms`) | ok |
| 7. Não regressão | sem `--backends`: **N1** (corrigido); ataques da **10.L** (log de jobs, 23 vetores) e da **9.5** (31) | ok |

### 4.1 Divergências do desenho (§11.9), não defeitos

Registro para o owner decidir na 10.7; nenhuma é falha de segurança:

1. **S3 "modelo não residente"** e **"3 erros seguidos (5xx, conexão, timeout)"** da §11.9 **não** são seguidos: a 10.4 decidiu que "não residente" não é falha (carga a frio é normal) e a revisão de segurança (`54cc580`) tornou 5xx e timeout **neutros** para que um cliente não abra o disjuntor. Esta revisão confirma que a escolha é a certa (B2, B3).
2. **F2 "ou com espera estimada menor que a do primário"**: o código só repassa com **slot livre** no secundário (mais conservador).
3. **F7 "o mcpollama refaz 1 vez"** quando o backend falha **no meio**: o fallback só cobre conexão recusada e 503 de fila cheia (antes de o pedido sair). Refazer no meio duplicaria trabalho; a FB1 desta revisão vai no mesmo sentido.
4. O campo **`ctx`** do registro e o **F4 com modelo diferente** (D9-B2) não existem.
5. A aderência às regras **R1–R9 da faixa CPU** (§11.4) depende das decisões **D9-C1..C5** da 10.3, ainda pendentes; hoje a cpu só recebe trabalho com o primário sobrecarregado.

## 5. Gate (em `src/` do worktree, commit `b1d8418`)

| Item | Resultado | Arquivo |
| --- | --- | --- |
| `go build ./...`, `go vet ./...` | ok | `gate-test.log` |
| `gofmt -l .` | vazio | `gate-test.log` |
| `go test -count=1 ./...` | 21 pacotes ok | `gate-test.log` |
| `-race` (`CGO_ENABLED=1`, gcc de `E:\MCPOllam\deps\gcc-mingw-w64\bin`) em `./gateway/... ./cmd/mcpollama-gateway ./ollama ./tools ./mcpollama-mcp` | 14 pacotes ok | `gate-race.log` |
| build Linux (`GOOS=linux CGO_ENABLED=0`) do gateway e do `mcpollama-mcp`, `go vet` Linux do gateway | ok (binários no scratchpad) | `gate-linux-budget.log` |
| CR no `index.html` | **0** | `gate-linux-budget.log` |
| `TestToolsListBudget`, `TestToolsListBudgetWithFileTools`, `TestCatalogInSync` | PASS (descrições das tools e `TOOLS.md` intactos) | `gate-linux-budget.log` |
| `FuzzRouting` **10 min** (`./gateway/queue`, commit `b1d8418`, 19:30:42–19:40:46) | **PASS**: 775.006 execuções, 422 entradas no corpus (94 novas), 0 falha | `fuzz-routing-10min.log` |
| ataques 9.5 + 10.L + 10.5 | 75/75 (`-short`) e 79/79 (completo), 0 FALHA | `ataques-*.log` |

## 6. Commits

| Commit | Conteúdo |
| --- | --- |
| `b1d8418` | `fix: Fase 10.5 - …`: `pool.go`, `registry.go`, `proxy.go`, `queue.go` (`Result.AfterConnect`), `ollama/api.go` e as 3 regressões |
| (docs) | este report e a pasta `.testes`, o A3 da 10.L atualizado, o MANUAL §16 e a tabela de erros, e o Changelog do plano da Fase 10 |

## 7. Pendências (numeradas)

1. **Linux (10.6/10.7):** rodar no servidor, como `mcpgw`, o `TestAdv5S4FileModesLinux` (0644, 0640, symlink para 0644, `token_file` 0644), o `TestRegistryWrongModeIsRefused` e os pulados da 10.L. O **dono** do `backends.json`/`token_file` não é conferido (igual ao `--admin-file`); decidir se a 10.6 grava `root:mcpgw 0640` + checagem de dono, ou `mcpgw:mcpgw 0600` como a senha (a regra atual exige 0600, então é a 2ª).
2. **Decisão do owner (R5):** `max_prompt_tokens` pela estimativa bytes ÷ 3 é manipulável (~3× com dígitos). Opções: (a) manter e documentar (é só tempo de CPU, limitado pelo `--run-timeout`); (b) bytes ÷ 1, a cota segura, que manda bem menos trabalho à cpu; (c) contar com o tokenizador (`/api/tokenize` não existe no Ollama 0.40; seria um `/api/embed`/`generate` com `num_predict:0`, que custa um prefill).
3. **Decisão do owner:** o monitor **aberto** mostra o `host:porta` de cada secundário (no título da linha e no "último erro"). Sem credencial, mas facilita atacar o secundário direto; dá para mostrar só com login.
4. **10.7:** medir com o Ollama ocupado o custo real da checagem (3 GET a cada 5 s por backend) e se o `/api/ps` atrasa durante uma carga a frio (se passar de 3 s três vezes seguidas, o disjuntor do **primário** abre durante uma carga normal).
5. **Eventos de flapping:** um backend oscilando gera 1 evento `backend_down`/`up` por troca (até 17.280/dia a 5 s). Opcional: limitar a 1 por minuto por backend, como o `teto_conteudo_falhou` da 10.L.
6. Quando o primário está **fora já na partida**, o digest dele é desconhecido e **nada** é elegível (F5: os pedidos esperam). É o comportamento seguro ("sem prova, sem repasse"), mas significa que o pool não ajuda num gateway que sobe com a GPU caída.
7. Herdadas da 10.4 e ainda abertas: 10.S (secundário 4× RTX 2060), backup antes da migração do `gateway.db`, S4 (energia) desligado, alternativa (b) do 5xx com 2 IPs.
