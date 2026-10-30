# Report: Fase 10.1 e 10.2, medição da faixa CPU/RAM (segunda instância do Ollama) antes do código

> **Data:** 2026-10-30, das 14:58 às 16:19. **Executor:** agente Opus 5.5.
> **Planos:** `.workspace/plans/MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md` (execução) e a §11 de
> `.workspace/plans/MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md` (desenho).
> **Estado:** executado. Nenhum reboot (boot 2026-10-28 21:10:46 conferido antes e depois de cada bloco e de cada fase: 17 checagens).
> Nenhuma queda de conexão. A instância temporária foi removida e a produção foi provada intacta às 16:18:54. Sem commit.
> **Sigilo:** documentos só por ids neutros (`b01`, `b02`, `b03`); o token de produção foi lido de `/root/API-Key`
> (linha 2) pelo stdin e nunca impresso nem gravado; contra a instância CPU foi usado um token fictício.

---

## 1. Resumo

| Modelo | Faixa | `num_thread` | Prefill (tok/s) ~1k / ~3k / ~6k | Geração 300 (tok/s) | Carga a frio | RAM (processo / cgroup) | Qualidade S2 / S3 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `qwen3-ia` | **CPU** | **6** (padrão) | **30,4 / 25,1 / 24,4** | **4,63** | 5,4 s | 7,99 GB / 8,7 GB (pico) | **84,8%** (19, 18, 19 de 22) / **3/3** estritos |
| `qwen3-ia` | CPU | 10 | 23,9 / 23,1 / 22,6 | 4,66 | 5,4 s | 7,99 GB | — |
| `qwen3-ia` | CPU | 12 | 24,6 / 23,6 / 23,0 | 3,90 | 5,6 s | 7,99 GB | — |
| `qwen3-ia` | CPU | 4, afinidade 2-5,8-11 (sob carga da GPU) | ~19 (prompt de ~2,3k) | — | 5,3 s | 7,6 GB (cgroup) | — |
| `qwen3-ia` | **GPU** (produção, 4×100 W) | — | **~1.650–1.700** (1,2k) / **~2.260** (6,1k) | **53,2** | (residente) | 13 GB VRAM | **86,4%** (19, 19, 19 de 22) / **3/3** estritos |
| `nomic-embed-text` | CPU | padrão | 1 texto: **104–118 ms**; lote de 32 (~4,7k tokens): **4,4–4,5 s** | — | 0,26 s | 313–318 MB | — |
| `nomic-embed-text` | GPU (11434) | — | 1 texto: **9–11 ms**; lote de 32: **271–283 ms** | — | (residente) | 851 MB VRAM | — |

(Prefill e geração vêm de `prompt_eval_count/prompt_eval_duration` e `eval_count/eval_duration` do próprio Ollama. Os
alvos "4k" e "8k" saíram com **3,1–3,2 mil** e **6,1 mil** tokens reais: errei a calibração de tokens por palavra, que
era 1,57 e não 1,9. A taxa por token mudou pouco entre 3k e 6k, então a conclusão não muda.)

**Os quatro achados que decidem:**
1. **A CPU é limitada por potência, não por threads.** O pacote fica cravado em **35 W** (PL1 do i5-10500T, RAPL
   `constraint_0` = 35.000.000 µW), com pico curto de **56,8 W** (PL2) nos primeiros segundos. Por isso mais threads
   não ajudam: 6 threads (1 por núcleo físico) são as mais rápidas; 10 e 12 pioram o prefill (−6% a −22%) e 12 piora a
   geração (−16%).
2. **A faixa CPU é muito lenta para o `qwen3-ia` (denso 9B):** prefill de **~24 tok/s** sustentado (~95× mais lento que a
   GPU) e geração de **~4,6 tok/s** (~11× mais lenta). Uma extração S2 de `b01` (4,9 mil tokens de prompt, 392 gerados)
   leva **~5 min na CPU** (293–312 s) contra **~10 s na GPU**; a S3 de `b03` (7,3 mil tokens) levou 6,4 min.
