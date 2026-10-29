# Report — Fase 8: comparativo (bake-off) de modelos locais para a finalidade do mcpollama

> **Data:** 2026-10-28, das 19:34 às 22:04. **Executor:** agente Opus 5.5. Avaliação cega da S1 em Opus (um avaliador por
> documento, verificação crítica); revisão de português da S4 em Sonnet.
> **Plano de origem:** `.workspace/plans/MCPOllam2026-10-28-193200-fase8-bakeoff-modelos.plan.md` (etapas 8.0–8.5).
> **Estado:** executado em **modo não extremo** depois de 6 reboots do servidor (§7). A S7 foi feita **por cálculo**,
> com validação pequena (2×8k). Aguarda a decisão do owner (**D8-3**). Sem commit; a 8.7 não foi executada.
> **Sigilo:** corpus **só com documentos do owner**, com ids neutros (`b01`…); nenhum trecho de documento neste report.

---

## 1. Resumo

| Modelo | S1: resumos com invenção | S1: invenções graves | S1: omissões | S2: nº+título | S3: formato | S4: pt-BR (0–10) | S5: prefill / geração (tok/s) | Carga a frio | S7 (por cálculo, 3 GPUs) | Estabilidade no hardware |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `qwen3-ia` (atual) | 11/25 (44%) | **6** | 34 | 81,8% | 18/18 | 6,5 | ~4.700 (8k) / ~89 | não medida (~32–36 s histórico) | nenhum alvo cabe | AER +0 em 10 min com 4 GPUs |
| **`gemma4:12b`** | **9/25 (36%)** | **0** | **3** | **100%** | 18/18 (4 com cerca) | **8,0** | ~1.140 / ~37 | 30,2 s | **os 3 alvos cabem** (base estimada) | AER +102 em 45 min com 4 GPUs |
| `qwen3.5:9b` | 9/20 (45%)¹ | 0 | 4 | 77,3% | 12/12¹ | 6,0 | ~3.200 (8k) / ~52 | 15,8–18,6 s | os 3 alvos cabem | **ligado a 4 dos 6 reboots**; AER +85 em 8 min |
| `granite4.2:8b` | 14/25 (56%) | 1 | 11 | 83,6% | 16/18 | 4,0 | ~3.300 (8–12k) / ~63 | 10,6–22,7 s | só 5×32k | AER subiu até o 6º reboot com 4 GPUs; +0 sem a 03 |
| `qwen3:14b` | 16/25 (64%) | **9** | 22 | 81,8% | 18/18 | 4,5 | ~2.100 (8–12k) / ~40 | 19,6 s | só 5×32k (sem 64k: nativo 40k) | AER +5 em 13 min (3 GPUs sem 06) |

¹ `qwen3.5:9b`: o documento `b03` (prefill acima de 8k) foi pulado por risco, por ordem do owner. Sem esse documento, a
amostra perde o texto em que todos os modelos mais omitiram.

- **Critério §5 do plano:** **todos os 5 modelos são eliminados** pela regra estrita da S1 (invenção em mais de 1 de 25
  resumos), inclusive o atual. A S2 (< 95%) elimina 4 dos 5 e a S3 elimina o `granite4.2:8b`. A regra estrita não
  separa os modelos neste corpus, então o ranking segue a ordem do plano (S1 → S2 → S4 → S5), com a capacidade como
  critério secundário.
- **Ranking:** 1º **`gemma4:12b`** · 2º `qwen3-ia` · 3º `qwen3.5:9b` (fora por estabilidade) · 4º `granite4.2:8b` · 5º `qwen3:14b`.
- **Recomendação:** **por ora, manter o `qwen3-ia`**. O `gemma4:12b` é o candidato à troca: é o único com 0 invenção
  grave, quase sem omissões, 100% na S2 e o melhor português. A troca (8.6) depende de três condições, decididas
  pelo owner (§6):
  - o mcpollama passar a enviar `think:false`;
  - revalidar o `gemma4` na configuração final de GPU;
  - aceitar latência 3 a 5 vezes maior.

---

## 2. Método e configuração

### 2.1 Ferramenta (8.0)
- `src/cmd/bakeoff/` (nova, versionável, **sem commit**), que reaproveita os drivers `stressdrv` e `acceptdrv`. Ela:
  - sobe o mcpollama com `--model <m> --allowed-models "<m>;nomic-embed-text" --timeout 5m`;
  - roda as suítes `load`, `ps` e `s1`…`s6`, mais os modos offline `ref`, `score` e `blind`;
  - grava as respostas dos modelos em `.workspace/bakeoff/out/<modelo>/` (ignorado pelo git) e manda só métricas para o stdout;
  - lê o token do `.env` e o apaga de toda saída.
- **Travas acrescentadas depois dos reboots** (ordens do owner):
  - aborta o processo inteiro na **1ª chamada com erro** (código 3);
  - antes de cada bloco, confere `uptime -s` e o AER de `00:1b.4` por SSH; se o boot mudar, para (código 4);
  - um modelo por vez, sem laço;
  - `-max-prefill-tokens` pula (`pulado-risco`) chamadas cujo prefill estimado passa do teto, com estimativa
    conservadora de 3 caracteres por token;
  - `-s5-sizes` e `-s5-gen`.
