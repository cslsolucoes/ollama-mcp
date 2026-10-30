# Revisão adversarial da etapa 10.L (log de jobs com prompt e resposta completos)

- **Projeto:** mcpollama (`E:\MCPOllam`), gateway `mcpollama-gateway`
- **Alvo:** commit `6cbaf22` (Fase 10.L): `src/gateway/{content,joblog}`, `monitor/jobs.go`, `store/jobs.go`, `proxy/{journal,proxy}.go`, `cmd/mcpollama-gateway/main.go`
- **Onde:** `master` do repositório principal (sem worktree); o ramo `wip/fase10-4-pool` não foi tocado
- **Revisor:** subagente adversarial, Opus 5.5 (verificação crítica de segurança: o `content.db` guarda documentos jurídicos de clientes)
- **Data:** 2026-10-30, das 17:30 às 18:10 (hora local, `Get-Date`)
- **Ambiente:** Windows 11 IoT Enterprise 10.0.28000, go1.27.0 windows/amd64, NVMe local. Sem WSL (o vetor de permissões POSIX fica para o binário Linux, ver pendência 1)
- **Restrições respeitadas:** sem servidor, sem SSH, sem `D:\MCP`, sem `.mcp.json` e sem tocar no ramo `wip/fase10-4-pool`. Todo upstream é `httptest`. Os binários Linux foram para o scratchpad. Sem push.

---

## 1. Veredito

**Nenhum achado Alto.** As promessas centrais da 10.L aguentaram os ataques:

- o texto só sai pelo `/content` e pelo export com `conteudo=1`, e só com sessão: 29 variações de caminho e de método, sessão expirada, cookie de outro gateway, cookie após logout e cookie duplicado;
- o SSE, o `/state`, o `/jobs/{id}` e as mensagens de erro não carregam o texto;
- o `gw.db` e o `-wal` não têm o texto, e o log também não;
- um processo morto de verdade (`TerminateProcess`) no meio do stream deixa `interrompido_gateway` (mesma boot) ou `interrompido_reboot` (boot trocada), com o prompt e a resposta parcial;
- os ids são de 128 bits (`crypto/rand`) e a 10.L não criou nenhum POST.

Há **5 achados Médios** (dois vindos da revisão automática de segurança, pelo coordenador: H1 e D6) e **5 Baixos**. **Todos foram corrigidos**, nos commits `cd71ed8` e `998a70d` (D6), cada um com teste de regressão no pacote (`fase10l_regress_test.go`). O D6 é um defeito **da minha própria correção** do D4, apontado pelo coordenador.