3. **A qualidade é equivalente:** S2 84,8% na CPU contra 86,4% na GPU no mesmo dia, com o mesmo binário e o mesmo
   modelo (diferença de 1 seção em 1 de 3 execuções, dentro do ruído do modelo, que ficou entre 79,5% e 86,4% em todas as
   medições da 8.9 e de hoje). S3: 3/3 JSON estritos nas duas faixas.
4. **A carga na CPU tira 4–10% da geração da GPU e quase nada do prefill.** Sem afinidade, a geração por pedido na GPU
   cai de **53,2 para ~48 tok/s (−10%)**; com `CPUAffinity=2-5 8-11` (2 núcleos físicos livres para o SO e o runner da
   GPU) e 4 threads, cai para **50,9 tok/s (−4,3%)**. O prefill da GPU não muda (~2.230 tok/s). **Energia não é
   problema:** pico somado GPU + CPU de **391,6 W**, bem abaixo do teto de 650 W.

**Conclusão para a D9-C1:** a faixa CPU **não acrescenta capacidade líquida** com o `qwen3-ia`. Com afinidade, a CPU
entrega ~20–24 tok/s (≈ +4% sobre os ~570 tok/s somados da GPU) e custa ~4% da geração da GPU: o saldo fica perto de
zero. Recomendo **não ligar a faixa como transbordo (R6/R7) com a GPU saudável**, e usar a `ollama-cpu` só como
**contingência** (o `cpu-local` do pool só recebe pedido pequeno quando o `gpu-local` está sem saúde ou com o disjuntor
aberto). Detalhes e alternativas na §6.

---

## 2. Método

### 2.1 Instância temporária (bloco A, 15:01:54)
- **Inventário:** i5-10500T, 6 núcleos / 12 threads (lógicas 0–5 = núcleos 0–5; 6–11 = irmãs HT dos mesmos núcleos),
  1 NUMA, L3 12 MiB, máx. 3,8 GHz; flags **AVX, AVX2, FMA, F16C; sem AVX-512 e sem AVX-VNNI**. RAM: 93 GB (2×16 + 2×32 GB
  DDR4 a **2.133 MT/s**), 9,5 GB em uso e 84 GB disponíveis no início; swap 14 GB. Sensores: `coretemp` (hwmon2), RAPL
  `intel-rapl:0` (`package-0`, PL1 35 W, PL2 92 W configurado).
- **Ollama 0.40.8**, produção com `OLLAMA_MODELS` padrão (`/usr/share/ollama/.ollama/models`, home do usuário `ollama`).
  O binário traz back-ends de CPU por microarquitetura (`libggml-cpu-*.so`), `cuda_v12`, `cuda_v13` e `vulkan`. O
  `ollama serve --help` lista `OLLAMA_NOPRUNE` e `OLLAMA_LLM_LIBRARY`.
- **Unidade transitória** `ollama-cpu-test` por `systemd-run`, com `User=ollama`, `Group=ollama`, **`DevicePolicy=closed`**
  (o cgroup não deixa abrir `/dev/nvidia*`), `Nice=10`, e o ambiente:
  `OLLAMA_HOST=127.0.0.1:11435`, `CUDA_VISIBLE_DEVICES=-1`, `HIP_VISIBLE_DEVICES=-1`, `ROCR_VISIBLE_DEVICES=-1`,
  `GGML_VK_VISIBLE_DEVICES=-1`, `OLLAMA_MODELS=<o da produção>`, `OLLAMA_NUM_PARALLEL=1`, `OLLAMA_MAX_LOADED_MODELS=2`,
  `OLLAMA_KEEP_ALIVE=10m`, `OLLAMA_CONTEXT_LENGTH=16384` e **`OLLAMA_NOPRUNE=1`** (acréscimo meu: as duas instâncias
  dividem o diretório de blobs, e a poda no início não deve rodar a partir da instância de teste).