- **Gates:** `go vet`, `gofmt` e `go test -count=1 ./...` passam em todos os pacotes (22:04); os testes do pacote
  cobrem a pontuação S2/S3/S4, a triagem da S1, a trava de risco e a recusa do binário de `D:\MCP`.
- **Binários de teste**, num caminho temporário (nada em `D:\MCP`):
  - `bakeoff-bd71b7a`: HEAD, como está;
  - `bakeoff-bd71b7a-nothink`: HEAD mais `"think": false` no corpo de `/api/generate` e `/api/chat`, compilado de uma
    **cópia** de `src/`. O patch está em `.workspace/bakeoff/variante-nothink.patch`; o `src/` do repositório não foi alterado.

### 2.2 Variante `think:false` (aprovada pelo owner para o bake-off)
`gemma4:12b`, `qwen3.5:9b` e `qwen3:14b` vêm com **raciocínio ("thinking") ligado por padrão** (`ollama show`); o
`granite4.2:8b` também tem essa capacidade. O mcpollama HEAD não envia `think`, então o modelo raciocina e o texto do raciocínio
fica fora da resposta, mas conta no tempo e no `eval_count`.

**Sondas com o HEAD (como está):**
- `gemma4:12b`: S2 com **2 de 2 timeouts de 300 s**. Um diagnóstico direto, de uma chamada, mostrou **300 s só
  raciocinando** (16.978 caracteres, resposta vazia), sem decidir entre o sumário desatualizado e a numeração real.
  Com `think:false`, a mesma tarefa levou 42 s. O resumo curto (b02) passou em 22 s;
- `qwen3.5:9b`: S2 em 25,6 s e S1 de b02 em 56,5 s, com 2.962 tokens para resumir 2 mil caracteres.

**Para comparar de forma justa, as suítes completas dos 4 candidatos usaram a variante.** O `qwen3-ia` não tem
thinking e rodou com o HEAD.

### 2.3 Corpus (8.1)
- Origem: `D:\Users\claiton.linhares\Documents`, só leitura, copiado para `.workspace/bakeoff/corpus/` (ignorado; conferido
  com `git check-ignore`). O mapa de ids fica só em `.workspace/bakeoff/mapa.txt`.
- Documentos usados:
  - **S1:** `b01` (docx, modelo de especificação, 17 mil caracteres), `b02` (pptx, 9 slides, 2 mil), `b03` (pdf, 15
    páginas, 31 mil), `b08` (pdf em inglês, 7,8 mil) e `b09` (pdf técnico, 4,5 mil);
  - **S2:** `b01`;
  - **S3:** `b01`, `b02` e `b03`;
  - **S4:** trechos em `.txt`. `b01-trecho` e `b09-texto` foram traduzidos para o inglês e `b08-trecho` para pt-BR;
  - **S5:** prompts sintéticos;
  - **S6:** `README.md` e `TOOLS.md` do repositório.
- **Removidos do corpus (aprovado pelo owner):** `b05` (lista de nomes de terceiros), `b06` (PDF com fonte sem mapa
  Unicode: texto ilegível) e `b10` (docx sem texto). `b04` e `b07` foram copiados, mas não usados: são grandes demais
  para o modo não extremo.
- **Gabarito da S2:** Word COM, somente leitura e invisível. O `ListString` dos parágrafos numerados dá **22 seções
  numeradas** (títulos de nível 1 a 3); ficam fora 3 parágrafos numerados vazios e 3 marcadores. O `Quit` falhou
  (assinatura COM), e foi encerrado só o PID aberto (40652).
- **Referência da S1:** o texto que o próprio `doctext` extrai, ou seja, o que o modelo recebe
  (`out/ref/<id>.txt`).

### 2.4 Configuração de GPU de cada medição
| Sigla | Config | Período | Medições |
| --- | --- | --- | --- |
| **4G-150** | 4 GPUs, 150 W | 19:52–20:50 | `qwen3-ia`: S1–S6 e S5 (8/16/32k). `gemma4:12b`: tudo. `qwen3.5:9b`: sondas HEAD e prefill de 8k |
| **4G-130** | 4 GPUs, 130 W | 20:59–21:10 | `qwen3.5:9b`: carga e prefill de 8k ×2 e 12k ×1. `granite4.2:8b`: carga, S5 8k e geração, S2, S3 de b01 |
| **3G-s03** | 3 GPUs sem a `03:00.0`, 130 W | 21:15–21:19 | `granite4.2:8b`: carga, S5 8k, S3 b03, S4, S1 b01 r1–r4. **Arquivado** (`out/granite4.2_8b/3gpu-sem03/`) e refeito na 3G-s06 por ordem do owner |
| **3G-s06** | 3 GPUs sem a `06:00.0` (com a `03:00.0`), 130 W | 21:22–21:58 | `granite4.2:8b`: S3 b02/b03, S1, S4, S6, S5 8k/12k. `qwen3:14b`: tudo. `qwen3.5:9b`: S2–S4, S1, S6 e geração. S7: validação 2×8k |

