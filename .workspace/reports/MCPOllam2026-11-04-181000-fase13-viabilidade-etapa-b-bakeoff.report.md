# Relatório — Fase 13 (viabilidade): etapa B e bake-off de modelos, 2026-11-04

| Campo | Valor |
| --- | --- |
| Período | 2026-11-04 14:02 → 18:07 |
| Autor | Claude Opus 5.5 (coordenação); execução no servidor por scripts em `/root/stress13/` |
| Pedido do owner | testar a viabilidade da Fase 13 (rota automática de modelo) **sem executá-la**; depois "refaça os testes", gemma4:e4b, gemma4:26b e gemma4:31b; "trabalhar os dois juntos conforme o que será feito na fase 13"; "deixando os 3 na memória"; tokens por job; "retorna ao que estava antes em produção" |
| Critério do owner | **qualidade e economia de IA paga acima do tempo** (15:19) |
| Estado final | produção **restaurada** às 18:07: `qwen3-ia` (= `qwen3.5:9b`, mesmo digest `56671c2ab938` de antes) + `nomic-embed-text`, 100% GPU, 64k; `NUM_PARALLEL=1`, `MAX_LOADED_MODELS=2`; gateway `--gen-slots 1`, lista `qwen3-ia;nomic-embed-text`; conferido por chamada real da estação (job `6d68db7101efac08752354607f9edadf`, 213 ms). Nenhum gemma4 no disco |
| Fase 13 | **não executada** (ordem do owner) |

## 1. Ambiente

- Servidor 10.100.2.2, Ollama 0.40.13, 4× RTX 3070 8 GB (32 GB), **100 W por placa**, clock travado (fonte de 850 W).
- Gateway 0.13.1-8f9cf6a, `--gen-slots 1`, `--run-timeout 10m`. Testes de VRAM/energia (etapa B) direto no Ollama
  (`127.0.0.1:11434`); bake-off de qualidade pelo caminho real (mcpollama de teste → nginx → gateway → Ollama), com o gateway aceitando
  temporariamente o modelo em teste (backup e restauração por `g13on.sh`/`t26on.sh`/`tgon.sh` e `g13off.sh`).
- `think:false` em todos os testes. Contexto: qwen3-ia 64k; gemma4 em dupla 32k (fixado no próprio nome por Modelfile, porque o gateway
  remove o `num_ctx` do cliente); gemma4 sozinho (26b/31b) 64k.

## 2. Etapa B — coexistência na VRAM (direto no Ollama, `stress13*.py`)

| Rodada | Hora | Configuração | gemma4 | Resultado |
| --- | --- | --- | --- | --- |
| 1 | 14:04 | qwen 64k, `NUM_PARALLEL=3` | 12b como alias `gemma4-ia` (32k) | **qwen3-ia 61% na CPU** (3,3 de 8,4 GB na VRAM), 10–100× mais lento; "não viável"; owner reiniciou o servidor 2× |
| 2 | 14:58 | qwen 64k, `NUM_PARALLEL=1` | 12b, `num_ctx 32768` por pedido | **todos 100% GPU**, 26,2/32 GB, folga mínima ~470 MiB, 23/23 ok, pico 239,1 W; gemma4 ~32–38 tok/s, qwen ~42 tok/s, juntos ~64 tok/s |
| 3 | 15:04 | **qwen 48k, `NUM_PARALLEL=2`** (teste e reverter) | 12b 32k | pior: gemma4 dobrou na VRAM (2,5→5,3 GB no `/api/ps`), nomic 0,4→0,8; 28,3/32 GB, folga ~380 MiB; vazão misturada caiu à metade; 23/23 ok; revertido às 15:07 |
| 4 | 15:19 | qwen 64k, `NUM_PARALLEL=1` | **e4b** 32k | 100% GPU, 22,8/32 GB, folga ~1,7 GB, 23/23 ok, pico 219 W; e4b 34–37 tok/s (não mais rápido que o 12b) |

Conclusões medidas: (1) a causa provável da rodada 1 é o `NUM_PARALLEL=3` (o gemma4 reserva contexto por pedido paralelo; o qwen35 não
paraleliza); (2) `NUM_PARALLEL=1` é a melhor configuração; (3) reduzir o qwen para 48k economiza só ~1,4 GB.

## 3. Bake-off de qualidade (S2 numeração, S3 extração JSON/lista/tabela, S6 contas e contagens)