- **Provas de que nenhuma GPU foi tocada:**
  - journal da unidade: `msg="inference compute" id=cpu library=cpu ... total="93.7 GiB"` e `total_vram="0 B"`;
  - `ollama ps` da 11435: `qwen3-ia` 6,7 GB **100% CPU** CONTEXT 16384 e `nomic-embed-text` 389 MB **100% CPU**;
  - `nvidia-smi --query-compute-apps` igual antes e depois (só os PIDs 193561 e 193611 da produção) em todos os blocos;
  - nenhum descritor `/dev/nvidia*` aberto pelos 3 processos da unidade (`ls -l /proc/<pid>/fd | grep -c nvidia` = 0);
  - produção (11434): `qwen3-ia` 13 GB **100% GPU** 65536 e `nomic` 851 MB **100% GPU**, os mesmos PIDs de runner o
    tempo todo (nenhum recarregamento).

### 2.2 Velocidade (bloco B, 15:03:50–15:29:35)
- Script `f10bench.py` no servidor, chamando a API nativa da 11435 no loopback com `think:false`, `temperature 0.7`.
  Cada prompt começa com um id único (nada de reaproveitar o cache de prefixo) e usa texto sintético em português de
  vocabulário contratual (nenhum documento real).
- Por `num_thread` ∈ {6, 10, 12}: prefill com alvos de ~1k, ~4k e ~8k tokens (`num_predict 8`) e uma geração de 300
  tokens com prompt curto. Mudar o `num_thread` **recarrega o modelo** (`load_ms` ≈ 5,4 s a cada troca); confirmei o
  `-t` do `llama-server` na linha de comando de cada runner (`-t 6`, `-t 10`, `-t 12`).
- Embed do `nomic`: 3 × 1 texto e 3 × lote de 32 textos, na 11435 (CPU) e na 11434 (GPU de produção, direto no
  loopback).
- Monitor de 1 s (`f10mon.sh`): W do pacote pelo RAPL (delta de `energy_uj`, como root), temperatura máxima do
  `coretemp`, uso de CPU por `/proc/stat`, W somado e uso das 4 GPUs pelo `nvidia-smi`.

### 2.3 Qualidade (15:30–15:56)
- Ferramenta `src/cmd/bakeoff`, modo `multi` com **1 agente e 6 tarefas** (3 × S2 de `b01` contra o gabarito de 22
  seções; S3 JSON de `b02`, `b01` e `b03`), jitter 0, binário de teste do HEAD `bf6693e`
  (`scratchpad\bk10\mcpollama-mcp.exe`, versão `bk10-bf6693e`; o `D:\MCP` não foi tocado).
- **GPU, mesmo dia (15:30:03–15:30:47):** pela produção, como cliente normal (nginx → gateway), `--timeout 10m`.
- **CPU (15:31:06–15:56:42):** túnel `ssh -N -L 11435:127.0.0.1:11435 lx-ai01-root` (PID local 19248, encerrado às
  15:57) e mcpollama com `--ollama-url http://127.0.0.1:11435`, `--tls-ca ""`, token fictício e `--timeout 20m`. Sem
  `-uptime-ssh`, para não abrir conexões extras (o boot foi conferido nos blocos B e C, que cercam esta etapa).
- Cada execução da S2 na CPU levou ~5 min (abaixo do limite de 15 min), então rodei as 3 previstas.

### 2.4 Interferência e energia (bloco C, 15:57:06–16:17:37)
- **Carga na GPU pela produção:** `f10gpu.py`, 2 clientes em série por `https://127.0.0.1:8443` (nginx → gateway →
  Ollama), com `Authorization: Bearer` lido do stdin (`sed -n 2p /root/API-Key | python3 f10gpu.py`). Cada cliente
  alterna um pedido "gen" (~1,2 mil tokens de prompt, 500 gerados) e um "pre" (~6,1 mil tokens de prompt, 3 gerados). A
  fila do gateway mantém a GPU sempre ocupada. O TLS foi aceito sem verificação de certificado **só neste loopback do
  próprio servidor** (o certificado é do domínio público, não de `127.0.0.1`).
- **Carga na CPU:** pedidos seguidos na 11435 (prompt de ~2,3 mil tokens, `num_predict 200`), um atrás do outro, de 20 s
  antes até o fim da janela da GPU. **Desvio:** o modelo respondeu "FIM" em 3 tokens, então a carga da CPU foi quase só
  **prefill** (a parte que mais exige da CPU e da energia), e não geração contínua. A interferência da geração na CPU
  (limitada por banda de memória) não foi medida separadamente (pendência 4).
