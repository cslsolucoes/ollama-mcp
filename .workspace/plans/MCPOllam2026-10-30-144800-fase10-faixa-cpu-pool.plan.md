# Plano — Fase 10: faixa CPU/RAM (segundo Ollama) e pool de backends com repasse por sobrecarga

> **Estado:** **EM EXECUÇÃO** desde 2026-10-30 14:48. O owner pediu: "já pode executar, ao concluir 100%".
> **Desenho (fonte única):** a **§11 do plano da Fase 9** (`MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md`):
> §11.1–11.8 tratam da faixa CPU e §11.9 do pool de backends e do repasse por sobrecarga. Este plano não repete o desenho;
> registra a execução, as decisões e o estado.
> **Decisões já tomadas pelo owner:**
> - **D9-B1 (2026-10-30 14:47): "só a CPU local por enquanto".** O pool nasce com `gpu-local` + `cpu-local` (`ollama-cpu`
>   em 127.0.0.1:11435). Os secundários remotos são aceitos por configuração (`backends.json`), mas nenhum fica ativo.
>   O fallback do mcpollama (`--ollama-url-fallback`) é implementado e fica desligado até existir um secundário.
> - Herdadas da Fase 9: "primazia perfeita" (o mesmo modelo nas duas faixas, por padrão), `alta` nunca vai para a CPU, e o
>   teto de energia GPU+CPU é medido antes de ligar.

## Contexto do hardware (medido em 2026-10-30)
- **CPU:** Intel i5-10500T (6 núcleos / 12 threads, TDP de **35 W**).
- **RAM:** 94 GB (~9,5 GB em uso), com 15 GB de swap.
- **GPUs:** 4 a 100 W. A fonte de 850 W é o limite conhecido (memória `servidor-ia-reboots-causa-fonte`).
- O RAPL (potência da CPU) só é legível pelo root, mas a medição como root é válida.
- **Modelos instalados:** `qwen3-ia` (= `qwen3.5:9b`, denso 9B Q4_K_M, raciocina por padrão) e `nomic-embed-text`. Nenhum modelo novo é baixado sem autorização.

## Etapas

| Etapa | Equivale a | Conteúdo | Modelo do agente | Estado |
| --- | --- | --- | --- | --- |
| 10.1 | 9.9 | Inventário + **spike de medição** com uma `ollama-cpu` **temporária** (porta 11435, sem tráfego de produção): prefill e geração por modelo e por `num_thread`, carga a frio, RAM, embed do `nomic` na CPU × GPU, qualidade S2/S3 na CPU × GPU | opus | **concluída** (2026-10-30 16:19; report `MCPOllam2026-10-30-161904-fase10-1-medicao-cpu.report.md`) |
| 10.2 | 9.10 | **Interferência e energia:** carga na CPU junto com carga na GPU; queda de vazão da GPU, temperatura e potência (GPU por `nvidia-smi` + CPU por RAPL como root); define `CPUAffinity`, `CPUQuota` e `num_thread` | opus (mesmo agente da 10.1) | **concluída** (2026-10-30 16:19; mesmo report) |
| 10.3 | 9.11 | **Decisões D9-C1..C5 com o owner**, a partir dos números (ligar a faixa?, `nomic` na CPU?, modelo e limites) | loop principal | pendente |
| 10.4 | 9.12 | **Código:** pool de backends (`gpu-local`/`cpu-local`/remotos por `backends.json`), regras R1–R9 e F1–F8, EWMA, estimador de tokens, roubo de trabalho, disjuntor, histerese, "drenar"; monitor por faixa e quadro Backends; store; mcpollama (`lane`/`rota`/fallback). Testes com 3 Ollama falsos | sonnet (worktree) | pendente |
| 10.5 | 9.13 | **Revisão adversarial** da faixa e do pool + reexecução dos ataques da 9.5 | opus | pendente |
| 10.6 | 9.14 | **Implantação:** `ollama-cpu.service` + `ollama-cpu-warm` definitivos; gateway com o pool; backups e rollback ensaiado | loop principal | pendente |
| 10.7 | 9.15 | **Aceite ao vivo:** carga mista (GPU + baixa na CPU), transbordo, roubo de trabalho, queda da `ollama-cpu` → R9, qualidade igual, potência dentro do teto; 2ª via no SQLite | opus | pendente |
| 10.8 | 9.16 | Documentação (`MANUAL`, `CONEXAO-IA`, `CLAUDE.md`), report final e plano fechado | sonnet + loop principal | pendente |