| # | Sev. | Achado | Prova antes | Correção |
| --- | --- | --- | --- | --- |
| **H1** | **Média** (coordenador, revisão automática) | `GET /monitor/api/jobs/{id}` **sem login** devolvia `content.Info`, com o **`prompt_sha256`** e os tamanhos exatos (`prompt_bytes`, `resp_bytes`, `stored_bytes`). Quem tem um documento candidato calcula o SHA-256 e **confirma que ele foi enviado à IA** | vetor H1: o hash aparece em `/monitor/api/jobs/{id}`; regressão `TestRegress10LNoPromptHashWithoutSession` falha no `6cbaf22` | `content.Info` só vai com a sessão do admin **e** pedido do próprio site (`adminRead`). O hash não sai em nenhuma rota aberta e continua no `/content` e no `/jobs/{id}` do admin |
| **H2** | **Média** | Tamanhos exatos nas rotas abertas: `bytes_in` em `/state` (fila e recentes), `/history`, `/jobs`, `/jobs/{id}`, export JSONL/CSV; `resp_bytes` (novo na 10.L) idem. O tamanho exato estreita a confirmação | vetor H2: `"bytes_in":12345` em 5 rotas e as células `12345`/`4321` no CSV | sem sessão, `bytes_in` e `resp_bytes` saem **arredondados para cima em KiB** (`coarseBytes`), inclusive no snapshot da fila e no export sem conteúdo. O export com `conteudo=1` (admin) segue exato |
| **D4** | **Média** | O teto total (`--content-max-bytes`) apagava **um job por commit FULL**, **dentro** da gravação que passou do teto. Essa gravação é o prompt de um pedido já aceito ou o `Write` do stream, **com o slot da GPU preso** | 614 jobs apagados levaram **829 ms a 1,69 s** dentro do `SavePrompt` (NVMe local). No teto real (2 GiB, 10% = 200 MiB) e com jobs de 20 KB, seriam ~10 mil commits, dezenas de segundos de stream parado | o teto roda **em segundo plano**, uma rodada por vez, em **transações de 500 jobs** (1 fsync por lote). `Close` espera a rodada. Depois: **4 a 14 ms** |
| **D6** | **Média** (coordenador, revisão automática, sobre a minha correção do D4) | A goroutine do `triggerCap` (criada em `cd71ed8`) repetia `enforceCap` em `for {}` até `total <= teto`. Com o `enforceCap` falhando (erro de SQLite, disco cheio, banco travado ou fechado) **girava para sempre: 100% de uma CPU**, e o processo seguia batendo no banco | `TestRegress10LCapFailureDoesNotSpin` no `cd71ed8`: **presa 5 s** (o teste a solta à força) → `d6-antes.log` | `enforceCap` devolve `(removidos, erro)`. Com erro ou 0 removidos, o laço sai (`capBusy=false`), relê o total real, entra em **espera de 1 min** antes de novo disparo e grava **`teto_conteudo_falhou`** (no máximo 1 por minuto). Com progresso, faz no máximo **5 rodadas por disparo**, com backoff de 20 ms × tentativa que o `Close` corta. Depois: termina em < 1 ms, sem goroutine sobrando e com **1** evento para 1.001 disparos |
| **R2** | **Média** | `content.db` corrompido, ou caminho que não abre, faz o gateway **sair com 1 na partida**: com `Restart=` no systemd, isso vira um laço de reinício, e o gateway inteiro (fila e proxy) fica fora | vetor R2: `lixo` e `diretorio` → "o gateway saiu com 1 na partida" | o gateway **sobe sem gravar o texto**, loga, grava o evento **`conteudo_indisponivel`** com o erro e deixa o arquivo **intocado** para inspeção. O monitor mostra `content_enabled:false` |
| A2 | Baixa | `GET` do `/content` e do export com `conteudo=1` não conferia a origem (`Sec-Fetch-Site`/`Origin`). O cookie `SameSite=Strict` já impede o envio de outro site; faltava a 2ª camada que os POSTs têm | cross-site com cookie → 200 | `requireSession` chama `sameSite(r)`: cross-site, same-site ou `Origin` alheio → **403** |
| I1 | Baixa | `status=` com 40 mil itens passa do teto de variáveis do SQLite → **500** (custo baixo, mas é um erro de servidor provocável) | 500 em 13 ms | só os status conhecidos entram no filtro (sem repetição); lista só de desconhecidos não casa nada (como antes) |
| I2 | Baixa (rebaixada de Média depois de escrita) | CSV: célula que começa com **LF** não era neutralizada (a lista OWASP cita TAB e CR; o LF é endurecimento) | `"\n=1"` sai cru | `csvSafe` inclui `\n` |
| P1 | Baixa | `os.OpenFile(..., 0600)` não muda o modo de um `content.db` **que já existia** (ex.: 0644 de uma cópia ou restauração). O `-wal` e o `-shm` copiam o modo do principal | por inspeção (sem POSIX aqui) | `os.Chmod(path, 0600)` no `Open`. Teste Linux pronto, ver pendência 1 |
| M1 | Baixa | Memória: 1 pedido de 32 MiB alocava **+161 MiB** a mais com o journal (o `SavePrompt` copiava o prompt inteiro 2×, e o `extractPrompt` crescia o builder por duplicação) | 138 → 299 MiB alocados; o `SavePrompt` sozinho alocava **65 MiB** para guardar 16 MiB | hash em blocos de 64 KiB, cópia só da parte guardada e `Grow` no builder: **+113 MiB**; o `SavePrompt` ficou com < 28 MiB. O resto vem do `json.Unmarshal` do `extractPrompt` (pendência 3) |