- **Fases** (cada uma com a `ollama-cpu-test` recriada por `systemd-run` com as propriedades da fase; 180 s de carga
  na GPU): base GPU sozinha (150 s) → CPU `num_thread 6` sem afinidade → CPU 12 sem afinidade → CPU 4 com
  `CPUAffinity=2-5 8-11` → a mesma com `CPUQuota=300%` → base GPU sozinha de novo (150 s). A afinidade foi conferida
  por `taskset -pc` nos dois processos da unidade (`2-5,8-11`).

---

## 3. Resultados da faixa CPU

### 3.1 Velocidade por `num_thread` (sem carga na GPU)

| `num_thread` | Prefill ~0,8k | Prefill ~3,1k | Prefill ~6,1k | Geração 300 | Carga (s) |
| --- | --- | --- | --- | --- | --- |
| padrão (= 6) | 31,3 tok/s (bloco A) | — | — | 4,74 tok/s (64 tokens) | 5,3 |
| **6** | **30,4** | **25,1** | **24,4** | **4,63** | 5,4 |
| 10 | 23,9 | 23,1 | 22,6 | 4,66 | 5,4 |
| 12 | 24,6 | 23,6 | 23,0 | 3,90 | 5,6 |

- **Potência (monitor do bloco B):** com carga, o pacote ficou em **35,2 W de média** (1.327 amostras) em todas as
  configurações, com pico de **56,8 W** no começo (PL2) e temperatura máxima de **77 °C** (média 66 °C). O uso de CPU
  passou de 53% (6 threads) para 85% (10) e 100% (12) **sem ganho de vazão**: o teto é a potência de 35 W.
- O prefill de ~30 tok/s do primeiro pedido de cada série inclui a janela do PL2; o valor sustentado é **~24 tok/s**.
- A banda de memória (DDR4-2133, 4 pentes) limita a geração a ~4,6–6,8 tok/s: são ~6,6 GB de pesos lidos por token.

### 3.2 RAM
- Runner do `qwen3-ia` com 16k de contexto: **RSS 7,99 GB** (os pesos vêm por `mmap` do mesmo arquivo da produção, que já
  está no page cache). Runner do `nomic`: 313–318 MB. `ollama serve`: 42–44 MB.
- cgroup da unidade: **8,7 GB de pico** com os dois modelos carregados.
- Carga a frio: `qwen3-ia` **5,3–5,6 s**; `nomic` **0,26 s**.

### 3.3 Embeddings `nomic-embed-text`: CPU × GPU

| Caso | CPU (11435) | GPU (11434) | Razão |
| --- | --- | --- | --- |
| 1 texto (~150 tokens), quente | 104–118 ms (wall 116–128 ms) | 9–11 ms (wall 19–21 ms) | ~10× |
| lote de 32 (~4,7 mil tokens) | 4,36–4,53 s (~1.070 tok/s) | 271–283 ms (~17.000 tok/s) | ~16× |
| carga a frio | 258–261 ms | (residente) | — |

- Anomalia: o 1º embed da GPU no bloco B levou **9,3 s** (`load_ms 0`), e os seguintes 9–11 ms. Provavelmente esperou um
  pedido de produção em andamento; não investiguei (pendência 6).

### 3.4 Qualidade CPU × GPU (mesmo binário, mesmo modelo, mesmo dia)

| Faixa | S2 por execução | S2 total | S3 JSON | Latência S2 | Latência S3 (`b02` / `b01` / `b03`) | Vazão somada |
| --- | --- | --- | --- | --- | --- | --- |
| GPU (produção) | 19, 19, 19 de 22 | **57/66 (86,4%)** | 3/3 estritos | 9,6–10,2 s | 3,1 / 3,7 / 7,2 s | 663 tok/s |
| CPU (11435, 6 threads) | 19, 18, 19 de 22 | **56/66 (84,8%)** | 3/3 estritos | **293–312 s** | 43 / 210 / **384 s** | **19 tok/s** |

- Saídas S2 do mesmo tamanho (392–393 tokens) nas duas faixas. Na S3 de `b03`, a CPU gerou 258 tokens contra 67 na GPU,
  com JSON válido e estrito nos dois casos.
