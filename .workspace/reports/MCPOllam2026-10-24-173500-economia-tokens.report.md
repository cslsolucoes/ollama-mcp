---
title: "Fase 6 — medição da economia de tokens do mcpollama no Claude Code"
created: 2026-10-24
status: concluído (amostra pequena; ver limitações)
locale: pt-BR
---

# Medição da economia de tokens — mcpollama no Claude Code

## Resumo

O mesmo pedido real foi feito ao Claude Code de dois jeitos: o Claude lendo o arquivo (`Read`) e o Claude chamando a tool **`summarize_file`** do mcpollama. Pedido: "resuma o `60-Requisitos_mcpollama.md` em até 150 palavras".

- **O custo Anthropic caiu de US$ 0,0370 para US$ 0,0124: −66,4%** (comparação com o cache quente).
- **~22 mil tokens do arquivo deixaram de entrar no contexto do Claude:** a criação de cache no turno final caiu de 23.244 para 1.069 tokens.
- O trabalho pesado foi para o servidor local: **27.474 tokens de prompt** processados pelo `qwen3-ia` em 18,8 s, com custo zero de cota Anthropic.
- **Custo em tempo:** a rodada com o mcpollama levou +15,6 s de parede (39,3 s × 23,7 s).
- **Qualidade:** os dois resumos estão corretos nos números (171 requisitos; 105/52/14). Diferenças no §4.

## 1. Método

| Item | Valor |
| --- | --- |
| Cliente | Claude Code CLI, headless: `claude -p ... --model haiku --output-format json` |
| Arquivo | `referencias/Brief_Referencias/60-Requisitos_mcpollama.md` (73.277 bytes, 72.114 caracteres, 356 linhas) |
| Pedido (igual nas duas variantes) | "Resuma em português, em no máximo 150 palavras, o arquivo ... (quais áreas, quantos requisitos, prioridades e os pontos mais críticos)" |
| Variante A (sem mcpollama) | `--strict-mcp-config --allowedTools Read` e "Use a ferramenta Read para ler o arquivo" |
| Variante B (com mcpollama) | `--mcp-config .mcp.json --strict-mcp-config --allowedTools mcp__mcpOllama__summarize_file` e "Use a ferramenta summarize_file ... NÃO leia o arquivo você mesmo" |
| Ordem | A (cache frio) → B → A2 (repetição de A com o cache quente, para tirar o efeito da ordem) |
| Servidor Ollama | `qwen3-ia` (MoE `qwen3-coder:30b-a3b`), 3 GPUs, `num_ctx` 65536 |
| Data | 2026-10-24, 17:28–17:31 |

Os números vêm do JSON do `claude -p` (`total_cost_usd`, `usage`, `num_turns`) e da linha `[uso]` do mcpollama. Saídas brutas em [`...economia-tokens.evidencias/`](MCPOllam2026-10-24-173500-economia-tokens.evidencias/) (`A.json`, `A2.json`, `B.json`; o token do Ollama foi trocado por `[omitido]`; varredura `known_hits=0`).

## 2. Resultados

| Rodada | Custo (US$) | Turnos | Criação de cache | Leitura de cache | Saída (raciocínio) | Turno final: criação de cache | Parede |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A — `Read`, cache frio | 0,03944 | 3 | 87.666 | 203.354 | 1.770 (1.080) | 16.060 | 31,2 s |
| **A2 — `Read`, cache quente** | **0,03702** | 2 | 53.584 | 123.220 | 1.492 (934) | **23.244** | 23,7 s |
| **B — `summarize_file`** | **0,01243** | 3 | 48.110 | 184.523 | 1.630 (837) | **1.069** | 39,3 s |

**Lado do servidor (B):** `[uso] modelo=qwen3-ia prompt_eval_count=27474 eval_count=443 total_ms=18776 load_ms=1 queue_ms=0 partes=1 fim=stop`.