## 2. Números

| Execução | Vetores (`VEC`) | OK | FALHA | Arquivo |
| --- | --- | --- | --- | --- |
| `6cbaf22` + vetores 10.L | 23 linhas (22 vetores; R2 tem 2 casos) | 13 OK + 3 MEDIDA + 1 N/A | **7 vetores** (H1, H2, A2, I1, I2, D4, R2×2) | `…10L.testes\antes.log` |
| código corrigido + vetores 10.L | 23 | 19 OK + 3 MEDIDA + 1 N/A | **0** | `…10L.testes\depois.log` |
| D6 no `cd71ed8` (regressão nova) | — | — | **1** (presa 5 s) | `…10L.testes\d6-antes.log` |
| vetores do `content` depois do D6 (D2, D3, D4, D5, P1, R1) | 6 | 4 OK + 1 MEDIDA + 1 N/A | 0 | `…10L.testes\depois-d6-content.log` |
| regressões no código antigo (`git archive 6cbaf22` no scratchpad) | — | — | monitor 5/5 e `jobs_test` atualizado **falham por asserção**; cmd 1/1 **falha por asserção**; content M1 **falha por asserção** (65 MiB); content D4/Close/P1 **não compilam** (usam `WaitCap`/`triggerCap`): a prova deles é o D4 do `antes.log` | `…10L.testes\regressao-no-codigo-antigo.log` |
| **ataques da 9.5** reexecutados no código corrigido, `-short` | 41 | **41** | 0 | `…10L.testes\ataques-9.5.log` |
| **ataques da 9.5** sem `-short` (inclui o S2, 142 s) | 43 | **43** | 0 | `…10L.testes\ataques-9.5-completo.log` |

Sobre os 43 contra os "48 VEC" do commit `6cbaf22`: contei as linhas `VEC|` emitidas nesta rodada (43, todas OK). Não reconstruí como o agente da 10.L chegou a 48; os 6 pacotes e os 7 arquivos são os mesmos.

### Medidas (vetores MEDIDA)

| Vetor | Resultado |
| --- | --- |
| **F1** fsync síncrono | 600 pedidos, 10 em paralelo, upstream instantâneo, 1 slot: sem journal ~0,1 s (p50 1,6 ms); com journal **3,9 a 7,8 s** (p50 61 a 118 ms), ou seja, **+6 a 13 ms por pedido** neste NVMe, variando entre rodadas. São **5 commits FULL por pedido aceito**, não "2 a 3": `JobEnqueued` + `SavePrompt` + `JobStarted` + `Sink.Finish` + `Finish`, mais um por flush parcial a cada ~2 s. Dois deles (`Started` e `Sink.Finish`) acontecem **com o slot preso**. Em 600 r/m (10 r/s) isso dá ~6 a 13% do tempo de slot de pedidos triviais; para jobs reais de segundos, é desprezível. **O gravador não trava a fila:** um `Finish` de 500 ms não atrasou 6 pedidos (42 a 61 ms no total), porque o hook roda fora do lock, depois de a vaga ser liberada; o `Snapshot` respondeu em < 1 ms |
| **D5** flush parcial | `UPDATE response = response \|\| ?` reescreve a linha inteira: **6 a 27 ms com 1 MiB, ~35 ms com 4 MiB, 165 a 178 ms com 16 MiB** por flush (1 a cada 2 s). Amplificação de escrita O(n²) no WAL em respostas enormes (pendência 4) |
| **M1** memória | 32 MiB de corpo: 138 MiB alocados sem journal, 299 antes e **251 depois** da correção (TotalAlloc, não pico) |
| D1 export | 20 mil linhas de metadados em 61 a 72 ms, pico de heap +2,6 a 3,3 MiB (paginação de 500) |
| D2 teto por job | prompt de 16 MiB + 2 → 16.777.216 guardados, `truncado`, sem partir a runa; resposta de 17 MiB → 16 MiB, `resp_bytes` = 17.825.792 |
| D3 teto concorrente | 16 goroutines × 40 jobs de 16 KiB num teto de 2 MiB: total ≤ teto, contador = `SUM` real, os 16 mais novos presentes, antes e depois |