- A 8.9 mediu o `qwen3.5:9b` (o mesmo `qwen3-ia`) entre 79,5% e 84,8% na S2 e 100% na S3. A CPU ficou dentro dessa
  faixa. **Mesmo modelo e mesma quantização: resultado equivalente**, como previa a §11.3.

---

## 4. Interferência (10.2)

| Fase | Configuração da CPU | Geração da GPU por pedido | Δ geração | Prefill da GPU (~6,1k) | tok/s somados na GPU | Prefill obtido na CPU |
| --- | --- | --- | --- | --- | --- | --- |
| base 1 | (sem carga na CPU) | 53,20 tok/s | — | 2.140 (1 pedido a 776; sem ele ~2.260) | 575,5 | — |
| `cpu-nt6` | 6 threads, sem afinidade | 47,80 | **−10,1%** | 2.234 | 529,6 | 21,0–22,9 tok/s |
| `cpu-nt12` | 12 threads, sem afinidade | 48,17 | −9,4% | 2.221 | 548,3 | 18,5 |
| `cpu-af8-nt4` | 4 threads, `CPUAffinity=2-5 8-11` | **50,91** | **−4,3%** | 2.234 | 573,7 | 19,3 |
| `cpu-af8-nt4-q300` | o mesmo + `CPUQuota=300%` | 50,95 | −4,2% | 2.235 | 575,3 | **14,5** |
| base 2 | (sem carga na CPU) | 53,15 | — | 2.221 | 566,1 | — |

(Δ contra a média das duas bases, 53,18 tok/s. As bases ficaram estáveis: 53,20 e 53,15. Por fase: 24 a 28 pedidos na
GPU, 0 erro, 0 HTTP 429/5xx. Os "tok/s somados" dependem da mistura de pedidos "gen"/"pre" de cada janela; a geração
por pedido é a métrica limpa.)

- **O runner da GPU depende da CPU na geração** (amostragem e laço por token): sem reservar núcleos, a perda é de ~10%;
  com 2 núcleos físicos (lógicas 0, 1, 6, 7) livres, cai para ~4%.
- **`CPUQuota` não protege mais a GPU** (50,95 contra 50,91) e tira 25% do prefill da CPU (19,3 → 14,5). Como a potência
  já é limitada pelo PL1 de 35 W, a cota não serve de teto de energia.
- O prefill da GPU não sofre efeito mensurável (2.221–2.235 com e sem carga).

## 5. Energia (monitor de 1 s, 1.348 amostras no bloco B e 1.134 no C)

| Fase | CPU W média / máx. | GPU W média / máx. (4 placas) | **GPU + CPU** média / **máx.** | Temp. CPU média / máx. |
| --- | --- | --- | --- | --- |
| CPU sozinha (bloco B, sem carga na GPU) | 35,2 / **56,8** | 56,8 / 183 (picos de tráfego de produção) | — / 197,5 | 66 / 77 °C |
| base GPU 1 | 16,1 / 18,3 | 272,3 / 351,9 | 288,4 / 369,1 | 59 / 62 °C |
| `cpu-nt6` | 35,2 / 48,4 | 271,5 / 348,2 | 306,8 / 383,9 | 72 / 77 °C |
| `cpu-nt12` | 34,5 / 48,6 | 269,9 / 351,0 | 304,4 / 385,7 | 73 / **80 °C** |
| `cpu-af8-nt4` | 34,9 / 36,0 | 275,5 / 355,0 | 310,5 / 390,6 | 74 / 78 °C |
| `cpu-af8-nt4-q300` | 34,9 / 36,8 | 276,3 / 356,3 | 311,2 / **391,6** | 74 / 79 °C |
| base GPU 2 | 16,8 / 18,6 | 282,7 / 358,1 | 299,5 / 375,5 | 63 / 66 °C |