**Regras de segurança da execução** (as mesmas da 8.9/9.7):
- 1 bloco por vez, conferindo o uptime antes de cada um;
- no 1º reboot: parar de vez e reportar;
- nada em background relançando testes;
- produção intacta, provada por `ollama ps` da instância GPU;
- poucas conexões SSH (fail2ban).

## Changelog

- 2026-10-30 14:48 — plano criado pelo loop principal (Opus 5.5). Ordem do owner às 14:44: "5 - já pode executar, ao concluir 100%".
  - No mesmo pedido, executados antes:
    - item 1: `cloudflared` 2026.10.3 → 2026.10.6, RPM oficial com SHA-256 da release conferido (`f05abc1e…a747bb`), 4 conexões do túnel de volta, internet `/api/version` 401 e `/monitor` 403;
    - item 3: nginx `limit_conn` 12 → 30 por IP (`339517e`; backup `ollama.conf.antes-conn30-144551`).
  - D9-B1 = só a CPU local. Disparada a 10.1/10.2 (agente opus).
- 2026-10-30 15:01:54–15:02:46 — **10.1 bloco A (inventário + instância temporária)**, agente Opus 5.5, 1 conexão SSH.
  - **Deu certo:**
    - inventário: i5-10500T 6c/12t (lógicas 6–11 = irmãs HT de 0–5), AVX2/FMA/F16C, **sem AVX-512/VNNI**; 93 GB de DDR4-2133
      (2×16 + 2×32); RAPL PL1 **35 W** e PL2 92 W; `coretemp` no hwmon2; Ollama 0.40.8; modelos em `/usr/share/ollama/.ollama/models`;
    - `ollama-cpu-test` por `systemd-run` (User=ollama, `DevicePolicy=closed`, Nice=10, `CUDA/HIP/ROCR/GGML_VK_VISIBLE_DEVICES=-1`,
      11435, NUM_PARALLEL 1, MAX_LOADED 2, KEEP_ALIVE 10m, CONTEXT 16384 e `OLLAMA_NOPRUNE=1`, este último um acréscimo meu porque os blobs são compartilhados);
    - provas: journal `inference compute id=cpu library=cpu`, `total_vram="0 B"`; `ollama ps` da 11435 com os 2 modelos **100% CPU**; `nvidia-smi` sem processo novo;
      0 descritores `/dev/nvidia*`; produção com os 2 modelos 100% GPU e os mesmos PIDs de runner.
  - **Não deu certo:** nada.
  - Sonda: prefill ~31 tok/s e geração ~4,7 tok/s → a matriz completa cabe em ~25 min.
- 2026-10-30 15:03:50–15:29:35 — **10.1 bloco B (velocidade por `num_thread` + embed)**, 1 conexão SSH, monitor de 1 s.
  - **Deu certo:**
    - `num_thread` 6/10/12: prefill 30,4/25,1/24,4 · 23,9/23,1/22,6 · 24,6/23,6/23,0 tok/s; geração de 300 tokens 4,63 · 4,66 · 3,90 tok/s;
    - carga a frio 5,3–5,6 s (trocar o `num_thread` recarrega o modelo); RSS 7,99 GB; cgroup com pico de 8,7 GB;
    - `nomic` CPU: 1 texto 104–118 ms, lote de 32 em 4,4–4,5 s; GPU: 9–11 ms e 271–283 ms.
  - **Achado:** a CPU fica cravada em **35 W (PL1)**, com pico de 56,8 W; mais threads não ajudam; 6 é o melhor.
  - **Não deu certo:** os alvos "4k/8k" saíram com 3,1k/6,1k tokens reais (calibração de 1,9 tokens por palavra; o real é 1,57).
    - **Solução:** registrar os tokens reais. A taxa varia pouco entre 3k e 6k, então a conclusão não muda.
  - Anomalia: o 1º embed da GPU levou 9,3 s; os seguintes 9–11 ms.
- 2026-10-30 15:30:03–15:56:42 — **10.1 qualidade (bakeoff `multi`, 1 agente × 6 tarefas)**, binário de teste `bk10-bf6693e`.
  - **Deu certo:**
    - GPU pela produção: S2 19/19/19 → **86,4%**, S3 3/3 estritos, S2 em ~10 s;
    - CPU pelo túnel `ssh -N -L 11435` (encerrado às 15:57), sem TLS e com token fictício: S2 19/18/19 → **84,8%**, S3 3/3 estritos;
      S2 em 293–312 s; S3 do `b03` em 384 s. **Qualidade equivalente.**
  - **Não deu certo:** nada. Sem `-uptime-ssh`, para economizar conexões; o boot foi conferido nos blocos B e C, que cercam esta etapa.