## 3. Método

1. Li o diff de `6cbaf22` e os arquivos-alvo (`content.go`, `joblog.go`, `monitor/jobs.go`, `store/jobs.go`, `proxy/journal.go`, o diff de `proxy.go`, `queue.go`, `monitor.go`, `admin.go` e `main.go`) antes de atacar.
2. Escrevi um arquivo de ataque por pacote (`fase10l_adv_test.go`): `monitor` (11 vetores), `content` (6), `joblog` (4 + o filho do K1) e `cmd/mcpollama-gateway` (1, com 2 casos). Cada um imprime `VEC|id|sev|arquivo:linha|resultado` e falha quando há defeito.
   - **K1** usa um **processo filho de verdade**: o binário de teste se reexecuta, grava entrada, início e 2 pedaços da resposta, avisa `PRONTO` e é morto com `Process.Kill` (`TerminateProcess` no Windows, `SIGKILL` no Linux). Depois, o pai reabre os bancos com a mesma boot e com outra boot. Isso prova a morte do processo; **não** prova queda de energia (o cache do SO sobrevive), e o `synchronous=FULL` é o que cobre esse caso.
   - **V1** passa pelo **mesmo roteamento do `main.go`** (`/monitor`, `/monitor/` e o resto para o proxy) com TCP cru, para o caminho chegar como foi escrito (`%2F`, `//`, `/./`, `..`, `;`, `%00`, maiúsculas, `HEAD`, `OPTIONS`, `POST`, `conteudo` duplicado, `conteudo=true`, `%31`, `CONTEUDO=1`, `conteudo[]=1`).
3. Rodei no `6cbaf22` (`antes.log`), corrigi, escrevi as regressões no pacote e rodei de novo (`depois.log`).
4. Prova de que a regressão falha antes: `git archive 6cbaf22 src` no scratchpad, com as regressões copiadas por cima.
5. Recoloquei os 7 arquivos de ataque da 9.5 em `src`, rodei com e sem `-short` e **removi**. Movi os ataques da 10.L para `…10L.testes\<pacote>\`; para rodá-los de novo, copie-os de volta (o `content` usa o `open` do `content_test.go`, e o `monitor` usa o `newJobFixture` do `jobs_test.go`).

### 3.1 Erros de processo meus (corrigidos antes dos números finais)

- **(a)** O 1º H1 acusou também a página `/monitor`. O HTML estático cita o **nome** do campo `prompt_sha256` no JavaScript. Falso positivo meu: na página, o vetor passou a procurar só o **valor** do hash.
- **(b)** O 1º I1 acusou "`status=ok,erro'); DROP…` devolveu linhas". O filtro separa por vírgula, e o `ok` casou de verdade: não era injeção. Tirei o `ok,` do caso; a prova de integridade é a contagem da tabela.
- **(c)** O 1º D4 não chegou ao teto, porque o prompt de 300 KiB foi cortado pelo `MaxJobBytes` de 64 KiB do próprio vetor. Subi o teto por job para 1 MiB e pus um `Fatal` se o teto não agir.
- **(d)** Um `python - <<'EOF'` grande pelo Bash falhou com erro de aspas do shell. Conferi com `git status` que **nada** foi aplicado pela metade e refiz as edições com a ferramenta de edição.
- **(e)** A 1ª correção do M1 usava `io.WriteString` no hash, que **copia a string inteira** (o `sha256` não implementa `WriteString`): 49 MiB. A regressão pegou, e troquei por blocos de 64 KiB.
- **(f)** **Uma** rodada de `go test ./gateway/content/`, logo depois da correção (e), saiu `FAIL` sem eu capturar qual teste. Rodei o pacote mais 3 vezes, os testes do teto 4× (`-count=4`) e o `-race`: **todos ok**. No gate depois do D6, o `FAIL` voltou e **dessa vez eu capturei**: `TestPartialAnswerIsFlushedWhileStreaming` (teste da própria 10.L, não meu) com `unexpected early flush`. Com `FlushEvery` de 20 ms, um fsync lento sob a suíte inteira faz a escrita "terceiro" encontrar o flush já vencido. Passei para 200 ms / 250 ms; `-count=10` isolado e `-count=3` do pacote ficaram ok. **Provavelmente** era a mesma falha da 1ª vez, mas não tenho a saída daquela rodada para provar.
- **(h)** O D6 é um erro **meu**: a correção do D4 criou um laço sem saída para falha. O coordenador o trouxe da revisão automática; ele foi confirmado no `cd71ed8` antes da correção.
- **(g)** Classifiquei o I2 como Média ao escrever o vetor (está assim no `antes.log`). Rebaixei para Baixa: o LF à frente não está na lista OWASP, e a correção é endurecimento.