- **Pico global GPU + CPU: 391,6 W** (16:14:16; CPU 35,3 W + GPU 356,3 W). **Teto proposto de 650 W: folga de ~258 W.**
- A CPU acrescenta **~19 W em média** sobre o ocioso (16 → 35 W) e no máximo ~40 W nos picos de PL2. Com 4 GPUs a 100 W
  (teto de 400 W) + CPU no PL2 de 92 W configurado (nunca atingido; máximo visto 56,8 W), o pior caso teórico fica em
  ~490 W. O risco da fonte de 850 W **não vem da faixa CPU**.
- AER corrigível de `00:1b.4` em 364 ao fim (não-corrigível 0, fatal 0, Xid 0). Não registrei o valor no início desta
  sessão; ao fim da 8.9 (28/10 23:49) era 160. Não dá para atribuir o aumento a esta etapa (pendência 7).

---

## 6. Recomendações para as decisões D9-C1..C5

**D9-C1: ligar a faixa CPU?** **Não como faixa de capacidade (R6/R7) com a GPU saudável.** Os números:
- a CPU entrega ~19–24 tok/s somados com o `qwen3-ia` (≈ 1 extração S2 a cada 5 min), cerca de **+4%** sobre os
  ~570 tok/s da GPU;
- em troca, a geração da GPU perde **4,3%** (com afinidade) a **10%** (sem afinidade);
- o saldo é **~0** com afinidade e **negativo** sem ela, e um pedido mandado para a CPU espera ~30× mais.

Alternativas para o owner, nesta ordem:
1. **(Recomendada) Contingência:** a `ollama-cpu` existe, mas o `cpu-local` só recebe pedidos quando o `gpu-local` está
   sem saúde (S3 da §11.9: `/api/version` falha, modelo fora de "100% GPU" ou disjuntor aberto), e só pedidos pequenos
   (limiares abaixo). Mantém o serviço vivo, devagar, se a instância da GPU travar sem o servidor cair. Custo: 8,7 GB de
   RAM e nada de GPU enquanto ociosa.
2. **Só `baixa` de fundo (R6):** aceitável se o owner quiser usar a CPU ociosa para trabalho noturno pequeno, sabendo
   que cada pedido custa ~4% da geração da GPU durante o tempo dele.
3. **Não implantar a `ollama-cpu`:** a Fase 10 fica com o pool, o disjuntor e o fallback do cliente.

**D9-C2: `nomic-embed-text` na CPU (R1)?** **Não; manter na GPU.**
- O critério prévio (< ~1 s) é atendido para 1 texto (0,12 s), mas a CPU é ~10× mais lenta em 1 texto e ~16× em lote
  (4,4 s para 32 textos).
- A VRAM deixou de ser escassa: com o `qwen3.5:9b` a produção usa ~15,9 GB dos 32 GB; os 0,85 GB do `nomic` não fazem
  falta.
- O embed na CPU também ocupa núcleos e tira geração da GPU (§4).
- Se a contingência (alternativa 1) for aprovada, carregar o `nomic` também na `ollama-cpu` (313 MB) como reserva.

**D9-C3: modelo da faixa CPU.** **O mesmo `qwen3-ia`** (qualidade equivalente medida: S2 84,8% × 86,4%, S3 3/3 × 3/3).
O denso 9B na CPU fica em **~24 tok/s de prefill e ~4,6 tok/s de geração** (~95× e ~11× mais lento que a GPU); uma S2
leva ~5 min. Um MoE com ~3B ativos geraria ~2–3× mais rápido (a geração é limitada pela banda de memória), mas o
prefill continuaria lento (limite de 35 W), exigiria baixar um modelo (não autorizado) e quebraria a "primazia
perfeita". Não recomendo.

**D9-C4: limiares** (valem para a alternativa 1 ou 2; com a 3 não se aplicam):

