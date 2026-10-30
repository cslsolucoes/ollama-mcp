# Report — Fase 10.7: aceite ao vivo do pool de backends (instância de teste)

- **Data/hora:** 2026-10-30, 19:44–20:04 (hora do servidor e da estação).
- **Executor:** subagente Opus 5.5 (etapa 10.7 do plano `MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md`).
- **Código sob teste:** `master` `736c205`. O gateway de teste usou **o mesmo binário da produção**
  (`/usr/local/bin/mcpollama-gateway`, `0.12.0-736c205`). O `mcpollama-mcp` foi compilado do HEAD numa cópia no scratchpad.
- **Produção:** **intacta**. Unit, nginx, drop-ins, gateway na 11500, `D:\MCP` e `.mcp.json` não foram tocados; a prova está na §5.
- **Boot:** `2026-10-30 16:42:03` (boot_id `8886b162…`) do começo ao fim, conferido no início de cada bloco e dentro do driver a cada espera.
- **Evidências:** `MCPOllam2026-10-30-200400-fase10-7-aceite-pool.testes/`:
  - `testes-linux.log`
  - `rodada-A.log`
  - `rodada-A2-B.log`
  - `fallback-mcpollama.log`
  - `sqlite-limpeza-estado-final.log`
  - `scripts/`: `drv.py`, `gw.sh`, `sec.sh`, `t1.sh`, `mcpcall.py`, `fbcount.go`
- **Conexões SSH:** 7 ao todo (reconhecimento, testes Linux, rodada A, rodadas A2+B, túnel, consultas finais com limpeza e estado final), sem problema com o fail2ban.
- **Carga:** no máximo 3 pedidos simultâneos, todos meus. Prompts de ~30 tokens e respostas de 25 a 160 tokens.

## 1. Montagem

| Peça | Como |
| --- | --- |
| Secundário temporário | `systemd-run --unit=ollama-sec-test`, `User=ollama`, `DevicePolicy=closed`, `Nice=10`, `CPUAffinity=2-5 8-11`, `CPUQuota=300%` (o melhor ponto medido na 10.2). Ambiente: `OLLAMA_HOST=127.0.0.1:11435`, `CUDA/HIP/ROCR/GGML_VK_VISIBLE_DEVICES=-1`, `OLLAMA_MODELS=/usr/share/ollama/.ollama/models` (o mesmo diretório da produção), `OLLAMA_NOPRUNE=1`, `NUM_PARALLEL=1`, `MAX_LOADED_MODELS=1`, `KEEP_ALIVE=10m`, `CONTEXT_LENGTH=8192` |
| Gateway de teste | `systemd-run --unit=mcpgw-aceite107`, `User=mcpgw`, `NoNewPrivileges`, `MemoryMax=1G`. Flags: `--listen 127.0.0.1:11599`, `--ollama 127.0.0.1:11434` (Ollama de produção = primário `gpu-local`), banco, `content.db`, admin e `backends.json` em `/var/tmp/aceite107/gw<R>/` (`mcpgw:mcpgw 0600`, pasta 0700), `--gen-slots 1`, `--priority-ips 127.0.0.1`, `--allowed-models "qwen3-ia;nomic-embed-text"`, `--overload-wait 3s`, `--overload-queue 2`, `--overload-hold 10s`, `--health-interval 5s`, `--breaker-failures 3`, `--breaker-open 20s`, `--gpu-interval 0`, `--aer-path -`, `--debug` |
| `backends.json` | `sec-cpu` = `http://127.0.0.1:11435`, `slots` 1, `modelos` `["qwen3-ia"]`, `ordem` 1, com `tipo` **`cpu`** nas rodadas A e A2 e **`remoto`** na rodada B. **É CPU de verdade nas três.** Mais um `morto` = `http://127.0.0.1:11436` (`cpu`, porta sem nada) |
| Admin | usuário `aceite`, senha aleatória gerada no servidor (arquivo 0600 do root, apagado), hash por `mcpollama-gateway hash-password` |
| Driver | `drv.py` (Python no servidor): pedidos `/api/generate` sem stream, `temperature 0`, `num_predict` 160. Lê o `/monitor/api/state` a 1 Hz e consulta o SQLite em modo somente leitura. Mede o `/api/ps`, `/api/tags` e `/api/version` da 11434 e o `/api/ps` da 11435 a 1 Hz (cenário g) |