As 5 execuções tiveram `isError=0`, 0 erro de protocolo, 0 HTTP 429 e **`queue_ms` = 0 em todas as chamadas
sequenciais**: sem anomalia. O servidor estava dedicado ao teste desde 19:40, sem outras IAs em uso (owner).

---

## 3. Resultados por suíte

### 3.1 S1: fidelidade de resumo (a mais importante)
**Método:**
- Rubrica em `.workspace/bakeoff/rubrica-s1.md`, repetível por outro avaliador.
- Pedido ao modelo: `summarize_file` com `max_words` 150, 5 repetições por documento.
- Avaliação **cega**: pacotes com rótulos aleatórios, um por documento (`out/blind/<doc>.txt`); a chave, em
  `<doc>.key.tsv`, só foi aberta depois.
- Um avaliador Opus por documento:
  - fixou de 3 a 5 pontos centrais **antes** de ler os resumos;
  - contou invenções (graves ou leves) e omissões de pontos centrais;
  - registrou as dúvidas;
  - "na dúvida sem passagem que sustente, é invenção".
- **Checagem automática** (números e palavras capitalizadas ausentes da fonte) como pista para o avaliador.
- **Conferência adversarial deste agente:** uma invenção grave sorteada (b02, do `qwen3-ia`) foi relida contra a
  fonte. O veredito procede: o resumo trocou o sujeito de um episódio narrado.
- Os vereditos e as justificativas estão em `.workspace/bakeoff/aval/<doc>.md`.

| Modelo | Resumos | Com invenção | Invenções | Graves | Omissões | b01 | b02 | b03 | b08 | b09 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `qwen3-ia` | 25 | 11 | 17 | 6 | 34 | 2/5 | **5/5** | 2/5 | 2/5 | 0/5 |
| `gemma4:12b` | 25 | **9** | **13** | **0** | **3** | 1/5 | 4/5 | 2/5 | 2/5 | 0/5 |
| `granite4.2:8b` | 25 | 14 | 22 | 1 | 11 | 2/5 | 2/5 | 3/5 | **5/5** | 2/5 |
| `qwen3:14b` | 25 | 16 | 22 | **9** | 22 | 4/5 | 4/5 | 1/5 | 4/5 | 3/5 |
| `qwen3.5:9b` | 20 | 9 | 11 | 0 | 4 | 1/5 | 3/5 | pulado | 5/5 | 0/5 |

(colunas b0x = resumos com invenção / 5)

Observações:
- Padrões gerais:
  - **Nenhum modelo** passa a regra "invenção em no máximo 1 de 25";
  - o documento curto (b02) induz invenções em todos: o modelo "completa" a apresentação;
  - o b09 (diagnóstico técnico) é o mais seguro.
- Por modelo:
  - `qwen3-ia`: tem 6 invenções graves, 5 delas no b02 (sujeito trocado na história de origem), e o maior número de
    omissões (34). Seus resumos são os mais curtos (mediana de 104 palavras);
  - `granite4.2:8b`: **ignorou o limite de 150 palavras em 25 de 25 resumos** (mediana de 251, máximo de 415). Isso é
    falha de instrução, não de fidelidade;
  - os demais respeitaram o limite, exceto 1 de 25 do `qwen3:14b`.
- **Dúvidas registradas pelos avaliadores:** 10 (b01), 12 (b02), 12 (b03), 9 (b08) e 7 (b09). As duas do b08 que mais
  mudariam a contagem estão em `aval/b08.pdf.md`; foram decididas a favor do modelo.