| Parâmetro | Valor recomendado | Base |
| --- | --- | --- |
| Estimador de tempo na CPU | `prompt_tokens / 22 + num_predict / 4,6` (s) | prefill sustentado 19–24 tok/s com afinidade e 4 threads; geração 4,6 tok/s |
| `--cpu-max-prompt-tokens` | **2.000** | 2.000 / 22 ≈ 91 s de prefill |
| `--cpu-max-est` | **180 s** | 2.000 tokens + 300 gerados ≈ 156 s |
| `--overflow-wait` (R7) | **300 s** (ou R7 desligada) | uma S2 na CPU leva ~300 s; transbordar só compensa se a espera na GPU passar disso. Com a GPU a ~10 s por S2, isso só ocorre com a fila de 32 quase cheia |
| Slots da CPU | **1** (`OLLAMA_NUM_PARALLEL=1`) | a arquitetura `qwen35` força 1 slot; cada pedido leva minutos |
| `OLLAMA_MAX_QUEUE` da CPU | **4** | 4 pedidos ≈ 10–20 min de fila; mais que isso é melhor esperar a GPU |
| Núcleos reservados | **2 físicos** (lógicas 0, 1, 6, 7) | −4,3% na geração da GPU, contra −10% sem reserva |
| `num_thread` | **4**, injetado pelo gateway em todo pedido da faixa CPU | 1 thread por núcleo físico permitido; trocar o valor recarrega o modelo (~5,4 s) |

**D9-C5: quem pode forçar a faixa.** A medição não muda o padrão: **a mesma lista `--priority-ips`**. Como um pedido
forçado para a CPU pode levar minutos, recomendo que o `X-Mcpollama-Lane: cpu` também respeite o
`--cpu-max-prompt-tokens`, salvo de um IP de administrador.

## 7. Valores para a `ollama-cpu.service` definitiva (se a D9-C1 for "contingência" ou "`baixa`")

```ini
[Unit]
Description=Ollama CPU (faixa CPU/contingencia)
After=ollama.service

[Service]
User=ollama
Group=ollama
ExecStart=/usr/local/bin/ollama serve
Environment="OLLAMA_HOST=127.0.0.1:11435"
Environment="CUDA_VISIBLE_DEVICES=-1" "HIP_VISIBLE_DEVICES=-1" "ROCR_VISIBLE_DEVICES=-1" "GGML_VK_VISIBLE_DEVICES=-1"
Environment="OLLAMA_MODELS=/usr/share/ollama/.ollama/models"
Environment="OLLAMA_NOPRUNE=1"
Environment="OLLAMA_NUM_PARALLEL=1" "OLLAMA_MAX_LOADED_MODELS=2" "OLLAMA_MAX_QUEUE=4"
Environment="OLLAMA_KEEP_ALIVE=-1"
Environment="OLLAMA_CONTEXT_LENGTH=16384"
DevicePolicy=closed
CPUAffinity=2-5 8-11
Nice=10
IOSchedulingClass=idle
MemoryHigh=12G
MemoryMax=16G
Restart=on-failure
```

- **`CPUQuota`:** não usar (não protege a GPU e corta 25% da CPU; a potência já é limitada a 35 W pelo PL1). Se o owner
  quiser uma trava explícita, `CPUQuota=400%` (= os 4 núcleos permitidos) não muda nada medido.
- **`MemoryMax=16G`** (e não os 60 GB da §11.2): o pico medido foi de 8,7 GB com os dois modelos e 16k de contexto.
- **`OLLAMA_CONTEXT_LENGTH=16384`:** medido sem problema (RSS de 8 GB). Com `--cpu-max-prompt-tokens 2000`, 8192 bastaria,
  mas um pedido forçado pelo administrador com documento maior seria truncado; mantenho 16384.
- **`num_thread`:** o Ollama não tem variável de servidor para isso. O gateway deve mandar `options.num_thread=4` em todo
  pedido da faixa CPU. Falta confirmar qual `-t` o Ollama escolhe sozinho com a afinidade de 8 lógicas (pendência 2).
- **`DevicePolicy=closed`** é a garantia mais forte de que a instância nunca abre uma GPU, mesmo que uma atualização do
  Ollama mude a descoberta de dispositivos.
- `ollama-cpu-warm`: carregar `qwen3-ia` com `num_thread 4` (o mesmo valor do gateway, senão o 1º pedido recarrega) e,
  com a contingência, o `nomic`.

---

## 8. Estado final do servidor (provado às 16:18:53–16:18:54, `srv/f10-r-out.txt`)
1. `systemctl status ollama-cpu-test` → **"Unit ollama-cpu-test.service could not be found."**; `list-units 'ollama*'`
   mostra só `ollama-cf-ips`, `ollama-warm`, `ollama` e o timer `ollama-cf-ips`; 0 arquivos em `/run/systemd/transient`
   e em `/run/systemd/system.control`.