**Rodadas:**
- **A** (gateway novo, `sec-cpu` tipo `cpu`): cenários d e c valem. Os cenários a, b e e desta rodada são inválidos (§3, erro 1).
- **A2** (gateway novo, `cpu`): cenários a, b e e.
- **B** (gateway novo, `remoto`, modelo descarregado do secundário antes): `alta` no `remoto`, mais o cenário f.

## 2. Tabela de cenários

| Cenário | Esperado | Obtido | Evidência |
| --- | --- | --- | --- |
| **1. Testes Linux** (como `mcpgw`) | `TestAdv5S4FileModesLinux`, `TestRegistryWrongModeIsRefused`, `TestAdvLPermissions`, K1 (`TestAdvLKilledMidStream` + filho `TestAdvLKillChild`) PASS | **PASS** em todos. Suítes `-short` dos 3 binários: pool 54/0/0, content 20/0/0, joblog 7 PASS/0 FAIL/3 SKIP (o helper e 2 marcados "lento") | `testes-linux.log`: `S4 OK: 0644, 0640, symlink para 0644 e token_file 0644 recusados`; `P1 OK: content.db/-wal/-shm 0600…`; `K1 OK (processo filho morto por SIGKILL)` |
| **2. Secundário 100% CPU** | modelo a 100% CPU, nenhuma GPU tocada | **PASS** | journal: `inference compute id=cpu library=cpu`, `total_vram="0 B"`; `ollama ps` 11435: `qwen3-ia 6.5 GB 100% CPU 8192`; 0 descritores `/dev/nvidia*` nos processos da unidade (PIDs 51618, 52355, 53155); cgroup `DevicePolicy=closed`, `CPUAffinity=2-5 8-11`, quota 3 s/s; `nvidia-smi --query-compute-apps` só com o PID 3455 da produção, antes, durante e depois |
| **(a) Repasse por sobrecarga** | fila/espera acima do limiar → secundário, regra gravada, 200, mesmo modelo e digest | **PASS** (A2). Rajada de 3 `normal`: `a1` no primário, **`a2` no `sec-cpu` com `failover_rule=F2:S1`**, `a3` no primário (esperou 2,6 s). Os 3 com HTTP 200 e `model=qwen3-ia`. Digest `56671c2ab938…3c90` **igual** no `/api/tags` da 11434 e da 11435 e no quadro | `rodada-A2-B.log` (GEN e SQLITE); quadro 19:57:53 `gpu-local sobrecarregado S1 used=1/1` e `sec-cpu used=1/1`; SQLite gwA2: `('sec-cpu','F2:S1','normal','ok',200,1)`; evento `overload_on S1 espera_estimada=7s fila=1` (19:57:47). O `a2` na CPU levou 86 s (carga a frio 6,6 s + 160 tokens) contra 3,4 s na GPU |
| **(b) Volta com histerese** | a sobrecarga só desliga depois do hold com a carga abaixo de 50%; o próximo pedido vai ao primário | **PASS** (A2). `overload_off` às 19:58:10, **11,3 s** depois do fim do último pedido no primário (hold 10 s), com o `a2` ainda rodando na CPU. O `b1` foi para o `gpu-local` com regra vazia | evento `overload_off carga abaixo de 50% dos limiares por 10s`; SQLite `b1 backend=gpu-local failover_rule=''` |
| **(c) Disjuntor** | backend morto → 3 falhas → aberto → meio-aberto; o secundário parado abre e, ao voltar, fecha | **PASS** (A). `morto`: `backend_down` → `breaker_open: 3 falhas seguidas` (10 s) → reabre `falhou no meio-aberto` a cada ~20–25 s (9 ciclos), **nunca fecha**. `sec-cpu` parado: `backend_down` → `breaker_open: 3 falhas seguidas` (10 s depois) → secundário de volta (`backend_up` 19:57:01) → `breaker_close: a checagem ativa deu certo` **20,0 s depois da abertura** (`--breaker-open 20s`), não antes | `rodada-A.log` (EVENTO e QUADRO `sec-cpu fora br=aberto` às 19:56:56); SQLite gwA: os eventos da 2ª via |
| **(d) Drenar e retomar** | API com sessão + CSRF + Origin, auditoria; drenado não recebe início | **PASS** (A). Login 200 (cookie `mcpgw_session` + `csrf`). Negativos: sem sessão **401**, sem CSRF **403**, Origin alheia **403**, CSRF errado **403**, id desconhecido **404**. `sec-cpu` drenado (estado `drenado`): 0 de 3 pedidos nele. Retomado. **Primário drenado:** o `d4` foi ao `sec-cpu` com **`F2:S3`** (`overload_on S3`), e o primário foi retomado depois | 4 linhas de `audit` (`user=aceite ip=127.0.0.1 acao=drain/undrain`, antes → depois e motivo); eventos `backend_drenado` e `backend_retomado` ×2 |
| **(e) `alta` com secundário `cpu`** | nunca vai para ele | **PASS** (A2). Com a sobrecarga **ligada** (quadro 19:59:26: `gpu-local sobrecarregado S1 fila=2`, `sec-cpu ok used=0/1`, livre e elegível para `normal`), as 3 `alta` esperaram o primário (wait 0, 2.937 e 5.876 ms). 0 no secundário | SQLite `priority_final=alta backend=gpu-local` ×3; evento `overload_on S1` às 19:59:20, `off` às 19:59:46, cobrindo a rajada (19:59:26–35) |
| **(e2) `alta` com secundário `remoto`** (2ª rodada, troca de tipo) | `alta` pode ir ao `remoto` | **PASS** (B). `B2` (`alta`) no `sec-cpu` com **`F2:S1`**, 200, `qwen3-ia` | SQLite gwB `('sec-cpu','F2:S1','alta','ok',200,1)` |
| **(e3) achado: `remoto` que é CPU fica `degradado`** | (desenho §16.3: `gpu`/`remoto` fora de 100% GPU → `degradado`) | Confirmado. Depois de carregar o modelo, o `sec-cpu` `remoto` passou a `degradado` (`modelo qwen3-ia:latest fora de 100% GPU: 100% CPU`), e a rajada seguinte de 3 `alta` ficou toda no primário | quadro 20:01:32; SQLite B4–B6 `gpu-local`. É o comportamento desenhado: o `tipo` precisa corresponder ao hardware (pendência 3) |
| **(f) Fallback do mcpollama** | dispara em erro de conexão e marca `backend=fallback`; um 4xx não dispara | **PASS.** **f1:** primário `127.0.0.1:9` (recusa) → `tentando o fallback uma vez` → 200 e `[uso] … backend=fallback` (`_meta.backend=fallback`); a 2ª via no SQLite do gateway de teste tem o pedido (rótulo do mcpollama, `eval_count=10`, igual ao `[uso]`). **f2:** primário vivo devolve **403** (modelo fora da allowlist) → erro da tool, **0 pedidos ao fallback** (o contador `fbcount` na 11598 não registrou nada). **f3 (controle do contador):** primário morto e fallback = contador → 1 pedido registrado, `o fallback também falhou`, e vale o erro do primário | `fallback-mcpollama.log` |
| **(g) Custo da checagem ativa** | tempo do `/api/ps` com o primário ocupado; passa de 3 s? | **Não passa.** Com a GPU gerando (3 rodadas, 340 amostras): `/api/ps` p50 0,6–0,8 ms, **máx 10 ms**; `/api/tags` máx 4,2 ms; `/api/version` máx 2,8 ms. No secundário durante a **carga a frio** (6,6 a 6,9 s, CPU): `/api/ps` máx **49,9 ms**. Recusas do secundário parado: rápidas (connection refused) | linhas `PS` dos dois logs. **Não medido:** a carga a frio **no primário (GPU)**, que exigiria descarregar o modelo de produção, o que é proibido (pendência 2) |

