# Revisão adversarial da Fase 12 (etapa 12.9)

- **Projeto:** mcpollama (`E:\MCPOllam`): `mcpollama-gateway` 0.13.0 (servidor de IA) e `mcpollama-mcp` 0.10.0 (estação), ainda não implantados
- **Alvo:** HEAD `fe8be32` no `master`, com toda a Fase 12 (12.2 a 12.8 e o contrato 1.1.5)
- **Revisor:** Opus 5.5 (coordenador desta revisão). Cinco revisores adversariais em Opus rodaram em paralelo, cada um num git worktree isolado (que já foi removido): F (`/api/feedback`), E (exportação e partição), M (monitor), P (pedido canônico e melhora) e B (bakeoff e mcpollama). O coordenador juntou os arquivos e **rodou tudo de novo** no `src/` principal (seção 3)
- **Data:** 2026-11-04, das 02:23 às 03:20 (hora local)
- **Restrições respeitadas:**
  - **nenhum código de produção alterado.** Os testes entraram em `src/` só como cópias temporárias e foram removidos; `git status --short src` está vazio;
  - sem servidor real, SSH, rede externa ou `-tags live`;
  - `GOCACHE`/`TMP` em `E:\MCPOllam\.tmp-adv`, removido no fim; nada gravado no D: fora do scratchpad