2. **11435 não escuta:** `ss -ltn` sem `:11435` e `curl` com conexão recusada.
3. Nenhum processo de teste (`f10bench`, `f10gpu`, `f10mon`) no servidor; processos `ollama`: só o `ollama serve` da
   produção (PID 193054) e o gateway.
4. **Produção:** `qwen3-ia:latest` 13 GB **100% GPU** 65536 Forever e `nomic-embed-text:latest` 851 MB **100% GPU**
   Forever. `ollama.service` ativo desde **12:12:43** (antes desta etapa; nunca reiniciado), os mesmos PIDs de runner
   (193561 e 193611) do começo ao fim.
5. Drop-ins da produção intactos: `10-performance.conf` e `10-performance.conf.bak-20261023-002653`. 4 GPUs a 100 W.
   Gateway `active`, nginx `active`.
6. Boot 2026-10-28 21:10:46; Xid 0; AER `00:1b.4` corrigível 364, não-corrigível 0 e fatal 0.
7. Local: o túnel SSH (PID 19248) foi encerrado; a 11435 local não escuta; nenhum `bakeoff.exe` ou `mcpollama-mcp.exe`
   de `scratchpad\bk10` rodando. Os 4 `mcpollama-mcp.exe` de `D:\MCP` em execução são dos clientes do owner e não foram
   tocados.
8. Observação: a RAM "usada" passou de 9,5 GB (15:01) para 14 GB (16:18), sem nenhum processo de teste vivo. Não
   investiguei a causa (pendência 8).

## 9. Artefatos (nada no git; `.workspace/bakeoff/` é ignorado)
- `srv/f10bench.py` (velocidade e embed), `srv/f10gpu.py` (carga na GPU pela produção, token pelo stdin),
  `srv/f10mon.sh` (monitor de 1 s), `srv/f10-a.sh`, `srv/f10-b.sh`, `srv/f10-c.sh`, `srv/f10-r.sh`.
- Saídas: `srv/f10-a-out.txt`, `srv/f10-b-out.txt`, `srv/f10-c-out.txt`, `srv/f10-r-out.txt`, `srv/f10-mon-b.txt`,
  `srv/f10-mon-c.txt`.
- Qualidade: `out/f10/multi/qwen3-ia/gpu-1x64k/` e `out/f10/multi/qwen3-ia/cpu-1x16k/` (`calls.jsonl`, `summary.json`,
  respostas); logs `out/f10-run-gpu.log` e `out/f10-run-cpu.log`.
- No servidor: `/root/bakeoff/f10/` (scripts e `mon-b.txt`/`mon-c.txt`).
- Nenhuma mudança de código em `src/`.

## 10. Pendências (numeradas)
1. **D9-C1..C5 com o owner (10.3)**, a partir da §6. A recomendação é a alternativa 1 (contingência).
2. Confirmar o `-t` padrão do Ollama com `CPUAffinity=2-5 8-11` (sem `num_thread` explícito) antes de definir o
   `ollama-cpu-warm`.
3. Calibrar o `bytes_in / tokens` do estimador do gateway com os documentos reais. Aqui só vale o texto sintético: 1,57
   tokens por palavra.
4. Medir a interferência da **geração** contínua na CPU, limitada pela banda de memória, sobre a GPU. O bloco C mediu
   carga de prefill, porque o modelo respondeu "FIM" em 3 tokens.
5. Se a alternativa 2 (`baixa`) for escolhida, repetir o bloco C com 10 agentes na GPU (os degraus da 8.9), para ver a
   perda com a fila cheia.
6. O embed de 9,3 s na GPU (1º do bloco B): conferir no SQLite do gateway se havia pedido de produção em andamento.
7. AER corrigível de `00:1b.4` em 364 (160 na 8.9): acompanhar pelo monitor; não há base desta sessão para atribuir.
8. RAM "usada" de 9,5 → 14 GB sem processo de teste vivo: verificar `smem`/slab numa próxima sessão SSH.