## 4. Gate (dentro de `src/`)

| Passo | Resultado | Evidência |
| --- | --- | --- |
| `go clean -testcache` + `go build ./...` | ok | `gate-test.log` |
| `go vet ./...` | ok | `gate-test.log` |
| `gofmt -l .` | vazio | `gate-test.log` |
| `go test -count=1 ./...` | 19 pacotes `ok`, 0 FAIL (rodada final, depois do D6 e do teste de flush estabilizado; a rodada anterior pegou o FAIL de §3.1 f) | `gate-test.log` |
| `-race -count=1 ./gateway/... ./cmd/mcpollama-gateway` (gcc de `deps\gcc-mingw-w64`, `CGO_ENABLED=1`) | 10 pacotes `ok` com o D6; `content` de novo com `-count=2` depois do ajuste do teste de flush | `gate-race.log` |
| `GOOS=linux` build do gateway e de `./...`, `vet` Linux e `go test -c` do `content` | ok (binários no scratchpad, fora do git) | `gate-linux.log` |
| ataques da 9.5 | 41/41 (`-short`) e 43/43 (completo) | `ataques-9.5*.log` |
| `index.html` | não mudou (LF, sem inline: vetor I3) | — |

## 5. O que mudou de comportamento (para o loop principal)

- `/monitor/api/jobs/{id}` **sem sessão** não tem mais a chave `content`. A página não a usava; ela busca o `/content` quando há login.
- Sem sessão, os números de bytes são **múltiplos de 1024**. Isso vale para `bytes_in` em `/history` e em `/state.recent`, que vinham da Fase 9: mexi neles por coerência, já que deixar o exato ali anularia o arredondamento do resto.
- O teto total é aplicado **logo depois** da gravação, não antes de ela voltar. Um teste antigo (`TestTotalCapDeletesOldestAndReportsEvent`) ganhou um `WaitCap()`.
- Se o teto falha (erro ou nada removido), ele espera 1 min antes de tentar de novo, e o total pode passar do teto nesse intervalo. **Olhe o evento `teto_conteudo_falhou`**: ele quer dizer disco cheio, banco travado ou corrompido.
- `content.db` que não abre não derruba mais o gateway: **olhe o evento `conteudo_indisponivel`** depois de implantar.

## 6. Pendências (numeradas)