- 2026-10-30 15:57:06–16:17:37 — **10.2 bloco C (interferência e energia)**, 1 conexão SSH; GPU carregada pela produção (nginx → gateway, token pelo stdin), 2 clientes.
  - **Deu certo:**
    - geração da GPU por pedido: base **53,2** → CPU nt6 sem afinidade **47,8 (−10%)**, nt12 48,2 (−9%), **nt4 + `CPUAffinity=2-5 8-11` 50,9 (−4,3%)**,
      o mesmo + `CPUQuota=300%` 50,95; prefill da GPU sem efeito (~2.230 tok/s);
    - 0 erro em 157 pedidos de GPU;
    - **pico GPU + CPU 391,6 W** (teto de 650 W); CPU com média de 35 W e máximo de 48,6 W; temperatura máxima de 80 °C (nt12).
  - **Não deu certo:** a carga da CPU foi quase só prefill, porque o modelo respondeu "FIM" em 3 tokens; a interferência da geração na CPU ficou sem medir (pendência 4 do report).
  - **Conclusão:** a faixa CPU dá ~+4% de vazão e custa ~4% da geração da GPU, então o saldo é ~0. A recomendação para a D9-C1 é usá-la só como **contingência** (report §6).
- 2026-10-30 16:18:53 — **restauração (bloco R)**, 1 conexão SSH.
  - **Deu certo:**
    - `ollama-cpu-test` "could not be found"; 11435 sem escuta e recusando conexão;
    - 0 processos de teste no servidor;
    - produção com `qwen3-ia` 13 GB e `nomic` 851 MB a 100% GPU; `ollama.service` ativo desde 12:12:43 (nunca reiniciado); drop-ins intactos; gateway e nginx ativos;
    - boot 21:10:46; Xid 0; AER `00:1b.4` com 364 corrigíveis e 0 não-corrigíveis/fatais.
  - Total de conexões SSH: 5 (4 blocos + túnel).
  - **Pendências** (numeradas, detalhe no report §10):
    1. D9-C1..C5 com o owner (10.3);
    2. `-t` padrão com afinidade;
    3. calibração bytes/token;
    4. interferência da geração na CPU;
    5. bloco C com 10 agentes, se for a `baixa`;
    6. embed de 9,3 s;
    7. AER 364, sem base da sessão;
    8. RAM "usada" de 9,5 → 14 GB.
  - Sem commit (ordem do owner).
- 2026-10-30 16:25 — **10.3 DECIDIDA pelo owner (AskUserQuestion): "Implementar só o pool pronto para um secundário futuro".**
  - **D9-C1: NÃO implantar a faixa CPU.** Pela medição 10.1/10.2:
    - o prefill na CPU é ~90× mais lento, a geração ~11× e os embeddings ~16×;
    - a CPU é limitada a 35 W (PL1);
    - a carga na CPU tira 4–10% da geração da GPU;
    - o saldo de vazão é ~0.
    - As decisões D9-C2..C5 ficam sem efeito.
  - **Escopo novo das etapas 10.4–10.8:** pool de backends, com `backends.json` opcional; **sem ele, um único `gpu-local`, que é o comportamento de hoje, e um teste de não regressão prova isso**. Incluem:
    - saúde ativa e passiva + disjuntor (3 falhas → aberto 30 s → meio-aberto);
    - sobrecarga S1–S3 (S4, energia, opcional);
    - repasse F1–F8 (mesmo modelo e digest; `alta` só para secundário GPU; sem migração durante a execução; histerese);
    - ação "drenar backend" no `/monitor`;
    - quadro "Backends";
    - store `backend_id`/`failover_rule`/`backend_samples`;
    - mcpollama `--ollama-url-fallback` (+ token e CA próprios), **desligado por padrão**.
  - **Aceite (10.7)** numa INSTÂNCIA DE TESTE do gateway, com um secundário temporário (por exemplo, uma `ollama-cpu` na 11435 só durante o teste), para provar o repasse e o disjuntor sem tocar na produção.
  - 10.4 disparada (agente sonnet).
