# Report: Fase 8.9, carga multiagente real (`gemma4:12b` × `qwen3.5:9b`)

> **Data:** 2026-10-28, das 22:46 às 23:50. **Executor:** agente Opus 5.5.
> **Plano de origem:** `.workspace/plans/MCPOllam2026-10-28-193200-fase8-bakeoff-modelos.plan.md` (§8, entradas de 22:53 a 23:50).
> **Decisão do owner que motivou a etapa (22:20–22:33):** "o foco é ter multiagentes trabalhando", "executando os
> serviços com primazia perfeita".
> **Estado:** executado. Nenhum reboot, nenhum erro de conexão. Produção restaurada e provada às 23:49. Sem commit.
> **Sigilo:** documentos só por ids neutros (`b01`…, `b07-g01`…); nenhum trecho de documento neste report; token nunca impresso.

---

## 1. Resumo

| Modelo | Degrau | Cabe? (100% GPU) | VRAM | S2 nº+título | S3 JSON | tok/s somados | p50 / p95 (s) | Erros | Pico W somado | AER `00:1b.4` |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `gemma4:12b` | 1 agente (controle) | sim | 16,3 GB | 100% (1) | 1/1 | 107 | 5,6 / 58,8 | 0 | 274 | 137 → 139 |
| `gemma4:12b` | **2×32k** | sim (2 slots) | 16,3 GB | **100%** (3) | 3/3 | 108 | 44 / 143 | 0 | 266 | 134 → 137 |
| `gemma4:12b` | **5×32k** | sim (5 slots) | 18,3 GB | **100%** (8) | 7/7 | 113 | 158 / 413 | 0 | 273 | 140 → 152 |
| `gemma4:12b` | 5×32k prompt grande | — | 18,5 GB | — | — | 481 (prefill 476) | 183 / 194 | 0 (5/5) | 278 | 152 → 153 |
| `gemma4:12b` | **10×32k** | sim (10 slots) | 19,1 GB | **100%** (13) | 15/15 | 128 | 245 / 548 | **2 timeouts (10 min)** | 275 | 153 → 155 |
| `gemma4:12b` | 10×32k prompt grande | — | — | — | — | não executado (o degrau teve erro) | — | — | — | — |
| `gemma4:12b` | **5×64k** | sim (5 slots, 64k) | 19,0 GB | carga não executada (o 10×32k não passou) | — | — | — | — | — | — |
| `qwen3.5:9b` | 1 agente (controle) | sim | 9,4 GB | 81,8% (1) | 1/1 | 479 | 1,6 / 9,2 | 0 | 315 | 156 → 156 |
| `qwen3.5:9b` | **2×32k** | sim, **mas 1 slot** | 9,4 GB | 84,8% (3) | 3/3 | 636 | 10 / 16 | 0 | 355 | 156 → 156 |
| `qwen3.5:9b` | **5×32k** | sim, 1 slot | 9,4 GB | 80,1% (8) | 7/7 | 787 | 25 / 42 | 0 | 357 | 157 → 158 |
| `qwen3.5:9b` | 5×32k prompt grande | — | 9,5 GB | — | — | 2.061 (prefill 2.036) | 23 / 42 | 0 (5/5) | 386 | 158 → 158 |
| `qwen3.5:9b` | **10×32k** | sim, 1 slot | 9,4 GB | 83,9% (15) | 15/15 | 707 | 63 / 83 | 0 | 359 | 158 → 159 |
| `qwen3.5:9b` | 10×32k prompt grande | — | 9,5 GB | — | — | 1.989 (prefill 1.963) | 47 / 92 | 0 (10/10) | 387 | 159 → 160 |
| `qwen3.5:9b` | **5×64k** | sim, 1 slot (64k) | 14,2 GB | 79,5% (8) | 7/7 | 729 | 29 / 39 | 0 | 349 | 160 → 160 |

(entre parênteses na S2: número de execuções concluídas; "tok/s somados" = (tokens de prompt + tokens gerados) ÷ janela do
degrau, do 1º início ao último fim; p50/p95 = latência por pedido das chamadas concluídas.)

