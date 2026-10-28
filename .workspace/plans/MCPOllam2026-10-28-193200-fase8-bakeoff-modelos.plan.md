# Plano — Fase 8: comparativo (bake-off) de modelos locais para a finalidade do mcpollama

> **Estado:** EM EXECUÇÃO desde 2026-10-28 19:34 (owner: "faça o teste com todos para o objeto do MCP"; pull dos 4 candidatos AUTORIZADO; troca de modelo, apagar modelos e mudar clientes NÃO autorizados).
> **Origem:** conversa com o owner em 2026-10-28 (19:10–19:32), depois da Fase 7.
> **Decisões do owner (AskUserQuestion, 19:32):**
> - candidatos: **`gemma4:12b`, `qwen3.5:9b`, `granite4.2:8b`, `qwen3:14b`**, contra o atual `qwen3-ia`;
> - teto de tamanho: **12B + 20% (até ~14,4B)**;
> - **somente modelos locais** (sem nuvem);
> - foco na **finalidade**, não em paralelismo;
> - execução: **"só preparar o plano"**.

---

## 1. Finalidade (o que o modelo precisa fazer bem)

O mcpollama existe para que um **modelo local** faça o trabalho mecânico sobre documentos: resumir, extrair e
traduzir, principalmente **contratos em pt-BR**. Isso mantém o conteúdo na rede local e economiza tokens
Anthropic. O consumidor (Claude) **cita cláusulas** e precisa confiar que nada foi inventado nem omitido.

| Requisito | Peso | Como se mede (§4) |
| --- | --- | --- |
| Fidelidade: não inventar, não omitir | **crítico** | S1, S2 |
| Português jurídico (leitura e escrita) | **crítico** | S1, S4 |
| Seguir o formato pedido (JSON, lista, tabela) | alto | S2, S3 |
| Valores literais certos (nº de cláusula, datas, nomes) | alto | S2 |
| Contexto suficiente para um contrato médio (~10–20 mil tokens) | médio | S5 |
| Velocidade (prefill e geração) | médio (desempate) | S5 |
| Código, visão, raciocínio longo | **não precisa** | — |

Limites conhecidos de **todos** os modelos, já medidos com o `qwen3-ia`: contas pedidas direto (20%), e
contagens do tipo "quantos são" (50%). A mitigação documentada continua valendo: passo a passo com `RESULTADO:`,
ou lista em JSON contada por fora. O bake-off mede se algum candidato muda isso, mas isso **não** é critério
de troca.

## 2. Candidatos

| Modelo | Tamanho | Atualizado (Ollama, 2026-10-28) | Por que entra | VRAM estimada (Q4) | Topologia |
| --- | --- | --- | --- | --- | --- |
| `qwen3-ia` (atual) | MoE 30B-A3B | — | base (`qwen3-coder`, **ajustado para código**) | ~21–23 GB | 4 GPUs |
| `gemma4:12b` | 12B | 2 semanas | maior e mais recente da faixa; família forte em multilíngue | ~7,5 GB + KV | 2 GPUs por instância |
| `qwen3.5:9b` | 9B | 2 semanas | geral, recente, segue instrução | ~6 GB + KV | 1 GPU por instância |
| `granite4.2:8b` | 8B | 2 meses | empresarial: RAG, **JSON estruturado**, multilíngue (Apache-2.0) | ~5 GB + KV | 1 GPU por instância |
| `qwen3:14b` | 14B | 1 ano | referência de 14B denso | ~9 GB + KV | 2 GPUs por instância |

- O suporte a português de cada família vem de conhecimento prévio e **será confirmado no teste** (S4). A
  página do Ollama quase não informa idiomas.
- Usar a quantização padrão da tag (Q4_K_M) para todos. Se um candidato vencer por pouco, testar `q8_0` à parte.
- **Fora:** modelos de nuvem; visão, áudio ou "decisão"; variantes da comunidade (uncensored, abliterated,
  fusões); abaixo de ~3B.