- 2026-10-30 16:55 — **Incidente + nova prioridade: log de jobs** (loop principal).
  - **2 reboots:**
    - **16:40:01:** durante uma `extract` do GestorERP (cliente `002-1985/42424`), com um prompt de ~36 mil tokens e as 4 GPUs entre 86 e 93 W;
    - **16:41:12:** durante a carga do modelo pelo `ollama-warm` no boot, sem nenhum pedido.

    É a fonte de 850 W outra vez, já a 100 W. Proteção imediata: fila pausada às 16:42:30, para cortar o laço.
  - **Decisão do owner (16:49):** travar o clock das GPUs (`nvidia-smi -lgc 210,1400`), aplicado e **persistido no `nvidia-powercap.service`** (backup `nvidia-powercap.service.antes-lgc-164936`), já que o warm roda depois do powercap. Fila religada às 16:49. A prova sob carga fica pendente: em repouso, o clock fica em 210 MHz.
  - **Falha achada:** o registro de um pedido só era gravado no fim, então a `extract` que derrubou o servidor **não aparece no histórico**.
  - **Pedido do owner (16:50):** "log de cada job da fila, incluindo o contexto enviado e se houve ou não resposta" + "monitorar em tempo real".
  - **Decisões (AskUserQuestion):** **prompt completo**, **resposta completa**, **retenção de 7 dias** para o conteúdo; os metadados seguem com 90 dias.
    - Esta mudança substitui a regra de sigilo "nenhum texto no banco", por decisão do owner.
    - Proteções do loop principal: banco separado em 0600, teto de tamanho total, texto visível só para o admin logado.
  - **A 10.4 foi estacionada** no ramo `wip/fase10-4-pool` (`d38fb61`, parcial, não compila). O `master` está limpo e compila.
  - Etapa nova **10.L (log de jobs)**, antes de retomar a 10.4.