**Economia:**
- custo: B / A2 = 0,336, ou seja **−66,4%** (−68,5% contra A, com o cache frio);
- tokens do arquivo fora do contexto do Claude: turno final A2 − B = 23.244 − 1.069 ≈ **22,2 mil tokens**. No Ollama, o mesmo conteúdo, mais as instruções, contou 27.474 tokens, com outro tokenizador.

**Efeito composto (inferência, não medido):** numa conversa longa, o arquivo lido com `Read` volta a ser processado (como leitura de cache) em **cada turno seguinte**. Com `summarize_file`, só o resumo de ~1 mil tokens permanece. A economia real por conversa tende a ser maior que a de uma chamada isolada.

## 3. Tempo de tool no Claude Code (PB5-3 / P-7)

- Uma chamada ao `generate` com `num_predict` 7000 durou **86.668 ms no servidor** (7.000 tokens, `fim=length`) e **completou sem corte** no Claude Code (parede total 107,3 s). Evidência: `T3.json`.
- Conclusão: o Claude Code aceita pelo menos ~87 s por tool. **Hoje quem limita é o `--timeout` de 120 s do mcpollama.** Uma geração no teto de `num_predict` (32.768 tokens, ~90 tok/s ≈ 6 min) passaria dele e teria o erro claro de timeout da Fase 2. O limite máximo do Claude Code **não foi medido**.

## 4. Qualidade (comparação dos dois resumos)

- **Iguais e corretos:** 171 requisitos em 12 áreas; 105 MUST, 52 SHOULD e 14 COULD.
- **B (`qwen3-ia`):**
  - listou **as 12 áreas com contagem**;
  - pontos críticos corretos (token só por env, TLS com `SystemCertPool`, contadores, `httptest`, `TOOLS.md`);
  - **uma imprecisão:** "prompt acima de ~128.000 caracteres falha". Na verdade é um teto configurável, e acima dele há chunking ou recusa.
- **A (Haiku lendo o arquivo):**
  - listou só as maiores áreas;
  - pontos críticos mais "arquiteturais" (stdout só JSON-RPC, boot sem rede, `--allow-root`, fila de GPU);
  - repetiu uma frase já superada do próprio documento ("prompt grande derruba o `qwen3-ia`").
- **Avaliação:** a qualidade é equivalente para o propósito (resumo de orientação). O resumo local é um pouco mais "listado" e o do Claude um pouco mais "interpretado". Nenhum dos dois inventou número.

## 5. Limitações (honestas)

1. **Amostra pequena:** 1 rodada por variante (+1 repetição de A). Os números mostram a ordem de grandeza, não um intervalo de confiança.
2. **O contexto base domina os totais:** a sessão headless carrega system prompt, `CLAUDE.md` e a definição das tools, dezenas de milhares de tokens iguais nas duas variantes. Por isso a comparação mais limpa é o **custo** e o **turno final**, e não a soma de tokens.
3. **Cache:** A rodou com cache frio. A2 corrige isso, e ainda assim a economia ficou em −66%.
4. **Modelo do cliente:** Haiku. Com Sonnet ou Opus o custo por token é maior, então a economia **absoluta** por arquivo cresce na mesma proporção (inferência).
5. **Um tipo de tarefa só (resumo)**; `extract` e `translate` não foram medidos.
6. **Servidor quente em B** (`load_ms=1`). Com o modelo frio, B ganharia ~22–36 s de parede, sem mudar o custo Anthropic. O nível `local` (`qwen3.8:27b-64k`) e o mcpollama (`qwen3-ia`) disputam a VRAM, e isso foi visto várias vezes.

## 6. Conclusão em uma linha

Para ler e resumir um arquivo de ~72 mil caracteres, o `summarize_file` cortou **~2/3 do custo Anthropic** e manteve **~22 mil tokens fora do contexto**, ao preço de ~15 s a mais de espera e de um resumo de qualidade equivalente.