**Os dois achados que decidem:**
1. **O `gemma4:12b` não ganha vazão com agentes simultâneos.** A vazão somada fica em 107–128 tok/s com 1, 2, 5 ou 10
   agentes; os pedidos só dividem o mesmo débito. A utilização média de cada GPU ficou em **25%** com 5 e com 10
   pedidos ativos, e a potência em 65–81 W por placa: não é limite de potência nem de VRAM. Com 10 agentes, o p95 chega
   a 548 s e 2 de 30 pedidos estouraram o `--timeout 10m`. A qualidade, por outro lado, ficou **impecável sob carga**:
   S2 22/22 em todas as 25 execuções concluídas (controle + degraus) e S3 JSON 26/26.
2. **O Ollama não roda o `qwen3.5:9b` em paralelo.** O journal registra `msg="model architecture does not currently
   support parallel requests" architecture=qwen35`, e o modelo sobe com `n_slots = 1` qualquer que seja o
   `OLLAMA_NUM_PARALLEL`. Os agentes entram numa fila e são atendidos um de cada vez. Mesmo assim, por ser 5 a 6 vezes
   mais rápido, ele atendeu 10 agentes × 3 tarefas em 203 s (p95 83 s) e 10 prompts de ~19 mil tokens em 93 s (p95 92 s),
   com 0 erro. A qualidade da S2 ficou no patamar dele com 1 agente (80–85%), com 2 execuções em 11/22.

**Recomendação (§6):** nenhum dos dois atende ao mesmo tempo "multiagente" e "qualidade impecável". Pelo critério do
owner ("primazia perfeita"), o **`gemma4:12b` é o único com qualidade impecável sob carga**, mas só serve como modelo
de produção se o owner aceitar a capacidade dele: cerca de **110 tok/s somados**, ou seja, **~1 extração do tipo S2
por minuto** para todos os agentes juntos. **`NUM_PARALLEL` recomendado para a produção: `1`** (com `MAX_QUEUE` ≥ 16 e
o paralelismo na fila da Fase 9).

---

## 2. Método

### 2.1 Configuração por degrau
- **"N×ctx"** = drop-in temporário `/etc/systemd/system/ollama.service.d/99-bakeoff.conf` com
  `OLLAMA_NUM_PARALLEL=N`, `OLLAMA_CONTEXT_LENGTH=ctx`, `OLLAMA_MAX_QUEUE=32` e `OLLAMA_MAX_LOADED_MODELS=1`. O resto
  vem do `10-performance.conf` da produção: `FLASH_ATTENTION=1`, `KV_CACHE_TYPE=q4_0`, `SCHED_SPREAD=1`, 4 GPUs a 100 W.
- **Script por degrau** (`.workspace/bakeoff/srv/step89.sh`, 1 sessão SSH por degrau):
  1. `uptime -s` contra o boot esperado (2026-10-28 21:10:46);
  2. para o `ollama-warm`;
  3. grava o drop-in em `.new`, `test -s`, `mv`, `sync`;
  4. `daemon-reload` e restart do `ollama`;
  5. carrega o candidato com 1 chamada curta (`think:false`, `num_predict 4`);
  6. registra `ollama ps`, VRAM por GPU e as linhas do journal (`n_slots`, `n_ctx_slot`, KV);
  7. `RESULTADO CABE` só com `100% GPU`; confere o boot de novo.
- **Ordem:** por modelo, 2×32k → 5×32k → 10×32k → 5×64k, subindo só se o anterior passasse. **Passar** = 0 erro de
  conexão ou de protocolo, 0 erro de aplicação (HTTP 429/5xx, timeout), qualidade no patamar do modelo, 100% GPU e
  nenhum evento de hardware. Latência é medida, não é critério de corte.

### 2.2 Carga (ferramenta `src/cmd/bakeoff`, modo novo `-suites multi`)
- **N agentes = N processos mcpollama separados**, como N sessões do Claude Code. Binário de teste do HEAD `ced50f8`,
  que já contém o `think:false` do `e519221`, com `--model <candidato> --allowed-models "<candidato>;nomic-embed-text"
  --timeout 10m`. Caminho: `scratchpad\bk89\mcpollama-mcp.exe`. A `variante-nothink.patch` não foi usada e o
  `D:\MCP\mcpollama-mcp.exe` não foi tocado.