**Veredito:** todos os cenários pedidos passaram, com evidência pelo `/monitor/api/state`, pelo SQLite e pela resposta do cliente.

## 3. O que NÃO deu certo e a solução (erros meus, sem mexer no código)

1. **Rodada A, cenários a, b e e: teste inválido.**
   - **Causa:** o prompt "em uma frase curta" fazia o modelo parar em ~25 tokens (0,65 s na GPU). Assim a espera estimada (EWMA × 2 ≈ 1,3 s) nunca passou de `--overload-wait 3s`, e a fila não passou de 2.
   - **Solução:** prompt de texto longo com `num_predict` 160 (3,3 s na GPU) e um gateway novo (A2). Com isso a, b e e passaram. A rodada A ficou no log como está, sem apagar nada.
2. **Critério automático do meu driver nos cenários a e e da A2: veredito errado.**
   - **Causa:** o driver marcou FALHA porque procurava o `overload_on` só depois do t0 da rajada, mas o evento tinha saído segundos **antes**, disparado pelo pedido anterior (ver o achado 1 da §4).
   - **Solução:** conferi o quadro de 1 Hz e os eventos no SQLite. A sobrecarga estava ligada durante as duas rajadas (a: `hist`/S1 de 19:57:47 a 19:58:10; e: S1 de 19:59:20 a 19:59:46), então os dois cenários passam. A correção é só do meu script; o código do gateway está certo nesse ponto.