1. **P1 no Linux:** provar o `0600` com arquivo pré-existente e com `-wal`/`-shm` recriados. O binário (com o D6) está em `<scratchpad>\linux\content.test` (sha256 `fd4e1ef1…17d33f8`); também dá para gerar com `GOOS=linux go test -c -o content.test ./gateway/content`. No servidor, como o usuário `mcpgw`, numa pasta temporária: `./content.test -test.run 'TestRegress10LExistingFileBecomesPrivate|TestSynchronousFullAndPrivateFile' -test.v`. Fica para o loop principal, que faz a implantação; eu não tenho acesso ao servidor.
2. **F1 no disco do servidor:** medir os +6 a 13 ms por pedido no disco real e confirmar `PRAGMA synchronous=2` nos dois pools.
3. **M1 restante (+113 MiB por pedido de 32 MiB):** vem do `extractPrompt` (o `map[string]json.RawMessage` copia o corpo, o `textOf` copia de novo e o builder, uma 3ª vez). Com `MemoryMax=1G`, isso já era apertado sem a 10.L: o proxy aloca 138 MiB por pedido de 32 MiB. Opções: decodificar com `json.Decoder` só os campos usados, ou limitar a extração a `MaxJobBytes` (o hash passaria a ser do texto guardado). Medir o **pico de RSS** ao vivo com 4 a 8 pedidos de 32 MiB.
4. **D5:** o flush parcial reescreve a linha inteira (165 a 178 ms com 16 MiB, a cada 2 s). Para respostas grandes, guardar a resposta em pedaços (`job_content_chunks`, só `INSERT`). Hoje as respostas do `qwen3-ia` são pequenas perto disso.
5. **Tamanhos aproximados:** o arredondamento a 1 KiB é **mitigação**, não sigilo. Os contadores de tokens (`prompt_eval_count`, `eval_count`) continuam exatos nas rotas abertas e também dão uma ideia do tamanho. **Decisão do owner:** esconder também esses contadores sem login, ou aceitar.
6. **Testes de tempo do `content`:** o `TestPartialAnswerIsFlushedWhileStreaming` foi estabilizado (§3.1 f). Se aparecer outro `FAIL` nesse pacote, rodar `go test -count=20 -v ./gateway/content/` e capturar o nome do teste. O teste de alocação (`TotalAlloc`) é o próximo suspeito.
7. O export pagina por `OFFSET`: com jobs chegando durante o download, uma linha pode sair repetida ou faltar (Baixa, o mesmo padrão do `/history`).
8. Das pendências 2 a 8 da 10.L: a **2** (esta revisão) está feita. A 3 (backup e cifra do `content.db`), a 4 (erro do Ollama não gravado), a 5 (`choices` e `images`), a 6 (teto por parte), a 7 (extrato duplicado com 2 jobs) e a 8 (perda máxima de ~2 s de texto) **não mudam**: nenhum ataque desta revisão as agravou. A 8 foi confirmada pelo K1, com a parte 2 gravada por estar depois do `FlushEvery`.

## 7. Arquivos

- Correção: `src/gateway/monitor/{jobs.go,monitor.go,jobs_test.go}`, `src/gateway/content/{content.go,content_test.go}`, `src/gateway/proxy/journal.go`, `src/cmd/mcpollama-gateway/main.go`
- Regressões: `src/gateway/monitor/fase10l_regress_test.go`, `src/gateway/content/fase10l_regress_test.go`, `src/cmd/mcpollama-gateway/fase10l_regress_test.go`
- Ataques (fora de `src`): `…10L.testes\gateway\monitor\`, `\gateway\content\`, `\gateway\joblog\`, `\cmd\mcpollama-gateway\` (`fase10l_adv_test.go`)
- Logs: `antes.log`, `depois.log`, `d6-antes.log`, `depois-d6-content.log`, `regressao-no-codigo-antigo.log`, `ataques-9.5.log`, `ataques-9.5-completo.log`, `gate-test.log`, `gate-race.log`, `gate-linux.log`
- MANUAL: `deploy/fase9-gateway/MANUAL.md` (API do `/jobs/{id}`, `/content`, export, `--content-db`, teto em segundo plano, tamanhos sem login, eventos)
- Commits: `cd71ed8` (fix), `998a70d` (fix D6) e o commit de docs deste report
- O `TestRegress10LCapNothingToRemoveEnds` (contador acima do teto com a tabela vazia) é **guarda**, não regressão: ele também passa no `cd71ed8`, onde a tabela vazia já encerrava a rodada.
