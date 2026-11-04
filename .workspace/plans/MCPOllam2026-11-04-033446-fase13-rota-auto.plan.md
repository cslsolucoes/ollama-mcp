# Plano: Fase 13, rota automática de modelo (velocidade × qualidade × tipo de atividade)

> **Estado:** 📝 PLANO, **não aprovado**. Nada foi implementado.
> **Pedido do owner (2026-11-04 03:34):** acrescentar a funcionalidade `auto`, que escolhe o modelo para cada job enviado pela IA à IA local. A escolha equilibra velocidade, qualidade e tipo de atividade e leva em conta a prioridade (urgência) que a IA declarar.
> **Última atualização:** 2026-11-04 03:35.
> **Relacionados:** Fase 12 (`.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`, em execução, na 12.9); contrato de dados `docs/CONTRATO-DADOS-TREINO.md` (1.1.6).

---

## 0. Para quem lê este documento

Este plano parte do estado de 2026-11-04. A §2 tem os fatos, com fonte. A §5 tem as decisões propostas. A §8 tem as questões que o owner precisa fechar antes da 13.1.

A metodologia é a mesma da Fase 12 (plano §11):
- coordenação em Opus, implementação em Sonnet e revisão adversarial em Opus;
- um teste por regra, provado por mutação;
- gate de build, vet, gofmt, `go test -count=1` e `-race`;
- os testes de ataque das Fases 9, 10.L, 10.5 e 12 são rodados de novo a cada mudança em `src/gateway`;
- commit por etapa, com autorização do owner.

## 1. Objetivo

Cada job enviado pela IA (Claude, Cursor, Codex) à IA local declara **o que precisa** e o sistema escolhe **o modelo**:
- **`prioridade` (velocidade × qualidade):** `rapido`, `preciso` ou `auto` (padrão). Com `auto`, o servidor decide pelo tipo de atividade, pela tabela de rota e pela carga do momento.
- **`urgencia` (quando precisa):** `alta`, `normal` (padrão) ou `baixa`. Ela controla a posição na fila e a tolerância de espera, por exemplo "`preciso`, mas não espero mais que X s".
- **Transparência:** a resposta diz qual modelo atendeu e por quê (`rota=`), para a IA saber o que conferir.
- **Ajuste por dados:** a tabela de rota parte dos números da Fase 8 e passa a ser ajustada pelos erros reais da Fase 12 (banco de provas por categoria e por modelo).

Fora do escopo: treino de modelo (LoRA, Fase 12 §10), modelos fora da lista aprovada pelo owner e paralelismo real com 2 Ollamas no pool (fica como opção, §7).

## 2. Fatos de partida (2026-11-04)

1. **Produção:** gateway `0.12.6-a155513`, mcpollama `0.9.2-f367b35`, Ollama 0.40.13. Servidor com 4 × RTX 3070 (8 GB) a 100 W e clock travado em 210–1400 MHz por causa da fonte de 850 W, que já causou reboots.
2. **Concorrência:** `OLLAMA_NUM_PARALLEL=3` e gateway `--gen-slots 3` (owner, 2026-11-04 02:51). O `qwen3-ia` (`qwen3.5:9b`, arquitetura `qwen35`) **gera 1 por vez**. O log diz "model architecture does not currently support parallel requests" e `n_seq_max = 1`. Os outros pedidos esperam dentro do Ollama.
3. **Modelos aprovados para esta fase (owner, 03:14):** `qwen3.5:9b` (`qwen3-ia`), `nomic-embed-text` e `gemma4:12b`. O `gemma4` foi removido do servidor na Fase 8 e precisa ser baixado de novo.
4. **Números da Fase 8** (`.workspace/reports/MCPOllam2026-10-28-220428-fase8-bakeoff-modelos.report.md` e `...235002-fase8-9-carga-multiagente.report.md`):