- 2026-10-30 17:27 - **10.L log de jobs** (agente sonnet; sem commit, sem servidor/SSH; `master`, nao toquei no ramo `wip/fase10-4-pool`).
  - **Estado: codigo concluido e verde no gate local; NAO implantado** (pendencia 1).
  - **Desenho (decisao: reaproveitar a tabela `requests`, nao criar `jobs`):** um registro por pedido, gravado em 3 tempos, sem duplicar historico.
    - Entrada: `queue.Request.OnAccepted(Record)` (novo; roda na goroutine do pedido, fora do lock, so se a fila aceitou, antes de esperar a vaga) -> `store.JobEnqueued` (`INSERT ... ON CONFLICT DO NOTHING`, status `na_fila`) + prompt no `content.db`.
    - Inicio: o proxy chama `Journal.Started` logo depois do `Acquire` e antes de encaminhar ao Ollama (write-ahead de verdade) -> `store.JobStarted` (`UPDATE ... WHERE status='na_fila'`: nunca desfaz um fim gravado antes).
    - Fim: `OnRecord` -> `joblog.Journal.Finish` -> `store.Finish` (UPSERT; mesmo `rowid`, o `backend` do inicio sobrevive). O gravador em lote assincrono fica so de reserva se o sincrono falhar.
    - Durabilidade: segundo pool `jdb` no mesmo arquivo com `synchronous=FULL` (o pool em lote segue NORMAL; `Store.SyncModes()` prova 1 x 2). O `content.db` inteiro e FULL.
    - Recuperacao no `Open`: o que ficou `na_fila`/`em_execucao` vira `interrompido_reboot` (boot_id diferente) ou `interrompido_gateway` (mesmo boot); `finished_at` = ultimo segundo do extrato de GPU do job; evento `jobs_interrompidos` com as contagens; `RecoverResponses` marca `responded/resp_bytes` a partir da resposta parcial do `content.db`.
    - Conteudo: pacote novo `gateway/content` (SQLite proprio, 0600, `auto_vacuum` incremental): `job_content` (prompt/resposta em BLOB, tamanhos, `prompt_sha256` do prompt inteiro, `truncado`, `resp_final`). Resposta em stream acrescentada a cada ~2 s (`FlushEvery`) e no fim; teto por parte (corte em fronteira de caractere), teto total (apaga os mais antigos ate 90% e grava `teto_conteudo`), retencao 7 dias. Embeddings: so "N vetor(es) de D dimensoes"; o vetor nunca e gravado (teste varre os bytes dos dois bancos).
    - Extrato de GPU: tabela `job_gpu_samples(ts, job_id, gpu, power_w, power_limit_w, clock_mhz, util_pct, mem_used_mib, temp_c)` (+ linha da CPU com `gpu=-1`), 1 linha por GPU por job por segundo, gravada duravelmente no mesmo callback do coletor de 1 s (nenhum `nvidia-smi` novo); nada e gravado sem job em execucao. `clocks.gr` entrou na consulta (9a coluna; 5 niveis de fallback; `gpu_samples.clock_mhz` tambem; o card da GPU mostra o MHz).
    - Monitor: aba **"Log em tempo real"** (3a aba) ao vivo pelo SSE (`state.jobs`, 200), botao Detalhes (DOM + `addEventListener`) com metadados, tabela e graficos SVG de W e de clock por GPU, e prompt/resposta so para o admin; "Exportar log" (JSONL/CSV). Acrescimo do owner (17:09): `recent_done`/`--monitor-keep-done` (5 m): sub-tabela "Concluidos nos ultimos 5 min" sob "Em execucao", esmaecida, status em cor, contagem regressiva, Detalhes; so exibicao.
    - API: `GET /monitor/api/jobs`, `/jobs/{id}`, `/jobs/{id}/content` (401 sem sessao), `/jobs/export` (`conteudo=1` exige sessao antes do 1o byte; CSV neutraliza `= + - @`).
  - **Arquivos:** novos `src/gateway/content/{content,content_test}.go`, `src/gateway/joblog/{joblog,joblog_test}.go`, `src/gateway/store/{jobs,jobs_test}.go`, `src/gateway/proxy/journal.go`, `src/gateway/monitor/{jobs,jobs_test}.go`, `src/gateway/queue/accepted_test.go`, `src/cmd/mcpollama-gateway/joblog_main_test.go`; alterados `queue/queue.go`, `proxy/proxy.go`, `store/store.go`, `gpu/gpu.go` (+`limit_test.go`), `monitor/{monitor.go,index.html,monitor_test.go}`, `cmd/.../{main,config,main_test}.go`, testes-sentinela, `deploy/fase9-gateway/MANUAL.md`.
  - **Flags (env `MCPGW_*`):** `--log-content` (true), `--content-db` (`content.db` ao lado de `--db`; `/var/lib/mcpollama-gateway/content.db`), `--content-max-bytes` (2 GiB), `--content-max-job-bytes` (16 MiB, para o prompt e a resposta, cada um), `--retention-content` (168 h), `--retention-job-gpu` (168 h), `--monitor-keep-done` (5 m). Todas no `--help` (testado) e no MANUAL.
  - **Deu certo:**
    - o job de um processo morto aparece: teste com **processo filho morto com `os.Exit(3)`** (sem Close/Flush) reaberto com outro boot_id -> `interrompido_reboot` com backend, inicio e extrato; e teste com upstream pendurado + reabertura -> prompt e resposta parcial no `content.db`;
    - o upstream ve, ANTES de responder, o job `em_execucao` e o prompt completo no `content.db` (write-ahead provado de dentro do upstream) e a resposta parcial em disco no meio do stream;
    - conteudo byte a byte (NUL, UTF-8 invalido, emoji, CRLF), truncamento, teto total apagando os mais antigos, retencao 7 dias (6 d 23 h fica, 8 d sai);
    - `main` ponta a ponta com `nvidia-smi` falso (o proprio binario de teste): extrato com `clocks.gr`, picos 95,5 W / 1400 MHz, nada gravado sem job, job do boot morto fechado como interrompido, texto so no `content.db`;
    - **navegador real (Playwright, gateway local + Ollama falso):** aba, lista ao vivo, painel de detalhe com 2 graficos SVG (4 polylines) e 9 linhas de extrato; prompt `</script><img src=x onerror=alert(1)>` e resposta com `onerror=alert(2)` exibidos como **texto**: 0 `img`/`script` criados, 0 alertas, sem erro de console (so o 404 do favicon).
  - **Nao deu certo / erros meus (corrigidos):**
    1. teste da `gpu` esperava 3 tentativas de fallback; agora sao 5 (ajustado, com testes novos do `clocks.gr`);
    2. o teste de contagem de "Raciocinio" na pagina passou de 3 para 4 tabelas (ajustado de forma legitima);
    3. coloquei `style="..."` inline na pagina e o teste de CSP pegou; troquei por classe;
    4. `--content-db` sem padrao derivado quebrou os testes de `run()` (gravava em `/var/lib`); agora o padrao fica ao lado de `--db`;
    5. o teste de `run()` com GPU falsa travou no `Close` do servidor falso (o coletor de modelos tambem fala com o Ollama) e falhou sob `-race` (processo falso lento): o handler separa as rotas e o teste espera ate o extrato ter 3 s;
    6. testes que liam o fim do job logo depois da resposta competiam com o `Release` do handler (o cliente recebe o corpo antes); agora esperam o estado final.
  - **Gate (dentro de `src/`):** `go build ./...`, `go vet ./...`, `gofmt -l .` vazio, `go test -count=1 ./...` tudo `ok`; `-race` em `./gateway/... ./cmd/mcpollama-gateway` ok (2 rodadas, a 2a depois da correcao 5); build `GOOS=linux` ok; **0 CR** no `index.html`.
  - **Ataques da 9.5 reexecutados** (`...revisao-adversarial-fase9.testes/`, copiados para `src`, rodados e **removidos**): 5 pacotes + `tools`, 48 vetores `VEC|`, **todos OK**. **Adaptacao registrada:** os vetores de sentinela (`C4`, `S5`, `M7`, "sem texto no .db/-wal/-shm") continuam validos **so para o banco de metadados**; eles nao exercitam o `content.db`. A parte nova da regra ("o texto so existe no `content.db`") e coberta pelos testes-sentinela adaptados (`store/sentinel_test.go`, `sentinel_policy_test.go`: texto presente no `content.db`, ausente no `.db/-wal/-shm`, token ausente nos dois) e por `monitor/jobs_test.go` (401, XSS, CSV). Os ataques da 9.5 **nao** cobrem os endpoints novos nem o `content.db`: ver pendencia 2.
  - **Pendencias (numeradas):**
    1. **Implantar** (binario, `Instalar-Gateway.ps1`; o `content.db` nasce sozinho em `/var/lib/mcpollama-gateway`) e validar ao vivo no servidor: `PRAGMA synchronous` real, extrato com o clock travado (`clocks.gr` ~ 1400), uma `extract` grande. A fila continua religada com o clock travado; a prova sob carga segue pendente.
    2. **Revisao adversarial nova** (opus) dos vetores da 10.L: 401 do conteudo, CSV/JSONL, tetos, SQLi nos filtros novos (`status` e `cliente` sao parametrizados), DoS pelo `fsync` sincrono por pedido (um cliente inundando a fila escreve 2 a 3 commits FULL por pedido aceito; os rejeitados so 1) e `MemoryMax=1G` com prompts de ate 32 MiB x vagas.
    3. **O texto sigiloso agora esta em disco por 7 dias:** decidir backup (excluir `content.db*` de qualquer backup compartilhado) e, se quiser, cifrar o disco. O MANUAL §12 documenta onde/quem/quanto tempo.
    4. Mensagem de erro do Ollama (`{"error":...}`) nao e gravada como resposta (so `error_class`); decidir se deve.
    5. A resposta de `/v1/*` com varias `choices` concatena todas; `images` das mensagens viram "(N imagens omitidas)".
    6. O teto por job vale para o prompt e para a resposta **cada um** (ate 32 MiB por job), nao para a soma.
    7. O extrato de GPU e gravado por job: com 2 jobs simultaneos a mesma leitura vira 2 conjuntos de linhas (aceito, ~1 KB/s).
    8. Interrupcao no meio de um commit pode perder a ultima transicao e deixar o job com o estado anterior (o `interrompido_*` o cobre); a perda maxima de texto e ~2 s (`FlushEvery`).
    9. Retomar a **10.4** (pool) depois do implante: o campo `backend` ja existe no log (`gpu-local`).
  - **Sem commit** (ordem do owner).