Pelo gateway, `bakeoff-f13.exe` com `mcp-f13.exe` (versão `bakeoff-f13-8f9cf6a`), suítes `s2,s3,s6`; duplas rodando **ao mesmo tempo**
com os 3 modelos na memória.

| Teste | qwen3-ia | e4b (dupla) | 12b (dupla) | 26b (sozinho) | 31b (sozinho) |
| --- | --- | --- | --- | --- | --- |
| S2 números | 86,4% | 86,4% | **100%** | **100%** | **100%** |
| S2 número + título | 84,5–85,5% | 81,8% | **100%** | **100%** | **100%** |
| S2 JSON estrito (sem cerca) | 5/5 | 5/5 | 0/5 | 0/5 | 0/5 |
| S3 ok | 100% | 94,4% (1 JSON inválido) | 100% | 100% | 100% |
| S3 sem cerca de código | 18/18 | 15/18 | 14/18 | 12/18 | 12/18 |
| S6 conta direta | 0/15 | 3/15 | 6/15 | 9/15 | **15/15** |
| S6 conta passo a passo | 10/10 | 10/10 | 10/10 | 10/10 | 10/10 |
| S6 contagem direta | 3/6 | 0/6 | **4/6** | 3/6 | 2/6 |
| S6 contagem por lista | 3/6 | 3/6 | 3/6 | **4/6** | 3/6 |
| Duração das 60 chamadas | ~11 min (em dupla) | ~11 min | ~16 min | ~12 min | **~39 min** |
| Chamadas com erro | 0 | 0 | 0 | 0 | 0 |

- Todos os modelos marcam os mesmos "inventados" `1.2, 1.2.1–1.2.3` no S2: suspeita de gabarito incompleto (**não verificado**).
- O qwen3-ia sempre perde `1.1.1–1.1.3`; os gemma4 grandes (12b/26b/31b) acertam tudo.

## 4. Carga de 10 agentes × 3 tarefas (gateway, 1 por vez, 300 s por chamada)

| Modelo | ok | timeout (300 s) | p50 / p95 | Geração agregada | S2 nas que terminaram |
| --- | --- | --- | --- | --- | --- |
| gemma4:26b | **25/30** | 5 | 228 s / 277 s | 15,3 tok/s | 100% |
| gemma4:31b | 6/30 | **24** | 188 s / 263 s | 3,6 tok/s | 100% |

Com o padrão de produção de **120 s** por chamada no mcpollama, os dois teriam mais timeouts. Isso alimenta a Fase 11 (assíncrono).

## 5. VRAM e energia (amostragem a 1 s, `w26.csv`)

| Modelo | VRAM máx. (com nomic) | Pico soma 4 GPUs | Média | Camadas na GPU |
| --- | --- | --- | --- | --- |
| dupla qwen + 12b | ~27,9 GB | 239 W | — | 100% |
| dupla qwen + e4b | 22,8 GB | 219 W | — | 100% |
| 26b | 24,1 GB | 173,6 W | 151,2 W | 31/31 (o `/api/ps` mostra 7–16% "CPU": buffer `CUDA_Host`) |
| 31b | 25,4 GB | 192,5 W | 157,6 W | 100% |

Cargas a frio: 26b 44–55 s; 31b 36,5 s; e4b 13,9 s; 12b 19–21 s.

## 6. Tokens por job (pedido do owner)

Tabelas com uma linha por job (suíte, rótulo, tool, modelo, tokens de entrada/gerados/total, ms, tok/s, fila, `job=`):
`.workspace/bakeoff/out-f13-e4b/tokens-por-job.md`, `out-f13-12b/tokens-por-job.md`, `out-f13-26b/tokens-por-job.md`,
`out-f13-31b/tokens-por-job.md` (gerador: `scratchpad/tokjobs.py`).

| Execução | Jobs | Entrada | Gerados | Total |
| --- | --- | --- | --- | --- |
| qwen3-ia (dupla e4b) | 60 | 201.138 | 8.005 | 209.143 |
| gemma4:e4b | 60 | 205.252 | 10.347 | 215.599 |
| qwen3-ia (dupla 12b) | 60 | 201.156 | 8.400 | 209.556 |
| gemma4:12b | 60 | 205.498 | 11.694 | 217.192 |
| gemma4:26b qualidade | 60 | 205.510 | 11.212 | 216.722 |
| gemma4:26b 10 agentes | 30 | 111.996 | 11.401 | 123.397 |
| gemma4:31b qualidade | 60 | 205.544 | 12.805 | 218.349 |
| gemma4:31b 10 agentes | 30 | 23.837 | 3.299 | 27.136 |