3. **Script `t1.sh`:** um `grep -- "…" -A2` com os argumentos fora de ordem só estragou a listagem dos SKIP. Os números de PASS/FAIL/SKIP estão certos.
4. **Túnel SSH:** o `TaskStop` não encerrou o `ssh.exe` filho do Git Bash (PID 37624). Encerrei com `Stop-Process` e conferi que as portas 11598 e 11599 da estação ficaram livres.

## 4. Achados (para o owner; nenhum é regressão nem bloqueia o aceite)

1. **(Baixa) S1 conta o próprio pedido que chega.**
   - **O quê:** em `pool.estWait`, a espera só vale 0 quando `WaitingGen == 0`. Na hora do despacho, o pedido novo já está na fila (`WaitingGen = 1`), então um pedido **isolado com o primário livre** dá espera = EWMA × 2 ÷ slots.
   - **Prova:** `overload_on S1 espera_estimada=7s fila=1` disparado por pedidos sozinhos (`b1` às 19:59:20; o `f1` do mcpollama às 20:02:27).
   - **Efeito:** o pedido roda no primário mesmo assim (F1). Mas o estado fica `sobrecarregado` e saem pares `overload_on`/`off` sem sobrecarga real. Com os padrões de produção (60 s, 1 slot), isso acontece a cada pedido isolado quando a EWMA passar de 30 s (um `summarize_file` de ~87 s basta).
   - **Sem efeito hoje:** a produção não tem secundário, e sem secundário o `Evaluate` nem roda.
   - **Correção sugerida:** descontar o pedido em despacho (`WaitingGen - 1` quando há slot livre), com um teste.
2. **(Informativo) `tipo: remoto` com CPU por trás só serve até o modelo ficar residente** (depois disso fica `degradado`). É o desenho §16.3. Regra de operação: o `tipo` tem de ser o do hardware real.
3. **(Cosmético) "Último erro" não é limpo quando o backend volta.** O `sec-cpu` voltou a `ok`/`fechado` e seguiu mostrando o erro de conexão antigo no quadro.
4. **(Cosmético) Mensagem do mcpollama para 403.** Mostra `endpoint bloqueado pelo nginx` também quando o 403 vem da allowlist do gateway. O texto do servidor vem junto e está certo.
5. **(Informativo) Um backend morto alterna aberto → meio-aberto → aberto a cada ~20–25 s** (9 eventos `breaker_open` em 4 min). A passagem a meio-aberto depende de alguém chamar `advance` (`Status`, `CanStart`). Nesta instância o quadro era lido a 1 Hz pelo meu driver e pela gravação de `backend_samples`, então não provei o ciclo sem nenhum leitor. Junto com a pendência 5 da 10.5 (limitar eventos de flapping), vale considerar 1 evento por minuto por backend.