- 2026-10-30 18:06 - **10.L revisão adversarial** (pendência 2 acima; subagente Opus 5.5, verificação crítica; `master`, sem servidor/SSH/`D:\MCP`/`.mcp.json`; ramo `wip/fase10-4-pool` intocado).
  - **Estado: concluída e verde no gate local; NÃO implantada** (o loop principal implanta). Commits `cd71ed8` (fix), `998a70d` (fix D6) e o de docs. Report: `.workspace/reports/MCPOllam2026-10-30-173000-revisao-adversarial-10L.report.md`, com os testes e os logs em `…10L.testes/`.
  - **Método:** um arquivo de ataque por pacote (`monitor` 11 vetores, `content` 6, `joblog` 4, `cmd` 1 com 2 casos; linhas `VEC|id|sev|arquivo:linha|resultado`). Rodada no `6cbaf22` (`antes.log`: 7 vetores em FALHA), correção, regressões `fase10l_regress_test.go` no pacote, rodada de novo (`depois.log`: 0 FALHA). Prova de "falha antes" com `git archive 6cbaf22` no scratchpad (`regressao-no-codigo-antigo.log`). Os ataques foram movidos para `…10L.testes/<pacote>/`.
  - **Deu certo (sem defeito):**
    - o texto só sai com sessão: 29 variações de caminho e de método pelo mesmo mux do `main.go` (`%2F`, `//`, `..`, maiúsculas, `HEAD`, `OPTIONS`, `POST`, `conteudo` duplicado ou estranho);
    - sessão expirada, cookie de outro gateway, cookie após logout, CSRF como cookie e cookie duplicado: todos 401;
    - o SSE, o `/state`, o `/jobs/{id}` e os erros não ecoam o texto, com ou sem sessão;
    - 7 pedidos malformados não ecoam o prompt, e o log, o `gw.db` e o `-wal` não têm o texto;
    - processo **filho morto de verdade** (`TerminateProcess`) no meio do stream: `interrompido_gateway` (mesma boot) e `interrompido_reboot` (boot trocada), com o prompt, a resposta parcial, `responded` e o evento;
    - ids de 128 bits (`crypto/rand`), nenhum POST novo, SQLi nos filtros sem efeito, XSS estático limpo, export de 20 mil linhas com +3 MiB de heap, teto por job exato, teto total certo com 16 goroutines;
    - **ataques da 9.5 reexecutados** (copiados para `src`, rodados e removidos): 41/41 `VEC` OK com `-short` e 43/43 sem `-short` (inclui o S2, 142 s).
  - **Achados e solução:**
    1. **H1 (Média, do coordenador):** `/monitor/api/jobs/{id}` sem login devolvia `prompt_sha256` e os tamanhos exatos (confirmação de documento). Solução: `content.Info` só com a sessão do admin e pedido do próprio site (`adminRead`); o hash não sai em nenhuma rota aberta.
    2. **H2 (Média):** `bytes_in`/`resp_bytes` exatos nas rotas abertas (fila, recentes, `/history`, `/jobs`, `/jobs/{id}`, export sem conteúdo). Solução: sem sessão, arredondados para cima em KiB (`coarseBytes`).
    3. **D4 (Média):** o teto total apagava 1 job por commit FULL dentro da gravação que passou do teto (614 jobs = 0,83 a 1,69 s, com o slot preso). Solução: em segundo plano, em transações de 500 (4 a 14 ms).
    4. **R2 (Média):** `content.db` corrompido, ou caminho que não abre, fazia o gateway sair com 1. Solução: sobe sem texto, com o evento `conteudo_indisponivel`, e deixa o arquivo intocado.
    5. **D6 (Média, do coordenador, sobre a MINHA correção do D4):** a goroutine do teto girava para sempre (100% de uma CPU) quando o `enforceCap` falhava. Solução: `enforceCap` devolve `(removidos, erro)`; com erro ou 0 removidos, sai, relê o total, espera 1 min e grava `teto_conteudo_falhou` (no máximo 1/min); com progresso, faz no máximo 5 rodadas, com backoff que o `Close` corta.
    6. **Baixas:** A2, `/content` e export de outro site → 403; I1, `status` com 40 mil itens dava 500 → só status conhecidos; I2, CSV neutraliza LF; P1, `content.db` pré-existente volta a 0600; M1, `SavePrompt` e `extractPrompt` sem cópias inteiras (+161 → +113 MiB por pedido de 32 MiB).
  - **Não deu certo / erros meus (corrigidos):**
    1. falso positivo do H1 na página `/monitor` (o JS cita o nome do campo);
    2. falso positivo do I1 (o `ok,` da lista casava de verdade);
    3. D4 que não chegava ao teto (o prompt era cortado pelo teto por job do próprio vetor);
    4. heredoc Python pelo Bash quebrou nas aspas; conferi que nada foi aplicado pela metade e refiz com a ferramenta de edição;
    5. a 1ª correção do M1 usava `io.WriteString`, que copia a string inteira (49 MiB); a regressão pegou;
    6. o D6 foi um defeito introduzido pela minha correção do D4;
    7. `TestPartialAnswerIsFlushedWhileStreaming` (da 10.L) falhava de vez em quando (`FlushEvery` de 20 ms sob a suíte inteira); passou para 200 ms. Provavelmente era o `FAIL` sem nome de uma rodada anterior.
  - **Medidas:**
    - **5 commits FULL por pedido aceito**, não 2 a 3: +6 a 13 ms por pedido no NVMe local, e 2 deles com o slot preso. Mesmo assim, um `Finish` de 500 ms **não trava a fila** (o hook roda fora do lock, depois de a vaga ser liberada).
    - O flush parcial reescreve a linha: ~35 ms com 4 MiB e 165 a 188 ms com 16 MiB, a cada 2 s.
  - **Gate (`src/`):** build, vet e `gofmt -l .` vazios; `go test -count=1 ./...` com 19 `ok`; `-race` em `./gateway/... ./cmd/mcpollama-gateway` com 10 `ok`; build, vet e `go test -c` Linux ok. O `index.html` não mudou.
  - **MANUAL:** `/jobs/{id}` (hash e tamanhos só para o admin), `/content` (403 de outro site), export (LF, KiB, `status`), `--content-db` (0600 forçado; não abrir não derruba), teto em segundo plano e `teto_conteudo_falhou`, tamanhos sem login, eventos.
  - **Pendências (numeradas):**
    1. P1 no Linux: rodar o `content.test` Linux no servidor, como `mcpgw`, numa pasta temporária (comando no report §6.1).
    2. Medir no disco do servidor o custo dos 5 commits FULL por pedido e confirmar `synchronous=2` nos dois pools.
    3. M1 restante (+113 MiB por pedido de 32 MiB, vindo do `json.Unmarshal` do `extractPrompt`); medir o pico de RSS com 4 a 8 pedidos de 32 MiB contra o `MemoryMax=1G`.
    4. D5: guardar a resposta em pedaços (só `INSERT`) se as respostas grandes virarem comuns.
    5. **Decisão do owner:** os contadores de tokens seguem exatos nas rotas abertas (o arredondamento a KiB é mitigação, não sigilo).
    6. Export por `OFFSET` pode repetir ou pular linha com jobs chegando (Baixa).
    7. Pendências 3 a 8 da 10.L (acima) seguem como estavam; a 8 foi confirmada pelo K1.