| Tipo | `qwen3.5:9b` | `gemma4:12b` |
| --- | --- | --- |
| Numeração de seções (S2) | 77–79% (80–85% sob carga) | **100%** |
| Português / tradução (S4) | 6,0 | **8,0** |
| Resumo com invenção (S1, mesmos 4 documentos) | 45% | 35% |
| Formato (S3) | 100% | 100% (envolve o JSON em cerca ```` ```json ````) |
| Conta direta / passo a passo | 0% / 100% | 40% / 100% |
| Contagem direta / por lista JSON | 50% / 50% | 33% / 67% |
| Prefill / geração (tok/s) | ~3.200 / ~52 | ~1.150 / ~37 |
| Vazão com 10 agentes | ~700–790 tok/s | ~110–130 tok/s (2 timeouts) |
| Paralelo real no Ollama | não (1 slot) | **sim** (N slots), sem ganho de vazão total |
| VRAM | 12,2–14,3 GB a 64k | 18,7 GB a 64k (1 slot) · 16,3 GB a 2×32k |
| Carga a frio | 16–19 s | 30 s |

5. **VRAM com os três residentes:** qwen3.5 a 64k + gemma4 a 64k + nomic ≈ **31,2 GB** de 32 (no limite). Com o **gemma4 a 32k e 2 slots**, ≈ **28,8 GB** (folga de ~3 GB).
6. **Raciocínio:** o `gemma4` vem com thinking ligado por padrão. Com ele ligado, a S2 deu 2 de 2 timeouts de 300 s (300 s só raciocinando, resposta vazia); com `think:false`, 42 s e 100%. O mcpollama manda `think:false` e o gateway aplica a política "raciocínio desligado por padrão". **Falta conferir que isso vale para o gemma4.**
7. **O que já existe:**
   - argumento `model` nas tools;
   - `--allowed-models` no gateway e no mcpollama;
   - `list_models`;
   - prioridade de fila no gateway (`--priority-ips`, ações do admin promover e despromover);
   - pool de backends (Fase 10);
   - cabeçalho `X-Mcpollama-Tool`;
   - melhora sem treino da Fase 12 no gateway (pacote `melhora`, SSOT com o bakeoff);
   - banco de provas `bakeoff -suites erros`.
8. **Escolha manual já possível sem código** (resposta ao owner, 03:18): basta baixar o gemma4, criar o alias, subir o `MAX_LOADED_MODELS` para 3 e incluir o modelo no `--allowed-models` do gateway e do `.mcp.json`. A IA passa `model` explicitamente. O `auto`, a válvula de carga, o `rota=` e a fila por modelo **exigem código**: é esta fase.

## 3. Conceitos

| Termo | Significado |
| --- | --- |
| `prioridade` | eixo velocidade × qualidade declarado pela IA: `rapido` / `preciso` / `auto` |
| `urgencia` | eixo de tempo: `alta` / `normal` / `baixa`; posição na fila e espera máxima aceita |
| classe da tarefa | o que o job é: `numeracao`, `traducao`, `resumo_curto`, `resumo_grande`, `extracao_simples`, `formato`, `rascunho`, `conta` |
| tabela de rota | arquivo versionado (ID e hash) que mapeia (tool, classe, prioridade) → modelo preferido + alternativo |
| válvula de carga | regra que troca o preferido pelo alternativo quando a espera estimada passa do aceitável ou o modelo está fora |
| raia por modelo | slots de geração separados por modelo no gateway, para um modelo não esperar atrás do outro |

## 4. Desenho (proposta)

### 4.1 Onde decide

**No gateway**, que conhece a carga, a fila, os modelos residentes e os erros (Fase 12). O pacote novo `gateway/rota` é puro e determinístico e fica compartilhado com o bakeoff, para medir a rota (SSOT, como o `melhora`).

O **mcpollama** só **classifica** a tarefa, porque é ele que conhece os argumentos da tool: tamanho do documento, instrução do `extract`, `format`, `target_language`. Ele envia três cabeçalhos:
- `X-Mcpollama-Prioridade`;
- `X-Mcpollama-Urgencia`;
- `X-Mcpollama-Classe`.

### 4.2 Fluxo de um job

1. A tool recebe os argumentos opcionais `prioridade` e `urgencia`. Sem eles, vale `auto`/`normal`. `model` explícito continua tendo precedência e a rota não se aplica.
2. O mcpollama calcula a **classe** por regras objetivas:
   - `extract` cuja instrução fala de seção, cláusula, numeração, item ou artigo → `numeracao`;
   - `translate` → `traducao`;
   - `summarize_file` até N caracteres → `resumo_curto`, acima disso → `resumo_grande`;
   - `extract` com `format` JSON ou lista → `formato`;
   - `generate`/`chat` → `rascunho`;
   - pedido de conta/contagem detectado → `conta`.
   
   Os três valores vão nos cabeçalhos.
3. O gateway consulta a **tabela de rota**, que dá o preferido e o alternativo, e aplica a **válvula de carga**:
   - espera estimada do preferido (fila da raia × tempo médio por classe) acima do limite da urgência → alternativo;
   - preferido fora (backend com falha ou não residente) → alternativo.
4. O gateway **reescreve o `model`** do corpo, só com `auto`/`rapido`/`preciso`, só para modelos da `--allowed-models`, e aplica a política de raciocínio do modelo escolhido. Em seguida coloca o pedido na **raia do modelo** com a posição dada pela urgência.
5. A resposta leva `X-Mcpollama-Rota: auto:numeracao→gemma4-ia (fila 2, espera 40 s)` ou `auto:numeracao→qwen3-ia (válvula: espera gemma4 > 120 s)`. O mcpollama mostra `rota=` na linha `[uso]` e no `_meta`.
6. O job registra em `requests` o modelo pedido, o modelo usado, a classe e o motivo, para medir e para a Fase 12 (o retrato e o treino passam a saber o modelo de cada amostra).

### 4.3 Tabela de rota inicial (v1; da Fase 8, a ajustar com dados)

| Classe | `auto` | Alternativo | `rapido` | `preciso` |
| --- | --- | --- | --- | --- |
| `numeracao` | gemma4 | qwen3.5 | qwen3.5 | gemma4 |
| `traducao` | gemma4 | qwen3.5 | qwen3.5 | gemma4 |
| `resumo_curto` | gemma4 | qwen3.5 | qwen3.5 | gemma4 |
| `resumo_grande` | qwen3.5 | gemma4 | qwen3.5 | gemma4 |
| `extracao_simples` / `formato` | qwen3.5 | gemma4 | qwen3.5 | gemma4 |
| `rascunho` | qwen3.5 | gemma4 | qwen3.5 | gemma4 |
| `conta` | qwen3.5 (passo a passo) | gemma4 | qwen3.5 | gemma4 |

### 4.4 Urgência (proposta)

| `urgencia` | Fila | Espera máxima aceita antes da válvula |
| --- | --- | --- |
| `alta` | entra à frente na raia (como o "promover" do admin), limitada por cliente | 30 s |
| `normal` | ordem normal | 120 s |
| `baixa` | atrás das outras; pode esperar o modelo preferido | 600 s (o run-timeout é 10 min) |

Abuso: `alta` fica limitada por cliente e por IP (cota), para uma estação não furar a fila de todas. A prioridade por IP que já existe (`--priority-ips`) continua valendo.

### 4.5 Raia por modelo

Hoje os slots de geração são compartilhados. Com dois modelos residentes:
- raia `qwen3-ia`: 1 slot efetivo (o Ollama gera 1 por vez);
- raia `gemma4-ia`: 2 slots reais (o gemma4 aceita paralelo).

Assim o gemma4 não espera atrás do qwen3.5, e o monitor mostra a fila de cada modelo. O `--gen-slots` passa a ser por modelo, por exemplo `--gen-slots "qwen3-ia=1;gemma4-ia=2"`, e o `NUM_PARALLEL` do Ollama precisa acompanhar a maior raia.

## 5. Decisões propostas (D13-n)

| # | Decisão | Estado |
| --- | --- | --- |
| D13-1 | A rota decide no gateway; o mcpollama só classifica; o pacote `gateway/rota` é SSOT com o bakeoff | proposta |
| D13-2 | Argumentos novos nas tools: `prioridade` (`rapido`/`preciso`/`auto`, padrão `auto`) e `urgencia` (`alta`/`normal`/`baixa`, padrão `normal`); `model` explícito tem precedência | proposta |
| D13-3 | A classe da tarefa é calculada no mcpollama por regras objetivas (§4.2), testadas caso a caso; na dúvida, `rascunho` | proposta |
| D13-4 | Tabela de rota em arquivo versionado (ID e hash), carregada pelo gateway (`--rota-tabela`); v1 = §4.3 | proposta |
| D13-5 | Válvula de carga por espera estimada × urgência (§4.4); modelo fora → alternativo; nunca falha o job por causa da rota | proposta |
| D13-6 | Raia de geração por modelo (§4.5) | proposta |
| D13-7 | Transparência: `X-Mcpollama-Rota` na resposta, `rota=` na `[uso]` e no `_meta`, e colunas novas em `requests` (modelo pedido, usado, classe e motivo) | proposta |
| D13-8 | O gemma4 entra como alias `gemma4-ia` com `num_ctx 32768` no Modelfile; os três ficam residentes (`OLLAMA_MAX_LOADED_MODELS=3`) | proposta (questão 1) |
| D13-9 | Política de raciocínio por modelo no gateway: desligado por padrão também no gemma4, conferido ao vivo | proposta |
| D13-10 | Integração com a Fase 12: amostra, retrato e banco de provas guardam o modelo; o `bakeoff -suites erros` compara por modelo e por categoria para ajustar a tabela (§4.3) com dados reais | proposta |
| D13-11 | Mudanças operacionais no servidor seguem a preferência do owner: aplicar direto e subir, sem teste de carga (memória `feedback-acao-simples-direta`) | **decidida** |

## 6. Etapas

| # | Etapa | Modelo | Aceite | Estado |
| --- | --- | --- | --- | --- |
| 13.0 | Aprovação do plano e das questões da §8 | owner | owner aprova | pendente |
| 13.1 | **Infra:** baixar o `gemma4:12b`; alias `gemma4-ia` (`num_ctx 32768`); `OLLAMA_MAX_LOADED_MODELS=3`; incluir o modelo no `--allowed-models` do gateway (o `.mcp.json` das estações fica para a 13.7) | coordenador | os três residentes (`ollama ps`), VRAM dentro de 32 GB, `think:false` respeitado pelo gemma4 numa chamada | pendente |
| 13.2 | **Pacote `gateway/rota`:** tabela versionada, classes, decisão `prioridade` × `urgencia` × carga, válvula | sonnet | um teste por regra, provado por mutação; determinístico | pendente |
| 13.3 | **Gateway:** cabeçalhos de entrada, reescrita do `model` (só a allowlist), raia por modelo, urgência na fila (com cota para `alta`), `X-Mcpollama-Rota`, colunas em `requests`, monitor com a fila por modelo e a rota de cada job | sonnet | testes com Ollama falso de dois modelos; ataques 9/10.L/10.5/12 OK | pendente |
| 13.4 | **mcpollama:** argumentos `prioridade` e `urgencia` nas tools, classificação da tarefa, cabeçalhos, `rota=` na `[uso]` e no `_meta`; `TOOLS.md` (medir o custo em tokens do `tools/list`) | sonnet | testes de classificação caso a caso; o `model` explícito vence | pendente |
| 13.5 | **Bakeoff e Fase 12:** medir a rota com o banco de provas (por modelo e por categoria); o modelo guardado na amostra e no retrato | sonnet | relatório por modelo; o gate da 12.7 vale por modelo | pendente |
| 13.6 | **Revisão adversarial:** fura-fila com `alta`; troca de modelo fora da allowlist; cabeçalho forjado; impasse entre raias; vazamento de rota; VRAM e energia | **opus** | achados corrigidos; testes guardados em `.workspace/reports/...fase13.testes/` | pendente |
| 13.7 | **Implantação:** gateway e mcpollama novos (junto ou depois da 12.10), `.mcp.json` das estações com a allowlist, `CLAUDE.md` global com a orientação de `prioridade`/`urgencia` | coordenador + owner | rota real no monitor; `rota=` visível na `[uso]` | pendente |

## 7. Riscos

1. **Energia:** com os dois modelos gerando juntos, mais placas ficam no pico ao mesmo tempo, com a fonte de 850 W que já causou reboots. Mitigação: segunda fonte (já comprada); até lá, limitar a raia do gemma4 a 1 ou 2 slots.
2. **VRAM no limite:** o gemma4 a 64k deixa a soma em ~31 GB. O alias a 32k dá ~3 GB de folga. Se o `num_ctx` do Modelfile não prevalecer sobre o `OLLAMA_CONTEXT_LENGTH` global, reavaliar (questão 1).
3. **Troca de modelo (swap):** se um dos três sair da VRAM, a recarga custa 16–30 s. A válvula trata "não residente" como "fora".
4. **Classificação errada:** uma tarefa de numeração classificada como `extracao_simples` cai no modelo mais fraco. Mitigação: regras testadas, `preciso` sempre disponível, e o erro passa a ser medido pela Fase 12.
5. **Custo em tokens:** os argumentos novos aumentam o `tools/list`. Medir na 13.4 e enxugar, como na 12.5.
6. **Abuso da `urgencia=alta`:** cota por cliente e por IP; teste adversarial na 13.6.
7. **Paralelo real continua limitado** ao gemma4 (o qwen3.5 gera 1 por vez). Paralelo de verdade para o qwen3.5 só com 2 Ollamas no pool (opção fora desta fase).

## 8. Questões para o owner (fechar antes da 13.1)

1. **Contexto do gemma4:** 32k, com folga de ~3 GB de VRAM (recomendado), ou 64k, no limite?
2. **Padrão quando a IA não declarar nada:** `auto` (recomendado) ou `rapido`, que mantém o comportamento de hoje?
3. **Urgência:**
   - os limites da §4.4 (30 s / 120 s / 600 s) estão bons?
   - a cota para `alta` vale por cliente, por IP ou pelos dois?
4. **Ordem com a Fase 12:**
   - implantar a Fase 13 junto com a 12.10;
   - ou depois dela;
   - ou a 13.1 (infra, só configuração) já agora, deixando a escolha manual por `model` disponível antes do código.
5. **Energia:** instalar o gemma4 antes da segunda fonte, limitando a raia dele a 1 slot, ou esperar a fonte?

## 9. Changelog

- 2026-11-04 03:35: plano criado a partir do pedido do owner (03:34) e da conversa das 02:55–03:20 (modelos, VRAM, acerto por tipo, rota proposta).