## 7. Leitura pelo critério do owner (qualidade primeiro)

1. **Qualidade:** 12b, 26b e 31b empatam em 100% na numeração e na extração; o 31b é o único perfeito em conta direta; o qwen3-ia é o
   mais fraco em numeração (86%) e conta direta (0/15). O e4b não acrescenta nada ao qwen3-ia.
2. **Viabilidade operacional hoje (síncrono, 1 por vez, 120 s):** a dupla **qwen3-ia + 12b** cabe e não piora o qwen; o **26b** sozinho
   cabe com folga e completou 25/30 com 10 agentes; o **31b** só é viável com assíncrono/lotes (Fase 11), por ser ~4× mais lento.
3. **Formato:** os gemma4 embrulham o JSON em cerca de código; as tools toleram, mas a rota precisa considerar.
4. **Raciocínio:** todos acertam o passo a passo; medir `think:true` (falta a opção `-think` no bake-off) é o próximo teste de qualidade.

## 8. Mudanças feitas e desfeitas no servidor (todas com backup)

| Hora | Mudança | Situação final |
| --- | --- | --- |
| 14:52 | apagados `qwen3.5:9b` (nome duplicado), `gemma4-ia`, `gemma4:12b` | o nome `qwen3.5:9b` voltou e saiu de novo na restauração (só o `qwen3-ia` fica) |
| 14:53–16:25 | `gemma4:12b`, `e4b`, `12b` (32k por Modelfile), `26b`, `31b` baixados e apagados conforme cada teste | nenhum gemma4 no disco |
| 15:04–15:07 | 48k + `NUM_PARALLEL=2` (teste e reverter) | revertido, backup `10-performance.conf.bak-20261104-150423` |
| 15:23–18:05 | gateway aceitando o gemma4 em teste | restaurado (`--allowed-models qwen3-ia;nomic-embed-text`) |
| 16:30 | **`qwen3-ia` apagado** por ordem do owner (produção sem modelo até 18:07) | restaurado: `ollama pull qwen3.5:9b` + `cp` → `qwen3-ia`, **mesmo digest** |
| 16:34 | reinício do servidor pelo owner (falso alarme "degradado") | — |
| 18:05 | `OLLAMA_MAX_LOADED_MODELS` 3 → **2** (igual à cópia do repositório) | backup `10-performance.conf.bak-20261104-180536` |

## 9. Erros de processo (honestidade)

1. 16:30–18:07: a produção ficou sem `qwen3-ia` por decisão do owner; avisei o impacto antes.
2. 17:53: mandei um `summarize_file` ao modelo local durante o teste do 31b; entrou na fila e expirou aos 120 s (pedia o `qwen3-ia`,
   inexistente naquela hora). O owner pediu para não usar o Ollama durante os testes.
3. 18:03: ao parar o amostrador de watts, o `pkill -f` casou com a própria linha do SSH (o nome do arquivo estava no comando) e derrubou a
   sessão; refeito com exclusão da própria sessão. Sem efeito no servidor; dados completos (4.828 amostras).

## 9b. Acréscimo 18:40 — `qwen3.8:27b` (pedido do owner 18:12: "qwen3.8:27b + nomic-embed-text")

- Modelo: arquitetura **`qwen35`**, **27,3B denso**, Q4_K_M, 17 GB no disco, contexto máx. 262144, visão (`clip`). Carga a frio 33,2 s.
  A 64k + nomic: **66/66 camadas na GPU**, 19,6 GB no `/api/ps`, VRAM total ~24,1 GB. Teste 18:14–18:40; durante o teste, por ordem do
  owner (18:15), só ele e o nomic no disco; produção restaurada automaticamente no fim (`volta2.sh`: `qwen3-ia` baixado de novo, mesmo
  digest `56671c2ab938`, 100% GPU, gateway com a lista original, healthz 200).