- 2026-10-30 18:12 — **10.L IMPLANTADA e conferida** (loop principal).
  - **Instalações:** `0.11.0-6cbaf22` às 17:29 e `0.11.1-f7975fb` às 18:09 (correções da revisão adversarial `cd71ed8`/`998a70d`), pelo instalador (`-Modo Atualizar`). O gate foi refeito como 2ª via antes de cada uma: 19 pacotes ok, gofmt vazio, 0 CR.
  - **Conferido no servidor:**
    - pedido-sentinela → job `ok` com início e fim;
    - prompt (com a sentinela) e resposta (332 caracteres) no `content.db`;
    - **sentinela ausente** do `gateway.db`/`-wal`;
    - `/jobs/{id}/content` sem sessão → 401;
    - extrato de GPU do job: 4 GPUs + CPU, clock máximo de 1.395 MHz (trava ok);
    - `content.db*` com 600, dono `mcpgw`;
    - **teste Linux do `content` (P1/sync FULL) rodado no servidor como `mcpgw`: PASS 2/2**;
    - nenhum evento `conteudo_indisponivel`/`teto_conteudo_falhou` depois da instalação.
  - **Extrato temporário `gpu-extrato`** (16:53–17:31): parado e removido; o CSV fica em `/root/gpu-extrato-20261030.csv`.
  - `CONEXAO-IA.md` (PC = servidor) e `CLAUDE.md` (`3358810`) atualizados.
  - **Estado das etapas:**

    | Etapa | Estado |
    | --- | --- |
    | 10.1/10.2 | concluídas |
    | 10.3 | decidida |
    | **10.L** | **concluída** |
    | 10.4 | estacionada (`wip/fase10-4-pool`) |
    | 10.5–10.8 | pendentes (dependem da 10.4) |
    | **10.S** (nova) | pendente (depende do pool e do IP/SSH do owner) |

    A 10.S é a preparação do secundário 4× RTX 2060 (i5-10500T, 24 GB de VRAM, 32 GB de RAM depois da divisão dos pentes).
  - **Pendências do owner:**
    1. Backup × `content.db` (excluir ou cifrar).
    2. Opcional: arredondar também os tokens nas rotas abertas.
    3. Troca da fonte.
    4. IP/SSH do secundário.