## 3. Pré-requisitos e riscos (servidor)

1. **Autorização do owner** para cada mudança no servidor (pull ≈ 30 GB; units ou portas extras na topologia
   de 1 GPU por instância). Hoje há ~40 GB livres em disco no NVMe (conferir antes).
2. **Allowlist:** o mcpollama só aceita `qwen3-ia;nomic-embed-text` (`--allowed-models`, decisão de 26/10). O
   bake-off roda com um **binário de teste** que passa `--allowed-models` explícito, sem mudar os clientes.
3. **`OLLAMA_MAX_LOADED_MODELS=2`:** trocar de modelo descarrega o anterior (~20–35 s de carga a frio por
   troca). Rodar **um candidato por vez**, todas as suítes seguidas, e medir a carga a frio à parte.
4. **Hardware:**
   - 150 W por GPU;
   - link PCIe `00:1b.4` (GPU `03:00.0`) degradado;
   - fonte no limite.

   Uma chamada por vez; monitorar potência e AER durante as suítes. Parar se aparecer erro não-corrigível ou
   reboot.
5. **Sigilo:**
   - **só documentos do owner**, sem contratos de clientes; o corpus da Fase 7 foi apagado. Se o owner
     autorizar contratos de clientes, valem as regras da Fase 7: pasta ignorada pelo git, ids neutros, report
     só com métricas;
   - o texto gerado é avaliado **localmente**, por script, ou por um avaliador que não envie conteúdo de
     cliente a serviço externo.

## 4. Suítes (reaproveitar o que existe; SSOT)

Base: o driver `stressdrv` (modos `quality`, `count`, `limit`) e o `acceptdrv` (modo documento com saída
filtrada), ambos no scratchpad da sessão de 2026-10-26/28. **Promover para `src/cmd/bakeoff/`** (versionado), com
um modo por suíte, `--model` e `--allowed-models` por execução.

| Suíte | O que mede | Como | Métrica |
| --- | --- | --- | --- |
| **S1 Fidelidade de resumo** | frases inventadas ou omissões | `summarize_file` em 5 documentos do owner (DOCX, PDF, PPTX), N=5 cada | nº de afirmações sem base no texto (checagem por script de entidades e números + amostra manual pelo owner); trechos-chave presentes |
| **S2 Extração com valores** | cláusulas, números, datas | `extract` "liste as cláusulas com número e título" em DOCX com numeração | **números de cláusula conferidos contra o Word (COM)**, X de Y; JSON válido (%) |
| **S3 Formato** | segue o formato pedido | `extract` com JSON, lista e tabela | % de saídas no formato exato (parse automático) |
| **S4 Português e tradução** | qualidade do texto | `translate` pt→en e en→pt de 3 documentos + resumo em pt | preservação da estrutura (marcadores, títulos, identificadores); revisão humana curta do owner em 3 amostras |
| **S5 Desempenho** | velocidade e contexto | o modo `limit` adaptado: prompt de 8k, 16k e 32k tokens; geração de 1.000 tokens | prefill (tok/s), geração (tok/s), carga a frio, VRAM, contexto máximo 100% na GPU |
| **S6 Limites conhecidos** (informativo) | contas e contagem | modos `quality` (P3) e `count` (R1) | taxa de acerto (não decide a troca) |

**Gate de cada execução:** 0 HTTP 429 (1 s entre chamadas), 0 erros de protocolo, mesma versão do binário.

## 5. Critério de decisão

1. **Elimina:** S1 com taxa de invenção > 0 em mais de 1 de 25 resumos; ou S2 abaixo de 95% de números de
   cláusula corretos; ou S3 abaixo de 95%.