- **Testes:** `MCPOllam2026-11-04-022324-revisao-adversarial-fase12.testes\`, com um arquivo por pacote, um `README.md` (como rodar e de quais helpers cada arquivo depende) e os logs em `logs\`

---

## 1. Veredito

**Nenhum Bloqueador.** As garantias centrais da fase resistiram:
- nenhuma amostra de outra origem é copiada (estação, IP, `X-Real-Ip` forjado e IPv4 mapeado);
- nenhum sha256 puro, imagem ou pedaço da chave nos bytes dos bancos e logs;
- `X-Mcpollama-Doc` nunca chega a um backend HTTP/1.1;
- nenhuma rota do admin funciona sem sessão, CSRF e mesmo site (15 rotas × 19 sondas, tiradas do código por go/ast);
- nenhum XSS e nada vaza ao visitante;
- o predicado único derruba cada condição sozinha (24 casos);
- nenhum vazamento treino/teste no mesmo instante (fuzz de 1.400 passos);
- o bakeoff não deixa o conteúdo sair da rede: 25 URLs, rebinding, redirect e proxy de ambiente;
- o token não troca de servidor, e os relatos automáticos não atrasam a tool.

Ficam **9 achados Importantes** e **19 Menores**, todos com FAIL que se repete. Nenhum foi corrigido aqui, porque a etapa proibia mexer no código. Quatro deles são **lacunas do contrato** e dependem de decisão do owner: I-1, I-2, I-5 e I-6.

Rebaixei um achado da rubrica: **E1.4** (UTF-8 inválido no JSONL) ficou em Importante, não em Bloqueador. O defeito não injeta linha: a linha continua sendo 1 objeto. Ele só aparece com um cliente fora do mcpollama e com a confirmação do admin.

## 2. Achados

### 2.1 Importantes

| # | Achado | Onde | Vetor / teste | Correção sugerida |
| --- | --- | --- | --- | --- |
| **I-1** | **Uma estação de outra origem põe em quarentena grupos já confirmados.** O ataque é relatar ids de jobs da vítima, que o `/monitor/api/state` aberto lista. O relato entra como `sem_pedido`, ou como `distribuida` com os ids rotulados `fallback`, que nunca são validados. Nenhuma amostra é copiada, mas as referências e o `doc_hash` do job alheio entram nos grupos (C-4.5: todas as não apagadas). Treino e teste se unem, o retrato vai a `valido=0`, e descartar ou apagar o relato forjado não desfaz a quarentena. **Lacuna de contrato** (C-4.5 × C-1.11a) | `treino/ingest.go:330-347`, `treino/partition.go:131-211` | F1i, F8a · `TestAdv12FForeignLinkForcesQuarantine` | referência de outra origem ou não validada não liga grupos; não copiar o `doc_hash` de job alheio (decisão do owner) |
| **I-2** | **O texto apagado continua nos bytes do `treino.db`.** Depois de "Apagar", nenhuma linha guarda o conteúdo, mas descrição, resposta certa, pedido e resposta continuam em claro: no `-wal` logo depois, e no `treino.db` mesmo após `wal_checkpoint(TRUNCATE)` + `incremental_vacuum` (páginas livres). Ficou provado no mesmo teste que `secure_delete=ON` + checkpoint TRUNCATE resolve (F7e). **Lacuna de contrato**: a C-6.3 fala de campos, não de bytes | `treino/treino.go:169` (`dsn` sem `secure_delete`); `state.go` (`vacuum()`) | F7d, F7e · `TestAdv12FNoTextNoPlainHashAndRemanence` | `_pragma=secure_delete(ON)` no dsn e checkpoint TRUNCATE depois de apagar |
| **I-3** | **O JSONL pode divergir do que o Ollama viu.** A exportação procura `messages`/`prompt`/`system` com a grafia exata. O Ollama e o próprio gateway (`policy.go:104`, `journal.go:56`, `hmackey.go:211`) casam sem caixa e com `ſ`, e vence a última. Exemplos: com `prompt`+`PROMPT`, o JSONL leva a isca; `ſystem` some; com só `Prompt`, a ocorrência é exportável e não sai | `treino/export.go:56,62,65` | E1.6 · `TestAdv12EE1JSONLInjection` | resolver as chaves como o Ollama (EqualFold, a última vence) ou recusar duplicata por caixa ou dobra na entrada |
| **I-4** | **Uma linha do JSONL pode sair com UTF-8 inválido.** As `messages` do chat são copiadas cruas (`json.RawMessage`). O Ollama aceita o pedido, `json.Valid` dá true, mas JSON Lines e a RFC 8259 exigem UTF-8, e um leitor estrito aborta o arquivo inteiro | `treino/export.go:56-59` | E1.4 · `TestAdv12EE1JSONLInjection` | recodificar cada mensagem, ou dar `errNotExportable` quando `!utf8.Valid` |
| **I-5** | **Um caso exportado no treino pode virar caso de teste.** Basta liberar a quarentena para o teste com um membro já exportado para o treino; hoje há só um aviso (`exportados_na_outra_parte`). A C-4.12 vale "ao mesmo tempo", mas a avaliação mede um caso que pode ter sido treinado. **Lacuna de contrato** (C-4.11 × C-4.12/C-5.1) | `treino/partition.go:434-494` | E3.4 · `TestAdv12EE3LeakAcrossTime` | recusar a liberação para o teste, ou tirar do retrato o membro com `exportou` campo=treino (decisão do owner) |
| **I-6** | **Apagar e relatar de novo o mesmo job troca o lado dele.** Isso vale enquanto o job estiver no `content.db` (7 dias): ele sai num arquivo de treino e depois no JSONL de teste e no retrato, sem aviso. **Lacuna de contrato** (C-4.5/C-4.8) | `treino/partition.go:131-133`, `treino/state.go:189` | E3.5 · `TestAdv12EE3LeakAcrossTime` | guardar os HMACs de job, documento e conteúdo já exportados e herdar o lado (decisão do owner) |
| **I-7** | **`X-Mcpollama-Doc` (e `X-Mcpollama-Station`) mandado como trailer chega a um backend HTTP/2.** O `r.WithContext` herda o `r.Trailer` que o `io.ReadAll` preencheu, e o Rewrite só apaga o `Header`. Viola a C-2.11. Só ocorre se o cliente mandar o trailer e o secundário https negociar h2, o que hoje não existe; não foi verificado se o nginx descarta trailers | `gateway/proxy/proxy.go:571` (+ `:205-207`) | P3.pool.h2.* · `TestAdv12PDocTrailerOverHTTP2Backend` | `out.Trailer = nil` (ou `pr.Out.Trailer = nil` no Rewrite) |
| **I-8** | **O texto de um exemplo pode levar marcador de arquivo aberto (C-8.11).** O filtro só tira linhas inteiras `^=====…=====$` separadas por `\n`. Passam: marcador no meio da linha, com cauda, separado por `\r`/U+2028/U+0085, ou unificando nonces | `gateway/melhora/texto.go:144` | P6.marcador* (5 vetores) e P6.nonceUnificado · `TestAdv12PExampleCannotCarryAFileMarker` | neutralizar qualquer ocorrência de `=====[A-Z]+_(INICIO\|FIM)[^=]*=====` (ou a sequência `=====`) em qualquer posição |
| **I-9** | **O gate da 12.8 aceita melhora sem medir o S2/S3 do candidato (C-5.12).** O gateway só aceita `qwen3-ia`, então o candidato com instruções tem o mesmo nome do atual. Os dois lados leem o mesmo `<out>/qwen3-ia/score.json`, e o gate deu "ACEITA" | `cmd/bakeoff/erros_report.go:378-379`; `erros.go:271` não recusa `-candidato` = `-atual` | B2-05 · `TestAdv12BS2S3OfACandidateWithInstructionsIsNeverMeasured` | `score.json` próprio para o candidato (ex.: `-candidato-score`) e falha do gate quando os caminhos forem iguais |

### 2.2 Menores

| # | Achado | Onde | Vetor / teste | Correção |
| --- | --- | --- | --- | --- |
| m-1 | `/api/feedback` aceita nome de campo com outra caixa ou dobra (`CATEGORIA`, `deſcricao`); a C-1.17 manda recusar campo desconhecido | `treino/http.go:69-70` | F3e2, F3e3 · `TestAdv12FBodyAndJSON` | conferir os nomes exatos (topo e `jobs[]`) antes do `Decode` |
| m-2 | O prazo do export não vale durante uma escrita presa: um cliente que não lê segura a goroutine e até 8 MiB por mais de 9 s com o prazo de 1,5 s, porque não há `SetWriteDeadline` nem WriteTimeout | `monitor/treino.go:170-177`, `treino/export.go:157-160,179-182`, `cmd/mcpollama-gateway/main.go:369` | M4.2 · `TestAdv12MM4M5ExportOverRealServer` | `http.NewResponseController(w).SetWriteDeadline(prazo)` |
| m-3 | Um export cortado no meio (cliente lento, erro de banco) chega como **200 completo** (fim de chunked normal); o admin pode treinar com um arquivo parcial | `monitor/treino.go:177-179` | M5.3 · idem | depois do 1º byte, abortar (`panic(http.ErrAbortHandler)`) ou gravar uma linha final de erro |
| m-4 | `readJSONLimit` aceita conteúdo depois do objeto (em `completar` e "Marcar erro"); o `/api/feedback` recusa | `monitor/treino.go:206-210` | M7.2 · `TestAdv12MM7ActionBodies` | `dec.Token()==io.EOF`, como em `treino/http.go:75-77` |
| m-5 | A edição do admin aceita descrição vazia ou só de espaços e `resposta_correta` com NUL, que o relato recusa | `treino/state.go:124-128` | M7.3 · idem | validar no `Update` como no `Validate` (`report.go:67-68,82-84`) |
| m-6 | O predicado e o `buildLine` divergem: `generate` sem prompt e `chat` com `messages []` passam no `Eligible`, contam na Summary, `exportavel=true`, retrato e cota, e não viram linha (C-3.1 "se e somente se") | `treino/export.go:56,62` × `treino/partition.go:64-88` | E2.29 · `TestAdv12EE2SinglePredicate` | levar "o pedido tem forma de conversa" ao `Eligible` |
| m-7 | `Summary.Cota` mostra 31 numa categoria porque soma os grupos dos 15%; a cota real não é excedida | `treino/query.go:96` (+ `partition.go:366`) | E3.6 · `TestAdv12EE3CotaShownAboveThirty` | contar só a cota ou renomear o campo |
| m-8 | Duplicata por caixa ou dobra no corpo (`system`/`SYSTEM`, `ſystem`, `messages`/`Messages`): a melhora aplica numa chave e o Ollama lê outra | `melhora/apply.go:41,56` | P7.dup* (5) · `TestAdv12PApplyKeepsWhatTheOllamaSees` | `ErrUnsupportedBody` com mais de uma chave EqualFold |
| m-9 | O rastro da melhora relê o ponteiro do índice depois da busca: com troca no meio, `melhora_indice_id` aponta o índice errado (latente, porque o `SetIndex` só roda na partida) | `melhora/transformer.go:180` | P10.trocaDuranteBusca · `TestAdv12PIndexSwappedDuringTheSearch` | usar o `ix` local |
| m-10 | As buscas da melhora entram na fila como 127.0.0.1 e gastam a cota por IP do loopback; na rajada, um `/api/embed` local recebeu 503 | `proxy/embedder.go:79` | P9.cotaLoopback · `TestAdv12PMelhoraSearchesChargeTheLoopbackQuota` | chave de cota própria, ou fora do `PerIP` |
| m-11 | Um `hmackey.Key` por valor imprime os bytes da chave (`%v`, slog, panic), porque `String`/`GoString`/`Format` estão no receptor `*Key` (latente: nenhum código guarda por valor) | `hmackey/hmackey.go:36,103` | P5.fmtValor · `TestAdv12PKeyNeverPrinted` | bytes atrás de ponteiro |
| m-12 | No bakeoff, categoria com `Cand.N != Atual.N` é pulada, e o alvo sem ganho foi aceito (fail-open; só alcançável por `resultado-*.json` editado, no modo apurar) | `cmd/bakeoff/erros_report.go:161` | B2-04 · `TestAdv12BUnequalNIsSkippedNotRejected` | N diferente vira falha do gate |
| m-13 | Uma linha JSON inválida ou não-JSON no meio do stream é descartada em silêncio, e o caso foi medido e passou | `cmd/bakeoff/erros_replay.go:218,255-256,276-277` | B4-01, B4-02 · `TestAdv12BStreamShapes` | tolerar só a linha parcial final no teto |
| m-14 | Resposta acima de 16 MiB, sem `done`, é pontuada (passou em `omitiu`) | `cmd/bakeoff/erros_replay.go:231-241`, `erros.go:422` | B4-06 · idem | com `Truncated`, não medir (exceto `truncado`, que reprova) |
| m-15 | O sinal de menos U+2212 na resposta certa é lido como positivo (`−5` = 5) | `cmd/bakeoff/erros_score.go:290-307` | B2-09 · `TestAdv12BNumberScoring` | normalizar U+2212 para `-` |
| m-16 | A `-gateway-url` aceita caminho e query (o pedido foi a `/api/pull?nome=/api/generate`, sempre no mesmo host) | `cmd/bakeoff/erros_replay.go:81,169` | B1-05 · `TestAdv12BBaseURLPathIsNotAnEndpointBypass` | recusar caminho ≠ `/`, query, fragmento e userinfo |
| m-17 | O schema da `reportar_erro` não marca `correcao_aplicada` como obrigatório, mas o handler exige | `tools/reportar_erro.go:68` × `:253` | T3-03 · `TestAdv12BSchemaMatchesTheHandler` | incluir no `required` |
| m-18 | Só leitura: um job com estação parcial (máquina sem usuário) é tratado como "sem estação" (vale só o IP). O IP continua sendo barreira (F1g) | `treino/ingest.go:38` | — | exigir máquina **e** usuário ou nenhum |
| m-19 | Só leitura: todo erro de leitura do corpo vira 413 "corpo maior que 256 KB", inclusive Content-Length mentiroso | `treino/http.go:64-66` | — | distinguir `*http.MaxBytesError` |

### 2.3 Outras leituras para o owner (sem FAIL)

1. **Exemplo público com o pedido literal** (`melhora/texto.go:149`). O pedido da amostra entra no exemplo com até 1.200 runas. O admin só edita `resposta_correta`, `descricao` e `correcao_aplicada`, então não há como anonimizar o pedido de um caso real. O vetor P8.pedidoOriginalNoExemplo mediu um CNPJ do pedido chegando ao pedido de outro cliente. A C-8.2 depende só de o admin marcar como público apenas casos sintéticos. **Sugestão:** permitir editar o pedido do exemplo, ou exigir uma marca "sintético".
2. **Mesmo job relatado 2×** (E6.2, B2-08): conta 2 itens no retrato, 2 linhas no JSONL e dobra o peso rumo ao mínimo de 30 (`ingest.go:295-327`).
3. **Exportação sem snapshot** (`export.go:182-210`): uma ocorrência descartada durante a montagem do lote ainda sai naquele arquivo. Não foi provado.
4. **`-gateway-remoto`** (`erros_replay.go:99`): o transporte não tem CA, então com o Cloudflare Origin CA ele não deve funcionar (não é segurança).
5. **Flood** (F2): não há limite por estação. Uma estação sozinha enche os 2 GiB, e as outras passam a receber 507. O contrato não pede cota por cliente; hoje só o nginx limita (600/min).
6. **Teto** (F5b): relato perto do teto sem amostra nova, mas com 132 KB de texto, passa 128 KB do teto (só as amostras entram no orçamento), e a edição do admin não tem teto. Conforme a C-6.2 ("no máximo um relato acima").
7. **hmackey no Windows:** o `checkMode` ignora o modo, então o 0600 só é provado no Linux (12.10).

## 3. Números (rodados pelo coordenador no `src/` principal, com todos os arquivos juntos)

| Execução | Vetores novos (12.9) | OK | FALHA | Log |
| --- | --- | --- | --- | --- |
| rodada 1: ataques 9.5 + 10.L + 10.5 + 12.9 juntos, `-run TestAdv` | 364 | 325 | **39** | `logs/todos-rodada1.log` |
| rodada 2: só 12.9 | 364 | 325 | 39 (as mesmas) | `logs/fase12-rodada2.log` |
| `-race`: só 12.9 | 364 | 324 | 40 | `logs/fase12-race.log` |

- **Sob `-race`:** 0 `DATA RACE`. A 40ª FALHA foi o T4-01, por tempo (6,3 s > 5 s, com corpos de 10 MB e a retentativa do 409), e era **margem do teste, não defeito**. Subi o limite para 20 s no arquivo guardado (um travamento real iria aos 120 s do `--timeout`) e rodei de novo com `-race -count=2`: OK.
- **Vetores por revisor:**

  | Revisor | Total | FALHA |
  | --- | --- | --- |
  | F | 96 | 5 |
  | E | 52 | 6 |
  | M | 21 | 4 |
  | P | 158 | 16 |
  | B | 37 | 7 |

- As 39 FALHAs são os 28 achados das tabelas 2.1 e 2.2 menos os dois de só leitura (m-18, m-19), com 13 vetores extras de achados que têm mais de um vetor: I-1 (2), I-7 (2), I-8 (6), m-1 (2), m-8 (5) e m-13 (2).

### 3.1 Conjuntos guardados das revisões anteriores

**Correção feita** em `MCPOllam2026-10-28-235633-revisao-adversarial-fase9.testes\gateway\monitor\fase9_adv_test.go`, sem enfraquecer nada:
- o `f.h.extraAt = time.Time{}` (sumiu na 0.12.6) virou zerar `f.h.extras.have`/`at` sob `f.h.extras.mu`, o que obriga o `/state` a reler o banco, como antes;
- o "token errado" do M5 passou a trocar o último caractere por um **sempre diferente** (`otherChar`). Antes, com 1 chance em 64, o "errado" era igual ao certo e o vetor dava 409 esporádico.

Com isso, `fase9_adv_test.go` e `fase9_adv_m8_test.go` compilam no HEAD e passam (o `vec` é do arquivo irmão, e os dois vão juntos).

| Execução | 9.5 | 10.L | 10.5 | Total | Log |
| --- | --- | --- | --- | --- | --- |
| 1ª, só os antigos | 30 | 23 | 26 | 79: 77 OK, **2 FALHA** | `logs/antigos-completo.log` |
| rodada 1, junto com os novos | 30 | 23 | 26 | **79 OK, 0 FALHA** | `logs/todos-rodada1.log` |

As 2 FALHAs da 1ª rodada são **intermitentes e anteriores à Fase 12**:
- **Q4** (`gateway/queue`, `TestAdvActionsRaceStartEnd`): `registros=1799 pedidos=1800`. Falhou em 1/10 no HEAD e em **1/20 no `1fd5b27`**, de antes do código da Fase 12 (`git archive` no `.tmp-adv`). Na Fase 12 a `queue` só ganhou dois campos (`DocHash`/`DocKeyID`). Fica como pendência à parte.
- **D3** (`gateway/content`, `TestAdvLTotalCapUnderConcurrency`): é a corrida `enforceCap` × sink, já registrada como pendência na 12.3.

## 4. Método

1. Li o `CLAUDE.md`, o contrato 1.1.5 inteiro, a linha 12.9 da §12 e o changelog §16 do plano de 2026-11-02 a 2026-11-04. Usei como molde os reports e testes da 9.5, 10.L e 10.5.
2. Dividi as 7 superfícies entre 5 revisores Opus em worktrees isolados, com regras comuns:
   - só arquivos `*_adv_test.go` novos;
   - prefixos de nome por revisor (`adv12F/E/M/P/B`, `vec12X`), para os arquivos conviverem no mesmo pacote;
   - linha `VEC|id|OK/FALHA|descrição`;
   - FALHA com `t.Error`;
   - prova de que o caminho atacado foi percorrido;
   - 2 rodadas e `-race` nos vetores de concorrência.
3. Juntei todos os arquivos no `src/` principal com os ataques antigos. `go vet ./...` deu 0 erros. Rodei 2 vezes, mais `-race` (seção 3). Depois removi todas as cópias e conferi `git status --short src` vazio e `git diff --stat HEAD -- src` vazio.

### 4.1 Erros de processo

- **Coordenador:** o limite de 5 s do T4-01 era apertado demais para `-race` com a máquina carregada; foi corrigido no arquivo guardado (seção 3).
- **Revisores:** cada um corrigiu falsos FALHA próprios antes da entrega. Exemplos:
  - B: o id "maiúsculo" só tinha dígitos;
  - F: o critério da soma bruta no 507, quando o teto conta páginas; o `Write` converteu `\ufeff` em BOM cru;
  - E: fixtures de fuzz que não criavam grupos de treino (ampliadas, com um vazamento forçado como prova de que o vetor detecta);
  - M: prazo de 1,5 s não restaurado, que fez o M5.2 falhar sob `-race`; falso positivo na varredura do histórico;
  - P: um helper pegava o bloco forjado pelo cliente; uma busca por "SEGREDO" no texto inteiro.
- Nenhum processo ficou em segundo plano e nenhum arquivo de produção foi tocado.

## 5. Pendências (numeradas)

1. **Decisões do owner (lacunas do contrato):** I-1 (referência alheia nos grupos), I-2 (`secure_delete`: os bytes do arquivo contam como "conteúdo"?), I-5 (liberar para o teste um caso já exportado no treino) e I-6 (lado de um job relatado de novo depois de apagar).
2. **Corrigir antes da 12.10:** I-3, I-4, I-7, I-8 e I-9. Os Menores m-1 a m-17 podem ir no mesmo lote. Cada correção tem o teste de ataque correspondente, que passa de FAIL a OK.
3. **Na 12.10 (Linux):** provar o 0600 do `hmackey`, do `treino.db` e do arquivo blind; verificar se o nginx repassa trailers (I-7).
4. **Fora da Fase 12:** Q4 da 9.5 (`queue`, ~1/20, preexistente) e D3 da 10.L (preexistente).
5. **Owner:** exemplo público com o pedido literal (2.3 item 1) e a contagem do mesmo job relatado 2× (2.3 item 2).