- 2026-10-30 18:17 — **Ordem do owner: "finalizar a fase até onde é possível, e congelar o plano".**
  - Respostas das 18:12:
    - 1 = retomar a 10.4 (retomada às 18:13 num worktree isolado, agente sonnet);
    - 2 = secundário 4× RTX 2060 **sem data** para ativar;
    - 3 = backup do `content.db` e divisão da RAM: **só anotar, sem data**;
    - 4 = fonte **sem data**, mas o owner **comprou o acionador para duas fontes**.
  - **Plano de fechamento:**
    1. 10.4 (código);
    2. 10.5 (revisão adversarial, opus);
    3. 10.6 (implantação com um único backend, comportamento idêntico ao de hoje; fallback do mcpollama desligado);
    4. 10.7 (aceite do pool numa **instância de teste**, com um secundário **temporário** que substitui o PC 2060: repasse por sobrecarga, disjuntor, histerese, drenar e fallback do mcpollama), sem tocar na produção;
    5. 10.8 (documentação + report).
  - Em seguida, **🧊 CONGELAR** este plano (convenção do ProvidersORM: estado no cabeçalho e o arquivo movido para `.workspace/plans/congelados/`). As pendências sem data ficam listadas para o descongelamento: 10.S secundário, segunda fonte, backup do `content.db` e divisão da RAM.