2. **Entre os que passam, ordem:** S1 → S2 → S4 → S5.
3. **Topologia:** se o vencedor couber em 1 GPU (≤ 9B), avaliar 3–4 instâncias, uma por GPU, sem a GPU do link
   degradado até o reparo. Isso fica para a Fase 8a (paralelismo), com stress de 10 min a 150 W antes de ir
   para os clientes.
4. **Troca só com decisão do owner,** depois do report.

## 6. Etapas

| Etapa | Conteúdo | Muda o servidor? |
| --- | --- | --- |
| 8.0 | Promover os drivers para `src/cmd/bakeoff/` (suítes S1–S6, `--model`, `--allowed-models`), testes, commit | não |
| 8.1 | Montar o corpus do owner (documentos dele, cópia em pasta ignorada, ids neutros) + gabarito da S2 via Word COM | não |
| 8.2 | Linha de base `qwen3-ia` em S1–S6 | não (modelo atual) |
| 8.3 | **Pull** dos 4 candidatos (~30 GB) + `ollama show` (contexto, licença, tamanho) | **sim — autorização** |
| 8.4 | S1–S6 por candidato, um de cada vez, com monitor de potência e AER | carga normal |
| 8.5 | Report (só métricas) + recomendação | não |
| 8.6 | Se houver troca: Modelfile e alias (`qwen3-ia` → vencedor?), `ollama-warm`, allowlist, docs, validação nos clientes | **sim — autorização** |
| 8.7 | Limpeza: `ollama rm` dos perdedores (regra "só modelos otimizados para o hardware") | **sim — autorização** |

## 7. Decisões que ficarão para o owner

- **D8-1:** autorizar o pull (8.3) e quando.
- **D8-2:** usar ou não contratos de clientes na S1/S2 (padrão: só documentos do owner).
- **D8-3:** troca do modelo padrão e o nome (manter o alias `qwen3-ia` apontando para o vencedor evita mudar os clientes).
- **D8-4:** embeddings: avaliar `qwen3-embedding:0.6b`, `embeddinggemma-2` e `granite-embedding:278m` contra o
  `nomic-embed-text` (S7 opcional: busca semântica em pt-BR).
- **D8-5:** `translategemma` como modelo só da tool `translate` (opcional).

## 8. Changelog

- 2026-10-28 19:33 — plano criado (estado: planejado). Nenhuma execução.
- 2026-10-28 19:34 — **owner autorizou a execução** (aça o teste com todos para o objeto do MCP): D8-1 = pull autorizado agora; D8-2 = só documentos do owner (padrão). Execução delegada a um agente Opus (8.0–8.5). Escopo vedado: troca de modelo padrão, Modelfiles/warm/drop-in/power cap, .mcp.json dos clientes, ollama rm.
- 2026-10-28 19:42 — **owner fixou os alvos de capacidade** (otimizar o máximo Simultâneos x Contexto): **10 × 32k**, **5 × 32k** ou **5 × 64k**. Nova suíte **S7 — capacidade simultânea × contexto**, por modelo, com a escada 5×32k → 5×64k → 10×32k. Drop-in **temporário** 99-bakeoff.conf (NUM_PARALLEL/CONTEXT_LENGTH), removido e provado ao final. Carga via loopback no servidor (sem nginx), 150 W mantidos, parada imediata em Xid, AER não-corrigível ou reboot. Critério **secundário** depois da fidelidade (S1). Hipótese a medir: as arquiteturas híbridas/SWA (qwen3.5, gemma4, granite4.2) têm KV bem menor que o qwen3-ia (~27 KB/token em q4_0). Owner liberou o servidor (já parei o uso nas outras IA).
- 2026-10-28 19:43 — owner: **	estar no servidor** (confirma a S7 via loopback no servidor) e **deixar somente os modelos que serão utilizados** → a etapa **8.7 (`ollama rm` dos não usados) está AUTORIZADA**, para ser executada pelo loop principal **logo depois que o owner confirmar o(s) modelo(s) escolhido(s)** com base no report (D8-3). O agente do bake-off não apaga modelos.