### 3.2 S2: seções numeradas contra o Word (22 itens, 5 execuções)
| Modelo | Nº presente | **Nº + título certos** | JSON válido | JSON sem cerca | Nºs fora do gabarito |
| --- | --- | --- | --- | --- | --- |
| `qwen3-ia` | 86,4% | 81,8% | 5/5 | 5/5 | 20 |
| `gemma4:12b` | **100%** | **100%** | 5/5 | 0/5 (cerca ```json) | 20 |
| `granite4.2:8b` | 87,3% | 83,6% | 5/5 | 5/5 | 16 |
| `qwen3:14b` | 86,4% | 81,8% | 5/5 | 5/5 | 20 |
| `qwen3.5:9b` | 79,1% | 77,3% | 5/5 | 5/5 | 19 |

**Armadilha do sumário desatualizado (achado relevante para contratos).** O `b01` tem um sumário estático com
numeração antiga (1.1, 1.2.x) que não bate com a numeração real do Word (1.1, 1.1.x). Todos os modelos copiaram os
números do sumário. Daí vêm os "números fora do gabarito" (4 por execução): não são números inventados do nada, mas
números errados para o documento.
- Os outros modelos trocaram a numeração real pela do sumário.
- O `gemma4` listou **as duas** (numeração real completa e certa, mais as entradas do sumário).

Para quem cita cláusulas, os dois comportamentos exigem conferir os números no original.

### 3.3 S3: formato (3 formatos × b01/b02/b03 × 2)
| Modelo | No formato pedido | Estrito (sem cerca) | JSON / lista / tabela |
| --- | --- | --- | --- |
| `qwen3-ia` | 18/18 | 18/18 | 6/6 · 6/6 · 6/6 |
| `gemma4:12b` | 18/18 | 14/18 | 6/6 · 6/6 · 6/6 |
| `granite4.2:8b` | **16/18 (88,9%)** | 16/18 | 5/6 · 6/6 · 5/6 (1 JSON inválido; 1 tabela com texto fora, ambos no b01 com 4 GPUs) |
| `qwen3:14b` | 18/18 | 18/18 | 6/6 · 6/6 · 6/6 |
| `qwen3.5:9b` | 12/12 executadas | 12/12 | b03 pulado por risco (6 itens) |

### 3.4 S4: português e tradução
- **Estrutura (automático):**
  - os 5 modelos preservaram 100% dos marcadores e dos identificadores do b01-trecho;
  - no b09 e no b08, entre 33 e 35 de 37 e 11 de 14. As "perdas" são traduções legítimas (CI/IC) e números de nota
    colados ao texto;
  - comprimento entre 0,95 e 1,11 do original em todos: nada foi truncado;
  - a razão de linhas menor que 1 é só a junção das quebras rígidas do PDF.
- **Português (b08, inglês → pt-BR):** revisão cega em Sonnet, notas 0–10 (`aval/s4-ptbr.md`):

| Modelo | Fidelidade | Português | Estrutura | **Nota** | Justificativa (resumo) |
| --- | --- | --- | --- | --- | --- |
| `gemma4:12b` | 8,5 | 7,5 | 9 | **8,0** | a mais fiel e natural; falhas pontuais e marcas de pt-PT |
| `qwen3-ia` | 6,5 | 6,5 | 8 | 6,5 | fluente; traduziu o nome próprio de uma série, teve 2 decalques e 1 erro de gênero |
| `qwen3.5:9b` | 6 | 6 | 8 | 6,0 | erros de sentido em expressões idiomáticas e erro de gênero |
| `qwen3:14b` | 5 | 5 | 8 | 4,5 | erros de sentido, termo deixado em inglês e erros de concordância |
| `granite4.2:8b` | 4 | 5 | 6 | 4,0 | erros de sentido, acréscimo inventado, palavra inexistente e linhas fundidas |

### 3.5 S5: desempenho (modo não extremo: prefill de 8k e 12k; 16k e 32k só no `qwen3-ia` e no `gemma4`, antes dos reboots)
| Modelo | Config | Prefill 8k (tok/s) | 12k | 16k / 32k | Geração 1.000 tok (tok/s) | Carga a frio | VRAM total 1×64k (monitor) | Contexto |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `qwen3-ia` | 4G-150 | 4.717–5.012 | — | 4.563–5.001 / 3.822–4.098 | 85,8–92,4 | — | 23,4 GB (4G) · 21,2 GB (3G) | 65.536 |
| `gemma4:12b` | 4G-150 | 1.130–1.156 | — | 1.167–1.185 / 1.108–1.111 | 36,8–37,5 | 30,2 s | 18,7 GB (4G) | 65.536 |
| `qwen3.5:9b` | 4G-150 / 4G-130 | 2.754–2.830 / 3.209–3.454 | 4.002 (1 de 2: reboot) | proibido | 52,3–52,6 (3G-s06) | 18,6 s (4G-150) · 16,6 s (4G-130) · 15,8 s (3G-s06) | 14,3 GB (4G) · 12,4 GB (3G) | 65.536 |
| `granite4.2:8b` | 4G-130 / 3G-s06 | 2.538–2.858 / 3.185–3.452 | 3.226–3.415 | — | 62,2–63,3 (4G-130) | 22,7 s · 19,0 s (s03) · 10,6 s (s06) | 15,1 GB (4G) · 11,2 GB (3G) | 65.536 |
| `qwen3:14b` | 3G-s06 | 2.078–2.117 | 2.139–2.302 | — | 39,5–40,0 | 19,6 s | 13,1 GB (3G) | **40.960** (nativo) |

- O prefill é aproximado por `prompt_eval_count / (total_ms − load_ms)` (inclui de 9 a 16 tokens gerados), e a geração
  por `eval_count / (total_ms − load_ms)`.
- 100% GPU em todas as cargas observadas, exceto o `nomic-embed-text` em 3 GPUs, que fica em 74%/26% CPU/GPU.
- **Latência típica nas tools (mediana):**

| Modelo | `summarize_file` | `extract` S2 | `translate` |
| --- | --- | --- | --- |
| `qwen3-ia` | 2,6 s | 6,9 s | 14,3 s |
| `qwen3.5:9b` | 4,7 s | — | — |
| `qwen3:14b` | 6,3 s | — | — |
| `granite4.2:8b` | 10,6 s | — | — |
| `gemma4:12b` | 11,9 s | 42 s | 51 s |

### 3.6 S6: limites conhecidos (informativo, não decide)
| Modelo | Conta direta | Conta passo a passo | Contagem direta | Contagem por lista JSON |
| --- | --- | --- | --- | --- |
| `qwen3-ia` | 3/15 | 10/10 | 3/6 | 6/6 |
| `gemma4:12b` | 6/15 | 10/10 | 2/6 | 4/6 |
| `granite4.2:8b` | 0/15 | 9/10 | 0/6 | 4/6 |
| `qwen3:14b` | 6/15 | 10/10 | 3/6 | 6/6 |
| `qwen3.5:9b` | 0/15 | 10/10 | 3/6 | 3/6 |

A mitigação documentada (passo a passo com `RESULTADO:`) vale para todos. Nenhum candidato muda o quadro.

### 3.7 S7: capacidade simultânea × contexto (por cálculo, mais validação pequena)
**Fórmula:** VRAM(N, ctx) = base (1 requisição, medida no monitor) − KV(64k) + KV/token × N × ctx + fixo/sequência
× (N − 1) + 1 GB de margem para buffers de computação. Considera-se usável 95% da VRAM (24 GB com 3 GPUs; 32 GB com 4).
Valores de **KV/token** lidos do journal do Ollama, com cache KV em q4_0:

| Modelo | KV/token | Fixo por sequência |
| --- | --- | --- |
| `qwen3-ia` | 27,0 KiB | — |
| `gemma4` | 4,5 KiB | 270 MiB (janela SWA de 3.072 células × 40 camadas) |
| `qwen3.5` | 9,0 KiB | 50 MiB de estado recorrente (arquitetura híbrida) |
| `granite` | 45 KiB | — |
| `qwen3:14b` | 45 KiB | — |

Script em `.workspace/bakeoff/s7calc.py`.

| Modelo | Config | 5×32k | 5×64k | 10×32k |
| --- | --- | --- | --- | --- |
| `qwen3-ia` | 3 GPUs | não cabe (24,3 GB) | não cabe (28,5) | não cabe (28,5) |
| `qwen3-ia` | 4 GPUs | **cabe** (26,4; sobra 4,0) | não cabe (30,6; falta 0,2) | não cabe (30,6) |
| `gemma4:12b` | 3 GPUs* | cabe (18,3) | cabe (19,0) | cabe (20,3) |
| `gemma4:12b` | 4 GPUs | cabe (20,7) | cabe (21,4) | cabe (22,8) |
| `qwen3.5:9b` | 3 GPUs | cabe (14,2) | cabe (15,6) | cabe (15,8) |
| `qwen3.5:9b` | 4 GPUs | cabe (16,0) | cabe (17,4) | cabe (17,6) |
| `granite4.2:8b` | 3 GPUs | cabe (16,2) | não cabe (23,2; falta 0,4) | não cabe (23,2) |
| `granite4.2:8b` | 4 GPUs | cabe (20,0) | cabe (27,0) | cabe (27,0) |
| `qwen3:14b` | 3 GPUs | cabe (19,1) | **não suporta** (nativo 40k) | não cabe (26,1) |
| `qwen3:14b` | 4 GPUs* | cabe (21,5) | não suporta | cabe (28,5) |

\* Base estimada: o `gemma4` não foi carregado em 3 GPUs nem o `qwen3:14b` em 4. A estimativa usa a diferença de
1,8 a 3,9 GB observada nos outros modelos.

- **Validação pequena (3G-s06), feita no `granite4.2:8b` e no `qwen3:14b`:**
  - ambiente: `99-bakeoff.conf` temporário com `OLLAMA_NUM_PARALLEL=2`; `ollama-warm` parado antes do restart e o
    drop-in removido no fim do mesmo script;
  - conferência de boot antes de cada carga.

| Modelo | VRAM (2 slots) | Previsto | Resultado | Parede | Prefill agregado | Pico somado | AER `00:1b.4` |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `granite4.2:8b` | 13,8 GB, 2×64k (KV de 5.760 MiB = 2 × 2.880, confirma os 45 KiB/token) | 14,1 GB | 2/2 ok | 9,3 s (p95 9,3 s) | 1.709 tok/s | 385 W | 96 → 99 |
| `qwen3:14b` | 14,5 GB, 2×**40k**: o Ollama limitou `num_ctx` 65536 a 40.960 ("requested context size too large") | — | 2/2 ok | 14,6 s (p95 14,6 s) | 1.091 tok/s | 392 W | 99 → 117 |

- **Conclusão da S7:**
  - nenhum modelo atinge **10×32k** nem **5×64k** com o `qwen3-ia` atual; com 4 GPUs, ele atinge só **5×32k**;
  - o `gemma4:12b` e o `qwen3.5:9b` (KV pequeno: só 8 camadas de atenção global) atingem **os três alvos** por cálculo,
    até com 3 GPUs;
  - o `granite4.2:8b` atinge os três só com 4 GPUs;
  - o `qwen3:14b` não faz 64k.
  - Ressalva: não houve carga simultânea pesada (modo não extremo). Os números são projeções, e o pico de potência
    com N alto não foi medido.

---

## 4. Critério de decisão (§5 do plano)

1. **Eliminação:**
   - **S1 (invenção em mais de 1 de 25):** elimina os 5, inclusive o atual. O melhor caso, o `gemma4`, tem 9 de 25;
   - **S2 (< 95% de números certos):** elimina `qwen3-ia`, `granite`, `qwen3:14b` e `qwen3.5`. O `gemma4` passa, com
     100% de número e título;
   - **S3 (< 95%):** elimina o `granite4.2:8b` (88,9%).
   - **Conclusão:** a régua estrita foi calibrada para um modelo que não existe neste corpus. Como ninguém passa,
     aplica-se o passo 2 (ordem S1 → S2 → S4 → S5) a todos, deixando claro que **nenhum** modelo dispensa a
     conferência dos fatos críticos no original.
2. **Ranking:**
   1. **`gemma4:12b`:** a melhor S1 (menos resumos com invenção, 0 grave, 3 omissões), a melhor S2 (100%) e a melhor
      S4 (8,0). Contra: a S5 mais lenta (latência 3 a 5 vezes maior);
   2. **`qwen3-ia`:** S1 intermediária, mas com 6 graves e 34 omissões; S2 de 81,8%; S4 de 6,5; de longe a melhor S5;
   3. **`qwen3.5:9b`:** S1 boa (0 grave, 4 omissões), mas sem o documento mais difícil; S2 de 77,3%; S4 de 6,0.
      **Não recomendável neste hardware:** está ligado a 4 dos 6 reboots e fez o AER da `03:00.0` subir muito;
   4. **`granite4.2:8b`:** S1 pior (14 de 25) e fora do limite de palavras; S3 de 88,9%; S4 de 4,0;
   5. **`qwen3:14b`:** a pior S1 (16 de 25 e 9 graves); S4 de 4,5; não faz 64k.
3. **Capacidade (critério secundário):** confirma a ordem. O `gemma4` atinge os três alvos do owner por cálculo, e o
   `qwen3-ia`, nenhum com 3 GPUs.

## 5. Recomendação

**Manter o `qwen3-ia` como padrão agora** e tratar o **`gemma4:12b` como candidato à troca (D8-3)**, desde que:
1. **O mcpollama passe a enviar `think:false`.** É mudança de código no `src/ollama` (hoje o OLL-010 só ignora o
   campo de raciocínio). Sem isso, o `gemma4` estoura o timeout nas extrações (2 de 2 a 300 s). **Achado para a
   produção: adotar qualquer modelo com thinking exige o mcpollama enviar `think:false`.**
2. **Revalidar o `gemma4:12b` na configuração final de GPU.** Ele só foi medido com 4 GPUs a 150 W, e o AER subiu
   +102 em 45 min com ele. A revalidação deve incluir S1 com N maior e com documentos do tipo-alvo (contratos, se o
   owner autorizar a D8-2), S2 em documento com numeração automática e um stress curto de 10 min na config final.
3. **O owner aceitar a latência:** mediana de 11,9 s para resumir e 42 s para a S2, contra 2,6 s e 6,9 s do
   `qwen3-ia`. Também cai a geração (37 contra 89 tok/s) e o prefill (1,1 mil contra 4,7 mil tok/s). O ganho é de
   fidelidade: 0 contra 6 invenções graves e 3 contra 34 omissões em 25 resumos. O ganho de capacidade só existe por
   cálculo.

Se o owner preferir velocidade, o `qwen3-ia` continua, com a regra atual de conferir no original os números e as
cláusulas citadas.

---

## 6. Achados para a produção

1. **`think:false` é pré-requisito** para qualquer modelo com thinking (§5.1).
2. **Sumário desatualizado engana todos os modelos** na extração de numeração (§3.2). Vale um aviso na descrição da
   tool `extract`, ou um alerta do `doctext` quando o sumário estático diverge da numeração real (proposta; decisão
   do owner).
3. O **`granite4.2:8b` ignora `max_words`** (25 de 25).
4. **`nomic-embed-text` com 3 GPUs:** não cabe junto com o `qwen3-ia` (74%/26% CPU/GPU; cerca de 2,3 s por embed).
5. **`gemma4:12b` carrega também um modelo auxiliar** (`gemma4-assistant`, de decodificação especulativa) e usa buffers de
   computação grandes (cerca de 7 GB com 4 GPUs). A VRAM real (18,7 GB) é bem maior que o "8,1 GB" do `ollama ps`.

## 7. Incidentes: reboots do servidor e falhas de supervisão

### 7.1 Reboots
A contagem e o diagnóstico são do coordenador, que investigou no servidor. A associação com as chamadas é deste
agente (`calls.jsonl`, monitor e journal).

| # | Quando | O que rodava | Config | Evidência |
| --- | --- | --- | --- | --- |
| 1 | ~20:50:00 | `qwen3.5:9b` (variante): S5, prefill de ~16 mil tokens, chamada às 20:49:55,7 | 4G-150 | monitor às 20:49:56: GPUs a 83–100% de uso, ~281 W somados; AER 223, não-corrigível 0; Xid 0 |
| 2 | ~20:51 | carga do `qwen3.5:9b` (`load_tensors` às 20:50:52–56), disparada pela minha chamada de 16k (20:50:41,7) | 4G-150 | journal; corte seco |
| 3 | ~20:52:5x (inferido) | minha chamada de 32k (20:51:57,8), enviada **depois** de o servidor voltar (20:51:35); nova carga às 20:52:05; conexão encerrada às 20:52:59 | 4G-150 | inferido pela queda da conexão e pela nova carga; **confirmar com `journalctl --list-boots`** |
| 4 | ~20:53 | carga do `qwen3.5:9b` às 20:53:19, disparada pela minha 2ª chamada de 32k (~20:53:00,5); boot seguinte às 20:54:03 | 4G-150 | journal |
| 5 | ~20:59:48 | `qwen3.5:9b`: início do prefill de 12k (20:59:47,2), já a 130 W e com o driver corrigido (abortou na 1ª falha) | 4G-130 | monitor de 1 s: ~200 W somados na última amostra; AER 3; boot seguinte às 21:00:18 |
| 6 | ~21:10:15 | `granite4.2:8b`: S3 do b03, prefill estimado de ~7,8 mil tokens. A chamada começou por volta de 21:10:14,8 e não foi registrada porque o processo foi encerrado. **Correção:** não era uma chamada pequena de ~1k | 4G-130 | **AER de 3 a 314 em 10 min** (60 → 314 só durante o `granite`); boot às 21:10:46 |

- **Causa provável (coordenador, depois do 6º):** o **link PCIe da GPU `03:00.0` (porta `00:1b.4`)**. A fonte
  Cougar de 850 W no limite é fator agravante, não a causa principal.
- **Evidência de AER por config (este agente):**

| Config | Período | Modelo | AER corrigível de `00:1b.4` |
| --- | --- | --- | --- |
| 4G-150 | ~10 min | `qwen3-ia`, inclusive prefill de 32k | +0 (110 → 110) |
| 4G-150 | 45 min | `gemma4:12b` | **+102** (111 → 213) |
| 4G-150 | ~1 min | `qwen3.5:9b` | +5, e reboot |
| 4G-130 | — | `qwen3-ia` | +8 |
| 4G-130 | 3 min | `granite` | +254, e reboot |
| 3G-s03 (sem a 03) | 4 min de `granite`, inclusive o mesmo b03 de ~8k | `granite4.2:8b` | **0 → 0**, sem reboot |
| 3G-s06 (com a 03) | ~11 min | `granite4.2:8b` | 1 → 1 |
| 3G-s06 (com a 03) | 13 min | `qwen3:14b` | 1 → 6 |
| 3G-s06 (com a 03) | 8 min | **`qwen3.5:9b`** | **11 → 96** |
| 3G-s06 (com a 03) | ~3 min | S7 (`granite`, `qwen3:14b`) e restauração | 96 → 124 |

Não-corrigível, fatal e Xid ficaram em 0 em todas as leituras.

**Leitura:** os erros acompanham o uso da `03:00.0` e crescem com cargas que trafegam mais pelo link. Sem a `03:00.0`,
nenhum erro. O `qwen3-ia` gerou poucos erros no mesmo link. Não se testou o `qwen3.5` em 3G-s03.

### 7.2 Falhas de supervisão deste agente (honestidade)
1. **A S5 do driver não parava em erro** e o laço `run-candidatos.sh` só parava por alerta de Xid ou AER
   não-corrigível, que um corte seco não gera. Depois do 1º reboot, o driver mandou chamadas **novas** (16k r2, 32k
   r1 e 32k r2): **pelo menos 2 delas depois de o servidor voltar**. Elas dispararam cargas do `qwen3.5` ligadas aos
   reboots 2 a 4. Em seguida o laço passou ao `granite4.2:8b` (2 chamadas de carga por volta de 20:54:07 e 20:54:25;
   o journal mostra o `granite` carregado às 20:54:29).
2. **O monitor SSH caiu no 1º reboot** e a trava baseada nele nunca disparou. Não houve retry automático (nem o
   cliente do mcpollama nem o driver reenviam), mas o efeito foi parecido.
3. **Correções aplicadas** antes de qualquer chamada nova, por ordem do owner:
   - abortar o processo inteiro na 1ª falha (testado: código 3);
   - `uptime -s` mais AER por SSH antes de cada bloco (código 4 se o boot mudar);
   - um modelo por vez, sem laço;
   - teto de prefill por estimativa;
   - monitor a ~1 s;
   - trava extra de AER acima de 100 durante o `qwen3.5` (não disparou: chegou a 94).
4. O reboot nº 5 aconteceu **com as correções já ativas**. O driver abortou na 1ª falha e não mandou mais nada.

## 8. Ameaças à validade
1. **N pequeno:** 25 resumos por modelo (20 no `qwen3.5`), 5 documentos e 1 avaliador por documento, com 7 a 12
   dúvidas registradas por documento. As diferenças grandes (0 contra 6 a 9 graves; 3 contra 34 omissões) são mais
   firmes que as pequenas (9 contra 11 resumos com invenção).
2. **Corpus só do owner, sem contratos:** a finalidade declarada são contratos em pt-BR. O `b01` é um modelo
   (template), o que favorece a invenção "isto é um sistema real".
3. **Configurações de GPU diferentes entre modelos** (§2.4): 150 W contra 130 W e 4 contra 3 GPUs. A S5 não é
   diretamente comparável entre linhas; a S1–S4 depende pouco disso (temperatura 0,2 nas tools de arquivo).
4. **Variante `think:false`** só nos candidatos; o `qwen3-ia` rodou o HEAD, que é equivalente porque ele não tem
   thinking.
5. **64k espalhado em 3 ou 4 GPUs** com `SCHED_SPREAD`; o desempenho em 1 GPU por instância (8a) não foi medido.
6. **A S7 é cálculo:** a margem de 1 GB para os buffers com N maior é estimada; as bases do `gemma4` com 3 GPUs e do
   `qwen3:14b` com 4 GPUs também.
7. **O `qwen3.5` ficou sem o `b03`** na S1 e na S3, e sem a S5 acima de 8k.
8. **Avaliação por IA:** o avaliador é um modelo (Opus), cego quanto ao modelo avaliado. A amostra manual do owner
   prevista no plano não foi feita.

## 9. Estado final do servidor (22:00)
- **Modelos:** `qwen3-ia` residente, 100% GPU, 64k, 21 GB; `nomic-embed-text` carregado em 74%/26% CPU/GPU; os
  candidatos foram descarregados (keep_alive 0).
- **Configuração:** `OLLAMA_NUM_PARALLEL=1`, `CONTEXT_LENGTH=65536`, `MAX_QUEUE=8`; `99-bakeoff.conf` removido (só o
  `10-performance.conf` e o seu `.bak`); 130 W nas 4 placas; `CUDA_VISIBLE_DEVICES` = 01, 03 e 05 (config 3G-s06 do
  owner, mantida).
- **Saúde:** boot às 21:10:46; AER de `00:1b.4` em 124 corrigíveis, não-corrigível 0; Xid 0.
- **Monitor:** encerrado.
- **Ficaram no servidor** (inofensivos): `/root/bakeoff/` (`pull.sh`, `pull.log`, `stop-monitor`) e `/root/bakeoff/s7/`
  (`s7load.py`, `load.json` e 2 CSVs de potência).
- **Disco:** 141 GB livres depois do pull de 27 GB; nenhum modelo foi apagado (a 8.7 depende do owner).

## 10. Artefatos
- **Código:** `src/cmd/bakeoff/` (`main.go`, `rpc.go`, `suites.go`, `score.go`, `offline.go`, `main_test.go`) e
  `.gitignore` (+ `/.workspace/bakeoff/`). Nada commitado.
- **`.workspace/bakeoff/`** (ignorado pelo git):
  - entrada: `corpus/`, `mapa.txt` e `gab/s2-word.tsv`;
  - método: `rubrica-s1.md`, `variante-nothink.patch` e `s7calc.py` / `s7calc-out.md`;
  - saídas dos modelos: `out/<modelo>/` (inclui `calls.jsonl`, por chamada) e `out-asis/` (sondas com o HEAD);
  - avaliação: `out/blind/` (pacotes cegos e chaves), `aval/` (vereditos) e `score-all.txt`;
  - servidor: `srv/`, com os scripts e as saídas de SSH e os 5 logs do monitor;
  - incidentes: `estado-parada-2026-10-28-2055.md`.

## 11. Pendências (numeradas)
1. **D8-3 (owner):** trocar ou não o padrão; ver as condições em §5.
2. **Reparo físico do link `00:1b.4`** (GPU `03:00.0`) para voltar a 4 GPUs com segurança. Até lá, a config 3G-s03
   foi a única com 0 AER.
3. **Mudança de código (owner):** o mcpollama enviar `think:false` (pré-requisito para qualquer modelo com thinking).
4. Revalidar o candidato escolhido na config final, com S1 ampliada (D8-2: contratos?) e stress de 10 min.
5. Confirmar os horários dos 6 boots com `journalctl --list-boots` (o 3º é inferido).
6. Medir o `qwen3.5:9b` na config sem a `03:00.0`, se ainda houver interesse.
7. **D8-4 e D8-5** (embeddings e `translategemma`): não executados.
8. **8.7 (limpeza dos perdedores):** não executada; depende do owner. Hoje há cerca de 27 GB de candidatos em disco.
9. Commit da ferramenta `src/cmd/bakeoff/` e do `.gitignore`: decisão do owner.
10. A amostra manual da S1 pelo owner, prevista no plano, não foi feita.