## 5. Limpeza e estado final provado (20:03:31)

- **Removido:**
  - as unidades `mcpgw-aceite107` e `ollama-sec-test` (paradas, `reset-failed`, ausentes de `list-units`);
  - a pasta `/var/tmp/aceite107` inteira (bancos, `content.db`, admin, `backends.json`, senha, binários de teste, logs);
  - na estação: o túnel, o contador e a cópia `scratchpad/a107` (fontes, binários de teste Linux, `mcpollama-mcp.exe` de teste e `fbcount.exe`).
- **Produção:**
  - **Boot** `2026-10-30 16:42:03`, boot_id igual.
  - **Gateway:** `mcpollama-gateway 0.12.0-736c205` **ativo**, MainPID **49814** (o mesmo de antes), `NRestarts=0`, ativo desde 19:43:53. `ollama` ativo.
  - **Portas:** só 11434 (ollama, pid 1806) e 11500 (gateway, 49814). **11435, 11436 e 11599 livres.**
  - **Modelos:** `ollama ps`: `qwen3-ia:latest 13 GB 100% GPU 65536 Forever`. `nvidia-smi`: o mesmo runner **PID 3455** nas 4 GPUs (4554/2974/2982/3680 MiB, idêntico ao reconhecimento das 19:47), ou seja, nenhum recarregamento.
  - **`nomic-embed-text`:** **não estava residente já no início** (reconhecimento às 19:47: só `qwen3-ia`), e eu não o carreguei, porque isso alteraria a produção. O estado final é igual ao inicial. O "qwen3-ia + nomic a 100% GPU" do pedido vale, portanto, só para o `qwen3-ia`; o `nomic` carrega no 1º embed.
  - **Arquivos de produção sem alteração:** `mcpollama-gateway.service` (12:12:39), `ollama.service` (22/10) e `ollama.service.d/10-performance.conf` (12:12:42), com datas anteriores ao teste; md5 do `systemctl cat mcpollama-gateway` = `93bd5c05…`.

## 6. Pendências (numeradas)

1. **Achado 1 (S1 conta o próprio pedido):** correção pequena em `gateway/pool` com teste de regressão, antes de ligar um secundário de verdade (10.S). Decisão do owner sobre a prioridade.
2. **Carga a frio no primário (GPU)** e o `/api/ps` durante ela (pendência 4 da 10.5): não medido, porque descarregar o `qwen3-ia` de produção é proibido nesta etapa. Medir numa janela de manutenção, ou no secundário 4× RTX 2060 (10.S). Na CPU, a carga a frio não passou de 50 ms no `/api/ps`.
3. **Operação:** documentar no MANUAL §16.1 que o `tipo` deve ser o do hardware real. Um `remoto`/`gpu` que roda em CPU fica `degradado` depois do primeiro pedido e sai do pool.
4. **Cosméticos:** limpar o "último erro" no `backend_up`; texto do 403 no mcpollama; limitar o flapping de `breaker_open` de backend morto (com a pendência 5 da 10.5).
5. **Herdadas, sem mudança:** 10.S (secundário 4× RTX 2060), decisões `max_prompt_tokens` bytes÷3 e `host:porta` no monitor aberto (pendências 2 e 3 da 10.5), backup do `gateway.db` antes da migração, S4.
6. **Plano:** o changelog da Fase 10 não tinha entrada da **10.6** (implantação da `0.12.0-736c205`), embora a produção já rode essa versão. Fica para o loop principal registrar.