- Cada agente espera um **jitter aleatório de 0–2 s antes de cada tarefa** e faz **K = 3 tarefas**, alternando:
  - **S2:** `extract` das seções numeradas de `b01`, contra o gabarito Word de 22 seções;
  - **S3:** `extract` em JSON (array de strings) de `b01`, `b02` ou `b03`, em rodízio.

  O agente i começa pela S2 quando i é par, então a carga sempre mistura as duas.
- **Sub-degrau "prompt grande"** (5×32k e 10×32k, só se o degrau passou): 1 `summarize_file` (150 palavras) por agente.
  - Nenhum documento do corpus rende 20–24 mil tokens sozinho: `b03` dá ~7,8 mil, e `b04` e `b07` passam de 85 mil.
  - Por isso foram usadas **10 fatias de ~80 mil caracteres do texto `doctext` de `b07`** (`b07-g01..g10`, ids neutros,
    no `mapa.txt`), uma por agente, para que nenhum agente reaproveite o prefixo de outro.
  - Tamanho real do prompt: **18,5–20,4 mil tokens** em 9 das 10 fatias; a `g01` rendeu 10,9–12,7 mil.
- **Travas:**
  - `uptime -s` + AER por SSH antes e depois de cada carga (`-expect-boot`);
  - **o 1º erro de conexão ou de protocolo mata todos os mcpollama do degrau** (código 3); nenhum ocorreu;
  - erros de aplicação (429, 5xx, timeout) são registrados e não abortam;
  - um degrau por vez, sob controle do agente, sem laço em background.
- **Saídas:** `.workspace/bakeoff/out/multi/<modelo>/<degrau>/` (ignorado pelo git): `calls.jsonl` por chamada,
  `summary.json` e as respostas. Os logs ficam em `out/multi/run-*.log`.

### 2.3 Medição no servidor
- **Monitor de 1 s** (`srv/mon.sh`, o mesmo da 8.4) de 22:53:34 a 23:49:03 → `srv/mon-out-89.txt`. Ele registra
  potência, temperatura, VRAM e uso por GPU a cada segundo; boot, Xid e AER de `00:1b.4` a cada 10 s; e `ollama ps`.
- **Pico de W, VRAM e ΔAER por janela:** modo novo `-suites monpeak`, que lê o arquivo do monitor na janela do degrau
  com margem de ~2 s.
- **1 agente:** para separar o efeito da concorrência do efeito da potência (o report 8.5 mediu a 150 W), rodei um
  **controle de 1 agente** com 2 tarefas na mesma config de cada modelo (100 W, ctx 32k).

---

## 3. Resultados por modelo

### 3.1 `gemma4:12b`
- **Cabe em todos os degraus**, 100% GPU, com slots reais (`n_slots` = N): 16,3 GB (2×32k), 18,3 GB (5×32k),
  19,1 GB (10×32k) e 19,0 GB (5×64k). Carga a frio de 14,7–14,9 s. O runner registra "speculative decoding context
  initialized".
- **Vazão não escala:**

  | Agentes simultâneos | tok/s somados | S2 (1.201 ou 843 tokens de saída): latência média |
  | --- | --- | --- |
  | 1 | 107 | 59 s |
  | 2 | 108 | 117 s |
  | 5 | 113 | 285 s |
  | 10 | 128 | 447 s (e 2 timeouts de 600 s) |

  - **2ª via (monitor):** utilização média por GPU de 25% (23:02–23:08 com 5 pedidos; 23:18–23:25 com 10). Pico
    somado de 266–278 W, contra 400 W possíveis.
  - **Prompt grande:** prefill agregado de **476 tok/s** com 5 pedidos de ~20k simultâneos, contra ~1.140 tok/s com 1
    pedido no report 8.5. A concorrência **reduz** até o prefill agregado.