| Teste | qwen3-ia | 12b | 26b | 31b | **qwen3.8:27b** |
| --- | --- | --- | --- | --- | --- |
| S2 números / número+título | 86,4% / ~85% | 100% / 100% | 100% / 100% | 100% / 100% | **100% / 100%** |
| S2 **itens inventados** | 19–20 | 20 | 20 | 20 | **0** |
| S2 JSON estrito | 5/5 | 0/5 | 0/5 | 0/5 | **5/5** |
| S3 ok / sem cerca | 100% / 18/18 | 100% / 14/18 | 100% / 12/18 | 100% / 12/18 | **100% / 18/18** |
| S6 conta direta | 0/15 | 6/15 | 9/15 | 15/15 | 0/15 |
| S6 passo a passo | 10/10 | 10/10 | 10/10 | 10/10 | 10/10 |
| S6 contagem direta / lista | 3/6 · 3/6 | 4/6 · 3/6 | 3/6 · 4/6 | 2/6 · 3/6 | 3/6 · 3/6 |
| Qualidade, 60 chamadas | ~11 min | ~16 min | ~12 min | ~39 min | **~12 min** |
| **10 agentes × 3** | — | — | 25/30 | 6/30 | **30/30** |
| p50 / p95 (10 agentes) | — | — | 228 / 277 s | 188 / 263 s | **163 / 187 s** |
| Geração agregada (10 agentes) | — | — | 15,3 tok/s | 3,6 tok/s | **22,6 tok/s** |
| S2 / S3 JSON nos 10 agentes | — | — | 100% / 15/15 | 100% / 4/4 | **330/330 · 15/15 estrito** |
| Energia pico / média | — | — | 174 / 151 W | 193 / 158 W | **235 / 203 W** |

- **Achado sobre o gabarito:** o `qwen3.8:27b` é o único modelo com **0 itens inventados** no S2; os itens `1.2, 1.2.1–1.2.3` que
  todos os outros listavam **não existem no documento** — a suspeita de gabarito incompleto (pendência 4) **não procede**: o gabarito
  está certo e os outros modelos inventavam.
- Ponto fraco: conta direta 0/15 (igual ao `qwen3-ia`); passo a passo 10/10. Energia média mais alta (denso calculando o tempo todo).
- Tokens por job: `.workspace/bakeoff/out-f13-qwen38/tokens-por-job.md` (60 de qualidade: 201.114 entrada / 9.707 gerados; 30 da carga:
  135.399 / 11.737).

## 9c. Acréscimo 2026-11-05 10:34 — `qwen3.8:27b`, conta direta com `think` ligado

Pedido do owner (10:28): "faça o teste com o think ligado para conta direta" (modelo escolhido: qwen3.8:27b). Script
`/root/stress13/think38.py` (cópia em `scratchpad/think38.py`), direto no Ollama, mesmo pedido da S6 P3 "direto" do bake-off
("Responda apenas com o número, sem texto: quanto é …?"), `temperature 0`, 5 contas × 3 repetições, 64k, 100% GPU (19,6 GB).
Controle `think:false` (`num_predict` 50) na mesma rodada; `think:true` com `num_predict` 8192. Resultados em
`/root/stress13/think38.json`.

| Modo | Acertos | Respostas | Raciocínio | Tokens gerados | Tempo por conta |
| --- | --- | --- | --- | --- | --- |
| `think:false` | **0/15** | 433, 552, 524, 880, 400 (iguais nas 3 repetições; os mesmos de ontem) | — | 4 | 0,5–0,9 s |
| `think:true` | **15/15** | 403, 564, 509, 940, 376 | 119–156 caracteres | 67–79 | **1,7–2,1 s** |

- O raciocínio curto resolve a conta direta por completo, com custo pequeno nesse tipo de pedido (~70 tokens e ~2 s), longe do
  "10–30× mais lento" medido para tarefas grandes na Fase 8.
- Produção restaurada às 10:34:47 (trap no fim do script): `qwen3.8:27b` apagado; `qwen3-ia` (já estava no disco) e nomic 100% GPU;
  `ollama-warm` success; gateway healthz 200. Durante o teste (10:29–10:34) o `qwen3-ia` ficou fora da memória.

## 10. Pendências

1. Implantar o gateway **0.13.2** (correção do falso alarme "fora da GPU"; ataques 138 PASS / 0 FAIL) — aguardando o owner.
2. Escolha do modelo da Fase 13 (dupla qwen + 12b, 26b, ou 31b só em lotes) — decisão do owner; Fase 13 não executada.
3. Opção `-think` no bake-off e medição com raciocínio.
4. ~~Conferir o gabarito do S2 (itens 1.2.x).~~ Resolvido em 18:40: o `qwen3.8:27b` acertou 22/22 sem inventar; o gabarito está certo.
5. Registrar este resultado no plano da Fase 13 (`.workspace/plans/MCPOllam2026-11-04-033446-fase13-rota-auto.plan.md`).