- **Qualidade sob carga:**
  - S2 22/22 em **25 de 25** execuções concluídas (controle + 3 degraus);
  - os 4 números do sumário desatualizado de `b01` apareceram em todas, junto da numeração real completa, como no 8.5;
  - S3 JSON 26/26 válidos (9 sem cerca e 17 com cerca ```json, como no 8.5);
  - os 5 resumos grandes vieram completos (nenhum `fim=length`).
- **Erros:** os únicos foram 2 timeouts no 10×32k, os dois em S2 iniciadas na primeira rajada; foram registrados como
  erro de aplicação. Por isso o 10×32k **não passou**, o sub-degrau "prompt grande" de 10×32k não rodou e a carga do
  5×64k não rodou. Fiz no 5×64k só a checagem de "cabe", sem carga: cabe, com 19,0 GB.

### 3.2 `qwen3.5:9b`
- **O Ollama força 1 slot:** `level=WARN source=sched.go:552 msg="model architecture does not currently support
  parallel requests" architecture=qwen35`, depois `n_seq_max = 1`, `n_slots = 1`, KV "1/1 seqs" e estado recorrente de
  50 MiB (`srv/journal89-qwen35-parallel.txt`).
  - Em todos os degraus, o "N×ctx" vira **1×ctx com fila**. O `OLLAMA_NUM_PARALLEL` não tem efeito neste modelo; só o
    contexto muda a VRAM: 9,4 GB com 32k e 14,2 GB com 64k.
  - Os degraus medem, então, **N agentes contra uma fila serial**.
- **Vazão:** 636–787 tok/s somados com 2 a 10 agentes, contra 479 com 1 agente (o ganho vem de a fila nunca ficar
  vazia). Prompt grande: ~2.000 tok/s de prefill agregado com 5 e com 10 agentes.
- **Latência:**
  - 10 agentes × 3 tarefas terminaram em 203 s, com p50 de 63 s e p95 de 83 s;
  - 10 resumos de ~19 mil tokens terminaram em 93 s (p95 92 s), atendidos em sequência a cada ~8–9 s.
- **Qualidade sob carga:**
  - S2 entre 79,5% e 84,8% por degrau, no patamar do próprio modelo (81,8% no controle; 77,3% no 8.5);
  - por execução: 18 ou 19 de 22 em 33 das 35 execuções, e **11/22 em 2** (uma no 5×32k e uma no 5×64k, com saída
    encurtada: 234 tokens contra ~392);
  - S3 JSON 33/33, todos estritos (sem cerca);
  - os 15 resumos grandes vieram completos.
- **Erros:** 0 em todos os degraus. **Todos os 4 degraus passaram** nos critérios de execução; a qualidade da S2 é
  inferior à do `gemma4` em qualquer carga.

### 3.3 Hardware (4 GPUs a 100 W, fonte de 850 W)
- **Nenhum reboot:** boot 2026-10-28 21:10:46 do início ao fim, conferido antes e depois de cada uma das 12 cargas (24
  checagens do driver) e 3 vezes em cada um dos 9 scripts de degrau.
- **Xid 0;** AER não-corrigível e fatal **0**; nenhuma linha `ALERTA` no monitor.
- **AER corrigível de `00:1b.4`:** 133 → 160 (+27) em 56 min. A maior parte veio no 5×32k do `gemma4` (+12 em 12 min).
- **Pico somado:** 387 W (`qwen3.5`, prompt grande com 10 agentes, 23:44:42), com cada placa no teto de 100 W. O
  `gemma4` não passou de 278 W.

---

## 4. Comparação com 1 agente

| Métrica | `gemma4` 8.5 (150 W) | `gemma4` controle 8.9 (100 W) | `gemma4` 10 agentes | `qwen3.5` 8.5 | `qwen3.5` controle 8.9 | `qwen3.5` 10 agentes |
| --- | --- | --- | --- | --- | --- | --- |
| S2 nº+título | 100% | 100% | 100% (13 concluídas) | 77,3% | 81,8% | 83,9% |
| S2: latência por pedido | 42 s | 59 s | média 447 s; 2 timeouts | — | 9,2 s | média 56 s |
| Geração por pedido | ~37 tok/s | ~20 tok/s | ~2 tok/s (843 tokens em ~447 s) | ~52 tok/s | ~45–50 tok/s (393 tokens em 9,2 s, prefill incluído) | igual ao controle (1 pedido por vez) |
| Vazão somada | — | 107 tok/s | 128 tok/s | — | 479 tok/s | 707 tok/s |
| Prefill (prompt grande) | ~1.140 tok/s (1 pedido, 16–32k) | — | 476 tok/s (5 × ~20k) | ~3.200 tok/s (8k) | — | ~1.960 tok/s (10 × ~19k) |

- A queda de 37 para 20 tok/s no `gemma4` com 1 agente vem dos **100 W** (150 W no 8.5). Ela se soma à falta de
  ganho com a concorrência.
- **Ressalva de comparabilidade:** a vazão somada do `qwen3.5` com 1 agente inclui os ~1 s de jitter entre tarefas e a
  S3 curta; não é o teto do modelo.

---

## 5. Incidentes e desvios
1. **Nenhum incidente de hardware.**
2. **Desvio da previsão (registrado no plano antes de rodar o 10×32k):** eu previ timeouts no `gemma4` com 10 agentes,
   e eles vieram (2 de 30).
3. **Desvio de método 1:** os degraus "N×ctx" do `qwen3.5` não têm N slots, porque o Ollama recusa paralelismo nessa
   arquitetura. Eles foram executados assim mesmo, porque medem o que importa (N agentes contra o servidor). O plano
   não previa isso; a S7 do 8.5, por cálculo, supunha slots paralelos.
4. **Desvio de método 2:** o "prompt grande" ficou em 18,5–20,4 mil tokens, um pouco abaixo da faixa pedida (20–24 mil),
   e a fatia `g01` rendeu só 10,9–12,7 mil. Usei 3,9 caracteres por token, calibrado no `b03`; o texto do `b07` rende
   mais caracteres por token.
5. **Acréscimos fora do pedido:**
   - os controles de 1 agente por modelo, para separar potência de concorrência;
   - a checagem de "cabe" do `gemma4` em 5×64k sem carga (1 carga curta de modelo).
6. **Processos:** no fim (23:49), nenhum processo meu ficou rodando: nenhum `bakeoff.exe`, nenhum `mcpollama-mcp.exe` do
   `scratchpad\bk89` e o SSH do monitor terminado. Restam 2 `ssh 10.100.2.2` abertos às 21:15 e às 21:56 por um
   `cmd.exe`: são anteriores a esta etapa (início às 22:46), não são meus e não foram tocados.

---

## 6. Recomendação (critério do owner: "multiagente + qualidade impecável")

1. **Nenhum finalista atende aos dois critérios ao mesmo tempo** nesta versão do Ollama, com 4 GPUs a 100 W:
   - o `gemma4:12b` tem a qualidade, mas não tem a vazão: ~110–130 tok/s somados para todos os agentes;
   - o `qwen3.5:9b` tem a vazão (~6×) e a latência, mas não tem a qualidade: S2 ~80–84%, 2 execuções em 50%, e as
     invenções da S1 no 8.5.
2. **Se "primazia perfeita" (qualidade) manda, que é a leitura que faço da decisão do owner: `gemma4:12b`**, com estas
   condições:
   - **`OLLAMA_NUM_PARALLEL=1`:** mais slots não dão vazão e pioram o p50, porque todos os pedidos terminam juntos e
     tarde, em vez de em ordem;
   - **`OLLAMA_MAX_QUEUE` ≥ 16:** o 8 de hoje devolveria 503 com 10 agentes simultâneos e 1 slot;
   - **timeout do cliente ≥ 10–15 min** na fila cheia;
   - **capacidade declarada aos agentes de ~1 extração S2 por minuto no total** (ou ~1 resumo de 20 mil tokens a cada
     ~20–40 s).
   - Na prática, 10 agentes ativos esperam minutos. O paralelismo útil passa a ser o da **fila da Fase 9** (ordem,
     prioridade, visibilidade no `/monitor`), não o do Ollama.
3. **Se o volume de multiagentes manda: `qwen3.5:9b`**, aceitando ~16–20% de erro de numeração na S2 e as invenções da
   S1. Nesse caso, exigir conferência do Claude em toda numeração ou cláusula. Hoje eu não recomendo esse caminho para
   contratos.
4. **Antes de fechar a D8-3, duas medições baratas podem mudar o quadro:**
   - (a) o `gemma4` **sem a decodificação especulativa** (o modelo auxiliar `gemma4-assistant`), suspeita principal de
     a vazão não escalar com slots;
   - (b) o `qwen3-ia` atual sob a mesma carga multiagente. A arquitetura dele aceita paralelismo, e o 8.5 projetou
     5×32k em 4 GPUs. Ele é rápido, mas teve 6 invenções graves na S1.

   O retorno a 130–150 W, depois da troca da fonte, também recupera ~1,8× de geração do `gemma4` com 1 agente.

**Insumo da Fase 9, D9-3 (`NUM_PARALLEL` de produção): `1`** para qualquer um dos dois finalistas. Motivos:
- no `qwen3.5`, o Ollama ignora valores maiores;
- no `gemma4`, valores maiores não aumentam a vazão (107 → 128 tok/s de 1 para 10 agentes), consomem até 2,8 GB a mais
  de VRAM e levam a timeouts.

Junto com isso: `MAX_QUEUE` ≥ 16 e a fila e a priorização no gateway.

---

## 7. Estado final do servidor (provado às 23:49, `srv/restore89-out.txt`)
1. **Drop-in removido:** `ls` mostra só `10-performance.conf` e `10-performance.conf.bak-20261023-002653`.
   `systemctl show ollama -p Environment` mostra `OLLAMA_NUM_PARALLEL=1`, `OLLAMA_MAX_LOADED_MODELS=2`,
   `OLLAMA_MAX_QUEUE=8` e `OLLAMA_CONTEXT_LENGTH=65536`.
2. `daemon-reload` → `restart ollama` → `restart ollama-warm`: `ollama=active` e `warm=active`.
3. **Embed** do `nomic-embed-text` com `keep_alive:-1` em loopback: HTTP 200 em 1,04 s.
4. **`ollama ps`:**
   - `qwen3-ia:latest`: 23 GB, **100% GPU**, CONTEXT **65536**, Forever;
   - `nomic-embed-text:latest`: 851 MB, **100% GPU**, Forever.
5. **Configuração e saúde:** 4 GPUs a 100 W (VRAM 6,7/6,0/6,0/6,1 GB); boot 21:10:46; Xid 0; AER `00:1b.4` 160
   corrigíveis, não-corrigível 0 e fatal 0.
6. **Monitor:** parado (`MON-FIM 23:49:03`).

---

## 8. Artefatos
- **Código (sem commit):**
  - `src/cmd/bakeoff/multi.go` (novo): modo `multi`, classificação de erros, agregação e pontuação;
  - `src/cmd/bakeoff/monpeak.go` (novo): modo `monpeak`;
  - `src/cmd/bakeoff/multi_test.go` (novo, 5 testes);
  - `src/cmd/bakeoff/main.go`: flags `-agents`, `-tasks`, `-jitter`, `-big-docs`, `-step`, `-expect-boot`, `-mon-file`,
    `-from` e `-to`.
  - Gate: `gofmt -l` vazio, `go vet` e `go test -count=1 ./cmd/bakeoff` verdes.
- **`.workspace/bakeoff/` (ignorado pelo git):**
  - `corpus/b07-g01..g10.txt` e `mapa.txt` (linha nova);
  - `srv/`: `pre89.sh`/`-out`, `step89.sh`, `restore89.sh`/`-out`, `step89-<modelo>-<degrau>.txt` (8 arquivos),
    `journal89-qwen35-parallel.txt` e `mon-out-89.txt`;
  - `out/multi/run-*.log` (12 logs) e `out/multi/<modelo>/<degrau>/` (`calls.jsonl`, `summary.json`, respostas).

## 9. Pendências (numeradas)
1. **D8-3 (owner):** escolher entre qualidade (`gemma4:12b` com capacidade de ~110 tok/s) e vazão (`qwen3.5:9b` com
   S2 de ~80%), ou pedir as medições do §6.4.
2. Medir o `gemma4` sem a decodificação especulativa: verificar se o Ollama permite desligar o modelo auxiliar.
3. Medir o `qwen3-ia` sob a mesma carga multiagente (2/5/10 agentes), como linha de base de produção.
4. **D9-3:** `NUM_PARALLEL=1` e `MAX_QUEUE` ≥ 16 na produção. Hoje o `MAX_QUEUE=8` recusaria o 9º pedido na fila.
5. Revalidar a 130–150 W depois da troca da fonte.
6. Commit das extensões do `bakeoff` (`multi.go`, `monpeak.go`, `multi_test.go`, `main.go`): decisão do owner.
7. O "prompt grande" ficou em ~19–20 mil tokens (g01 em ~11–13 mil). Se o owner quiser a faixa de 22–24 mil, refazer
   com fatias de ~90 mil caracteres.
