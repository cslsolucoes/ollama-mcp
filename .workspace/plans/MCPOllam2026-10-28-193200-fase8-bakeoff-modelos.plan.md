# Plano — Fase 8: comparativo (bake-off) de modelos locais para a finalidade do mcpollama

> **Estado:** **EXECUTADO (8.0–8.5); aguardando decisão do owner (D8-3).** Executado em modo não extremo depois de 6 reboots (S7 por cálculo + validação 2×8k). Report: `.workspace/reports/MCPOllam2026-10-28-220428-fase8-bakeoff-modelos.report.md`. Histórico: EM EXECUÇÃO desde 2026-10-28 19:34 (owner: "faça o teste com todos para o objeto do MCP").
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
- 2026-10-28 20:53 — **INCIDENTE: 2 reboots seguidos.** O 1º (~20:50:00) aconteceu durante as suítes do `qwen3.5:9b`, com ~280 W somados nas GPUs (carga baixa); o 2º (~20:51) aconteceu durante o `load_tensors` de uma chamada em curso. Journal com `SyncIntervalSec=10s`: 0 Xid, 0 AER não-corrigível (corte seco); AER corrigível de `00:1b.4` em 220 → 223 no minuto anterior. Hipótese: link PCIe da GPU `03:00.0`, não a fonte. O servidor voltou às 20:51:35, sem `99-bakeoff.conf` (a S7 não tinha começado). O loop principal mandou o agente parar. **Decisão do owner (AskUserQuestion):** `Continuar com as 4 GPUs` (opções recusadas: excluir a GPU 03:00.0, recomendada; parar o bake-off). Retomado com uma **trava**: no próximo reboot o agente para de vez, sem retry automático, e o owner decide. Até 20:50 estavam concluídos: linha de base `qwen3-ia` (83 saídas) e `gemma4:12b` (83 saídas); `qwen3.5:9b` estava incompleto.
- 2026-10-28 20:58 — **Mais 2 reboots (20:52 e 20:54), 4 no total.** **Causa da repetição: falha de supervisão.** O driver do agente não abortava em erro de conexão e o `run-candidatos.sh` (laço em background) relançava o próximo teste depois de cada queda; cada relançamento recarregava o modelo. O loop principal mandou parar por mensagem, mas o laço só parou quando o loop principal encerrou a árvore de processos (`taskkill /T` no pid 16168, 20:54:48). 1ª queda: prefill de ~16 mil tokens no `qwen3.5:9b`. **Owner:** a fonte é **Cougar 850 W** (no limite para 4 RTX 3070: ~600 W de GPU a 150 W + ~150–200 W do resto; picos de ms da Ampere desarmam a proteção); `no momento é o hardware que temos`. **Decisões:** produção a **130 W** (aplicado e persistente às 20:57; backup `.150w`; unit `loaded`/`enabled`); bake-off `continuar mas sem levar ao extremo`: driver aborta no 1º erro de conexão + checagem de uptime antes de cada carga e bloco; sem laço; S5 só 8k/12k; **S7 por cálculo** (KV por token do log → projeção de 5×32k, 5×64k e 10×32k) + no máximo 2 simultâneas de 8k; `think:false` aprovado para o teste (achado: `gemma4`/`qwen3.5` entram em thinking de 300 s sem ele; adotar em produção exige mudança no mcpollama). Servidor restaurado: `qwen3-ia` e `nomic` residentes, 100% GPU, 0 Xid.
- 2026-10-28 22:05 — **EXECUTADO (8.0–8.5); aguardando a D8-3.** Registro do agente executor (Opus 5.5), por etapa:
  - **8.0 Ferramenta (concluído).**
    - **Deu certo:** `src/cmd/bakeoff/` (`main.go`, `rpc.go`, `suites.go`, `score.go`, `offline.go`, `main_test.go`), reaproveitando o `stressdrv` e o `acceptdrv`. A ferramenta:
      - sobe o mcpollama de teste com `--model` e `--allowed-models "<m>;nomic-embed-text"` e `--timeout 5m`;
      - roda as suítes `load`, `ps` e `s1`–`s6`, mais `ref`, `score` e `blind` offline;
      - grava as saídas em `.workspace/bakeoff/out/` (ignorado; `.gitignore` + `/.workspace/bakeoff/`, conferido com `git check-ignore`);
      - lê o token por arquivo e o apaga de toda saída.
      `go vet`, `gofmt` e `go test -count=1 ./...` passam (10 pacotes, às 22:04). Sem commit.
    - **Não deu certo:**
      - (a) raízes aninhadas no `--allow-root` são recusadas pelo servidor; solução: a raiz padrão é o repositório (o corpus está dentro dele);
      - (b) o gabarito contava os 3 marcadores do Word (glifo de uso privado como `ListString`); solução: exigir um dígito no número;
      - (c) **a S5 não parava em erro e o laço `run-candidatos.sh` seguia para o próximo modelo**, o que contribuiu para os reboots 2 a 4 (ver o incidente). Solução, por ordem do owner:
        - abortar o processo inteiro na 1ª falha (código 3);
        - `uptime -s` + AER de `00:1b.4` por SSH antes de cada bloco e no fim (código 4 se o boot mudar);
        - teto de prefill estimado (`-max-prefill-tokens`, `pulado-risco`);
        - `-s5-sizes`/`-s5-gen`;
        - um modelo por vez, sem laço.
    - **Variante de teste** `think:false`, compilada de uma cópia de `src/` (patch em `.workspace/bakeoff/variante-nothink.patch`; o `src/` do repo não foi tocado), **aprovada pelo owner** e usada nos 4 candidatos.
  - **8.1 Corpus e gabaritos (concluído).**
    - Ids `b01`… (mapa só em `.workspace/bakeoff/mapa.txt`). A S1 usou `b01` (docx), `b02` (pptx), `b03`, `b08` (em inglês) e `b09` (pdf); a S4 usou trechos `.txt` de `b01`, `b08` e `b09`.
    - **Removidos (aprovado):** `b05` (nomes de terceiros), `b06` (texto ilegível) e `b10` (docx vazio).
    - Gabarito da S2 pelo Word COM (somente leitura, invisível): **22 seções numeradas**. O `Quit` falhou e foi encerrado só o PID aberto (40652).
    - Referência da S1 = texto do `doctext`. Rubrica da S1 em `.workspace/bakeoff/rubrica-s1.md`.
  - **8.2 Linha de base `qwen3-ia` (concluído, config 4 GPUs a 150 W).** 99 chamadas, 0 erro, 0 HTTP 429, `queue_ms` 0. Carga a frio não medida (o modelo já estava residente).
  - **8.3 Servidor (concluído).**
    - `df`: 168 GB livres, 141 GB depois do pull. Pull dos 4 em 6 min 39 s (27 GB).
    - `ollama show`:
      - os 4 são Apache-2.0 e Q4_K_M;
      - `gemma4`, `qwen3.5` e `qwen3:14b` têm thinking ligado por padrão; o `granite4.2` tem a capacidade;
      - o `qwen3:14b` tem contexto nativo de **40.960**.
    - Antes: 150 W e AER de `00:1b.4` em 110.
  - **8.4 Suítes por candidato (concluído, em modo não extremo depois dos reboots).** Configs de GPU por medição no report §2.4: 4G-150, 4G-130, 3G-s03 e 3G-s06.
    - **Deu certo:**
      - `gemma4:12b` (100 chamadas, 4G-150), `granite4.2:8b` (refeito na 3G-s06 a partir da S3 b02, 83 + 2 chamadas), `qwen3:14b` (96 + 2) e `qwen3.5:9b` (83 chamadas, 11 `pulado-risco` por prefill > 8k, 3G-s06): todos com 0 erro;
      - S5 com prefill de 8k e 12k (sem 16k/32k, exceto no `qwen3-ia` e no `gemma4`, medidos antes);
      - **S7 por cálculo** (KV por token lido do journal) e validação 2×8k com `99-bakeoff.conf` `NUM_PARALLEL=2` temporário no `granite4.2:8b` e no `qwen3:14b`: 2/2 ok nos dois, drop-in removido e provado (`NUM_PARALLEL=1`).
    - **Não deu certo:**
      - (a) com o mcpollama HEAD, o `gemma4` estoura o timeout de 300 s na extração (thinking sem fim). Solução no bake-off: variante `think:false`; em produção, exige mudança de código;
      - (b) **6 reboots** (report §7.1). O diagnóstico do coordenador é o link PCIe da GPU `03:00.0` (porta `00:1b.4`), com a fonte de 850 W no limite como agravante. Os erros AER acompanharam o uso da `03:00.0`: +102 com o `gemma4`; +254 até o 6º reboot com o `granite` em 4 GPUs; 11 → 96 com o `qwen3.5` na 3G-s06; **0 na config sem a `03:00.0`**. Soluções do owner: 130 W, 3 GPUs (primeiro sem a 03, depois sem a 06, para isolar a placa) e modo não extremo;
      - (c) o `qwen3.5:9b` ficou sem o `b03` (S1/S3) e sem a S5 acima de 8k, por ordem do owner.
  - **8.5 Report (concluído):** `.workspace/reports/MCPOllam2026-10-28-220428-fase8-bakeoff-modelos.report.md`.
    - **S1 (cega, Opus por documento; resumos com invenção / graves / omissões):**

      | Modelo | Com invenção | Graves | Omissões |
      | --- | --- | --- | --- |
      | `gemma4` | 9/25 | 0 | 3 |
      | `qwen3-ia` | 11/25 | 6 | 34 |
      | `qwen3.5` | 9/20 | 0 | 4 |
      | `granite` | 14/25 | 1 | 11 |
      | `qwen3:14b` | 16/25 | 9 | 22 |

    - **S2 (número + título certos):** `gemma4` 100%; os outros entre 77% e 84%. Todos caíram na armadilha do sumário desatualizado.
    - **S3:** 100% em todos, exceto o `granite` (88,9%).
    - **S4 (nota de pt-BR):** `gemma4` 8,0 · `qwen3-ia` 6,5 · `qwen3.5` 6,0 · `qwen3:14b` 4,5 · `granite` 4,0.
    - **S5:** o `qwen3-ia` é de longe o mais rápido (cerca de 4.700 tok/s de prefill e 89 de geração) e o `gemma4` o mais lento (cerca de 1.140 e 37).
    - **S7:** só o `gemma4` e o `qwen3.5` atingem os 3 alvos por cálculo (KV de 4,5 e 9 KiB/token); o `qwen3-ia` só faz 5×32k, e só com 4 GPUs.
    - **Critério §5:** todos são eliminados pela regra estrita da S1 (inclusive o atual). O ranking é feito por S1 → S2 → S4 → S5: 1º `gemma4:12b`, 2º `qwen3-ia`, 3º `qwen3.5:9b` (fora por estabilidade), 4º `granite4.2:8b`, 5º `qwen3:14b`.
    - **Recomendação:** manter o `qwen3-ia` agora. O `gemma4:12b` é o candidato, condicionado a:
      - (1) o mcpollama enviar `think:false`;
      - (2) revalidação na config final de GPU, com S1 ampliada e stress de 10 min;
      - (3) aceitar latência 3 a 5 vezes maior.
  - **Estado final do servidor (21:58):** `qwen3-ia` 100% GPU com 64k; `nomic` em 74%/26% CPU/GPU (não cabe inteiro em 3 GPUs); candidatos descarregados; `NUM_PARALLEL=1`; sem `99-bakeoff.conf`; 130 W; 3 GPUs (01, 03 e 05); AER em 124 corrigíveis; Xid 0.
  - **Pendências (numeradas):**
    1. D8-3: troca ou não (condições acima).
    2. **Reparo físico do link `00:1b.4` (GPU `03:00.0`) para voltar a 4 GPUs.**
    3. Mudança de código: o mcpollama enviar `think:false` (pré-requisito para modelos com thinking).
    4. Revalidar o escolhido na config final, com S1 ampliada (D8-2: contratos?) e stress de 10 min.
    5. Confirmar os horários dos 6 boots com `journalctl --list-boots` (o 3º é inferido).
    6. Opcional: medir o `qwen3.5:9b` sem a `03:00.0`.
    7. D8-4 e D8-5 não executados.
    8. **8.7 (limpeza dos perdedores, ~27 GB):** autorizada para o loop principal depois da D8-3; não executada.
    9. Commit de `src/cmd/bakeoff/` e do `.gitignore`: decisão do owner.
    10. A amostra manual da S1 pelo owner não foi feita.
- 2026-10-28 22:16 — **Causa redefinida pelo owner e produção a 4 GPUs / 100 W** (loop principal).
  - **Causa (owner, 21:46):** "o defeito não são as risers e sim a incapacidade da fonte para as quatro placas". Os
    risers são alimentados por **PCIe 6 pinos**.
    - Isto **substitui** o diagnóstico de "link PCIe da `03:00.0`" do registro de 22:05 (itens 8.4 (b) e a pendência 2).
    - Os erros AER corrigíveis de `00:1b.4` passam a ser tratados como sintoma.
    - Evidência a favor: os 6 reboots foram desligamentos secos (Xid 0, AER não corrigível/fatal 0, sem log), na subida de ocioso para carga. O power cap não corta os picos de milissegundos somados.
    - Registrado na memória do projeto (`servidor-ia-reboots-causa-fonte`).
  - **Decisão do owner (21:48):** "finalizando os testes, ativamos as 4 GPUs e fixamos a potência em 100 W", porque **a VRAM de 32 GB é a prioridade maior**.
  - **Executado às 22:11** pelo loop principal: 1 SSH, `set -e`, `test -s` nos backups e no `.new`, `mv` atômico, `sync`.
    - `nvidia-powercap.service`: `-pl 130` → `-pl 100`. 100 W é o `power.min_limit` da placa, conferido antes.
    - `10-performance.conf`: `CUDA_VISIBLE_DEVICES` com os 4 UUIDs.
    - Backups em `/root/srv-backup-20261028/`: `nvidia-powercap.service.130w-*` e `10-performance.conf.3gpu-sem06-*`.
    - Reiniciados: `daemon-reload`, `nvidia-powercap`, `ollama` e `ollama-warm`.
  - **Conferido:**
    - as 4 GPUs a 100 W;
    - `qwen3-ia` 100% GPU com 64k (23 GB) e `nomic-embed-text` 100% GPU, **os dois residentes** (`ollama ps` e `list_models` do MCP; embed em 1,0 s);
    - `nvidia-powercap`, `ollama` e `ollama-warm` ativos, unit `enabled`.
  - **Erro meu de leitura, corrigido:** um `list_models` chamado em paralelo com o `embed` mostrou só 1 modelo em VRAM. Ele leu o estado antes da carga; em sequência, mostra os 2.
  - **CONEXAO-IA.md** atualizado (4 GPUs / 100 W, causa = fonte, seção "2026-10-28 noite"): cópia local e `/root/CONEXAO-IA.md` com md5 iguais (`a75a6f0c…`); backup da versão anterior do servidor em `/root/srv-backup-20261028/`.
  - **Gate do código:** `go build`, `go vet`, `gofmt -l` (vazio) e `go test -count=1 ./...` passam (10 pacotes, 22:14).
  - **Sigilo do report:** varredura sem caminhos de documentos, nomes de arquivo ou segredos; só ids neutros.
  - **Pendências atualizadas:** a 2 passa a ser **trocar a fonte por uma de maior potência**; depois disso, revalidar 130–150 W e o contexto de 131072. A 9 (commit) é feita neste registro. As outras continuam.
  - **Efeito na S7:** com 4 GPUs (32 GB), vale a projeção de 4 GPUs do report §S7. O `qwen3-ia` volta a fazer 5×32k.
- 2026-10-28 22:40 — **8.8 think:false no mcpollama (concluído no código; sem commit).** Decisão do owner (22:33): o mcpollama envia `"think": false` em toda chamada de geração.
  - **Motivo:** `gemma4:12b` e `qwen3.5:9b` têm thinking ligado por padrão; sem `think:false` pensam até o timeout (300 s, resposta vazia). O projeto nunca quer raciocínio (o campo `thinking` já não é decodificado, OLL-010).
  - **Implementação:** `src/ollama/api.go`, `generateWire` e `chatWire` ganharam `Think bool \`json:"think"\``, sem `omitempty` e sem flag. Escolhi `bool` simples, e não `*bool` com `omitempty`, porque o valor zero (false) é exatamente o que tem de ir no fio, e assim não há como esquecer de preenchê-lo. Como todas as tools (generate, chat, summarize_file, extract, translate) passam por `Client.Generate`/`Client.Chat`, um único ponto cobre tudo, nos modos stream e não-stream. `embedWire` ficou intacto.
  - **Testes novos:** `src/ollama/think_test.go` (corpo bruto de /api/generate e /api/chat contém `"think":false`, em stream e `DisableStream`; /api/embed não contém "think") e `src/tools/think_test.go` (tools generate, chat, summarize_file, extract, translate enviam `think:false`; tool embed não envia). Nenhum teste existente comparava o corpo exato, então nenhum precisou mudar. Descrições das tools inalteradas: TOOLS.md não regenerado; `TestToolsListBudget*` passa.
  - **Gate (dentro de `src/`):** `go build ./...` ok; `go vet ./...` ok; `gofmt -l .` vazio; `go test -count=1 ./...` ok em 10 pacotes (doctext 22 s); `go test -race -count=1 ./...` ok em 10 pacotes (doctext 159 s). Os 7 testes novos (2 em ollama, 5 em tools) passam em `-v`.
  - **Validação ao vivo (3 chamadas, uma por vez, binário em `scratchpad\think\mcpollama-mcp.exe`; `D:\MCP\mcpollama-mcp.exe` não foi tocado), prompt "Responda apenas: pong":**
    1. `qwen3-ia` (padrão): 0,2 s, eval_count=2, texto "pong", load_ms=0. Modelo sem thinking aceita `think:false`.
    2. `gemma4:12b`: 23,9 s no total, dos quais load_ms=23034 (carga a frio); eval_count=2, texto "pong", fim=stop. Não ficou pensando.
    3. `qwen3.5:9b`: 22,3 s, load_ms=21883; eval_count=2, texto "pong", fim=stop.
  - **O que não deu certo e solução:** (a) o 1º script de chamada falhou por erro meu de passagem de parâmetros do PowerShell (nenhuma chamada ao Ollama foi feita); reescrevi com função inline, sem perda de cota. (b) `curl -H @-` (token por stdin) devolveu `unauthorized` (3 respostas, sem efeito); usei `-H "Authorization: Bearer ..."` direto, sem imprimir o token (saídas redigidas).
  - **VRAM:** `keep_alive:0` por curl em `gemma4:12b` e `qwen3.5:9b` (done_reason=unload); `/api/ps` depois de ~7 s devolveu `{"models":[]}`: nenhum modelo residente; o `qwen3-ia` recarrega a frio (~30 s) na próxima chamada.
  - **Arquivos alterados:** `src/ollama/api.go`; novos `src/ollama/think_test.go` e `src/tools/think_test.go`; este plano.
  - **Pendências:** 1. Commit (não feito, por instrução). 2. Instalar o binário novo em `D:\MCP\mcpollama-mcp.exe` só após fechar os clientes (build com `-ldflags -X main.Version`), e reiniciar a sessão do Claude Code. 3. Mencionar `think:false` em `TOOLS.md`/README se o owner quiser (nenhuma descrição mudou). 4. Medir o bake-off sem a variante-patch (`variante-nothink.patch` passa a ser redundante).
- 2026-10-28 22:46 — **8.8 conferida pelo loop principal e efeito colateral corrigido.**
  - **Gate refeito como 2ª via:** `go build`, `go vet`, `gofmt -l` vazio e `go test -count=1` em `./ollama`, `./tools` e `./mcpollama-mcp` passam.
  - **Não deu certo:** ao carregar `gemma4`/`qwen3.5` nas chamadas ao vivo (`MAX_LOADED_MODELS=2`), o Ollama **descarregou o `qwen3-ia` e o `nomic` da produção**. Depois do `keep_alive:0` dos candidatos, a VRAM ficou vazia.
  - **Solução (22:45):** `systemctl restart ollama-warm` + embed com `keep_alive:-1`. O `qwen3-ia` (64k) e o `nomic` voltaram residentes a 100% GPU.
  - **Lição para a 8.9:** todo teste com candidato tira a produção do ar; restaurar e provar com `ollama ps` ao fim.
  - **Decisões de 22:20–22:43:**
    - D8-3 → "o foco é ter multiagentes trabalhando" + "executando os serviços com primazia perfeita";
    - **medir `gemma4:12b` × `qwen3.5:9b` com carga real, em degraus de 2 → 5 → 10 agentes**, e decidir;
    - 8.7 parcial executada às 22:35 (`ollama rm granite4.2:8b qwen3:14b`; 154 GB livres);
    - nginx liberado de vez às 22:36 (`limit_conn 12`, `rate=600r/m`, `burst=60`; backup `ollama.conf.nginx-223601`; `nginx -t` OK).
  - **Fase 9** (fila + `/monitor` + SQLite) planejada em `MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md`.
- 2026-10-28 22:53 — **8.9 carga multiagente: INÍCIO (em progresso).** Executor: agente Opus 5.5. Finalistas `gemma4:12b` e `qwen3.5:9b`; degraus 2×32k → 5×32k → 10×32k → 5×64k (sobe só se o anterior passar) + sub-degrau "prompt grande" em 5×32k e 10×32k.
  - **Ferramenta (deu certo):** `src/cmd/bakeoff/` estendida, sem criar outra: `multi.go` (modo `-suites multi`: N agentes = N processos mcpollama, jitter 0–2 s antes de cada tarefa, K tarefas alternando S2 de b01 e S3 JSON de b01/b02/b03; sub-degrau `-big-docs`; classificação de erro `http429`/`http5xx`/`timeout`/`conexao`/`protocolo`; **o 1º erro de conexão ou de protocolo mata todos os mcpollama do degrau** (código 3); `-expect-boot` + `uptime -s` antes e depois); `monpeak.go` (modo `-suites monpeak`: pico de W somado, VRAM e ΔAER de uma janela do monitor); `multi_test.go` (5 testes). `gofmt -l` vazio, `go vet` e `go test -count=1 ./cmd/bakeoff` verdes às 22:52.
  - **Binário de teste:** HEAD `ced50f8` (contém o `think:false` do e519221), em `scratchpad\bk89\mcpollama-mcp.exe` (versão `bk89-e519221`); a `variante-nothink.patch` não foi usada; `D:\MCP` intocado.
  - **Prompt grande:** nenhum documento do corpus dá 20–24 mil tokens sozinho (b03 = 7,8k; b04 e b07 passam de 85k). Solução: 10 fatias de ~80 mil caracteres do texto `doctext` de `b07` (`corpus/b07-g01..g10.txt`, ids neutros, registradas no `mapa.txt`), uma por agente, para não haver prefixo comum entre agentes.
  - **Servidor antes (22:52, `srv/pre89-out.txt`):** boot 21:10:46; AER `00:1b.4` 133 corrigíveis, não-corrigível 0, Xid 0; 4 GPUs a 100 W; `qwen3-ia` (64k, 23 GB) e `nomic` residentes a 100% GPU; só o `10-performance.conf` (+ `.bak`).
  - **Monitor** de potência/AER a 1 s (`srv/mon.sh` reaproveitado) desde 22:53:34 → `srv/mon-out-89.txt`. Scripts do degrau e da restauração: `srv/step89.sh` (bootok → para o `ollama-warm` → grava o drop-in com `test -s` + `mv` → `daemon-reload` → restart → 1 chamada curta → `ollama ps`/VRAM/journal → `RESULTADO CABE|NAO-CABE`) e `srv/restore89.sh`.
- 2026-10-28 22:59 — **8.9 `gemma4:12b` 2×32k: PASSOU** (`srv/step89-gemma4-2x32k.txt`, `out/multi/run-gemma4-2x32k.log`).
  - **Cabe:** `n_slots=2`, `n_ctx_slot=32768`, `ollama ps` 100% GPU, CONTEXT 32768, VRAM 16,3 GB somados (4,6/3,5/4,1/4,1); carga a frio 14,9 s.
  - **Carga (2 agentes × 3 tarefas, 22:54:18–22:58:02):** 6/6 ok, 0 erro; S2 66/66 nº+título (100%), JSON 3/3 (com cerca); S3 JSON 3/3 (2 estritos); 108 tok/s somados (prefill 93 + geração 15,2 sobre a janela); latência p50 44 s, p95 143 s (S2 com 1.201 tokens de saída); pico 266 W somados; AER 134 → 137; Xid 0.
  - **Não esperado (registrado):** o S2 levou 105–143 s com 2 agentes contra 42 s no report 8.5 (1 agente, 150 W). Para separar o efeito da potência do efeito da concorrência, rodei um **controle de 1 agente na mesma config** (22:58:14–22:59:19, `run-gemma4-1x32k-controle.log`): S2 em 58,8 s (1.201 tokens → ~20 tok/s de geração a 100 W, contra ~37 a 150 W) e S3 em 5,6 s; vazão somada 107 tok/s, **igual à de 2 agentes**. Leitura parcial: no `gemma4` a 100 W a vazão agregada **não cresce** com 2 agentes; a fila só divide o mesmo débito.
- 2026-10-28 23:16 — **8.9 `gemma4:12b` 5×32k: PASSOU nos critérios formais, mas com latência ruim** (`srv/step89-gemma4-5x32k.txt`, `run-gemma4-5x32k.log`, `run-gemma4-5x32k-grande.log`).
  - **Cabe:** `n_slots=5`, 100% GPU, CONTEXT 32768, VRAM 18,3 GB; carga 14,9 s.
  - **Carga (5×3, 23:00:07–23:11:56, 709 s):** 15/15 ok, 0 erro; S2 176/176 (100%), S3 JSON 7/7 (2 estritos); **113 tok/s somados** (prefill 99 + geração 14,0), **p50 158 s, p95 413 s** (S2 de 1.201 tokens levou 404–413 s); pico 273 W; AER 140 → 152; Xid 0.
  - **Diagnóstico (2ª via, monitor):** utilização média por GPU **25%** entre 23:02 e 23:08 com 5 pedidos ativos e potência de ~70–78 W por placa: o `gemma4` nesta versão do Ollama (runner com "speculative decoding context") **não ganha vazão com slots paralelos**; os pedidos dividem o mesmo débito de ~110 tok/s somados. Não é limite de potência nem de VRAM.
  - **Sub-degrau prompt grande (5 × 1 `summarize_file` de b07-g01..g05, 23:12:19–23:15:33):** 5/5 ok (resumos não cortados); prompt real de **12.661 a 20.418 tokens** (g01 rendeu menos tokens que o estimado; os outros ~20k); p50 183 s, p95 194 s; prefill agregado **476 tok/s** (contra ~1.140 tok/s com 1 pedido no report 8.5): a concorrência **reduz** o prefill agregado; pico 278 W; AER 152 → 153.
  - **Decisão de execução:** subir para 10×32k (a regra é passar = 0 erro de conexão, qualidade mantida, 100% GPU, sem evento de hardware; latência é medida, não critério de corte). Previsão registrada antes de rodar: S2 acima de 600 s → timeouts do `--timeout 10m`.
- 2026-10-28 23:35 — **8.9 `gemma4:12b` 10×32k: NÃO PASSOU (2 timeouts); 5×64k só checagem de "cabe".** (`step89-gemma4-10x32k.txt`, `run-gemma4-10x32k.log`, `step89-gemma4-5x64k-so-cabe.txt`)
  - **Cabe:** `n_slots=10`, 100% GPU, 19,1 GB.
  - **Carga (10×3, 23:16:23–23:34:46, 1.104 s):** 28/30 ok e **2 timeouts de 600 s**, ambos em S2 da primeira rajada (erro de aplicação, não aborta). S2 286/286 nas 13 concluídas (100%); S3 JSON 15/15; **128 tok/s somados**; p50 245 s, p95 548 s; pico 275 W; AER 153 → 155; utilização média por GPU de 25% (2ª via, monitor).
  - **Consequência pela regra:** o sub-degrau "prompt grande" de 10×32k e a carga do 5×64k **não foram executados**.
  - **Acréscimo:** checagem de "cabe" do 5×64k, sem carga: 100% GPU, CONTEXT 65536, `n_slots=5`, 19,0 GB.
- 2026-10-28 23:48 — **8.9 `qwen3.5:9b` 2×32k → 5×32k → 10×32k → 5×64k: TODOS PASSARAM, com uma surpresa de método.**
  - **Não deu certo como planejado:** o Ollama **não roda o `qwen3.5` em paralelo**. O journal (`srv/journal89-qwen35-parallel.txt`) registra `msg="model architecture does not currently support parallel requests" architecture=qwen35`, `n_seq_max=1` e `n_slots=1` com qualquer `NUM_PARALLEL`. A S7 do 8.5 (por cálculo) supunha slots paralelos e não vale para este modelo.
  - **Solução:** rodei os degraus assim mesmo. Eles medem N agentes contra uma fila serial, que é o que os agentes vão encontrar de fato.
  - **Cabe:** 9,4 GB com 32k e 14,2 GB com 64k, 100% GPU em todos; carga a frio de ~9,7 s.
  - **Controle, 1 agente:** S2 18/22 em 9,2 s; 479 tok/s.

    | Degrau | Janela | S2 | S3 JSON | tok/s somados | p50 / p95 (s) | Erros | Pico | AER |
    | --- | --- | --- | --- | --- | --- | --- | --- | --- |
    | 2×32k | 35 s | 84,8% | 3/3 | 636 | 10 / 16 | 0 | 355 W | 156 → 156 |
    | 5×32k | 93 s | 80,1% (uma em 11/22) | 7/7 | 787 | 25 / 42 | 0 | 357 W | 157 → 158 |
    | 5×32k prompt grande | 43 s | — | — | 2.061 (5/5; 10,9–19,6k tokens) | 23 / 42 | 0 | 386 W | 158 → 158 |
    | 10×32k | 203 s | 83,9% | 15/15 | 707 | 63 / 83 | 0 | 359 W | 158 → 159 |
    | 10×32k prompt grande | 93 s | — | — | 1.989 (10/10; 10,9–19,9k tokens) | 47 / 92 | 0 | 387 W | 159 → 160 |
    | 5×64k | 100 s | 79,5% (uma em 11/22) | 7/7 | 729 | 29 / 39 | 0 | 349 W | 160 → 160 |

- 2026-10-28 23:50 — **8.9 CONCLUÍDA; restauração provada; report escrito.**
  - **Restauração (23:48:31–23:49:03, `srv/restore89-out.txt`):**
    1. `99-bakeoff.conf` removido: `ls` mostra só `10-performance.conf` + `.bak`; `systemctl show` mostra `NUM_PARALLEL=1`, `CONTEXT_LENGTH=65536`, `MAX_QUEUE=8` e `MAX_LOADED_MODELS=2`;
    2. `daemon-reload` + restart do `ollama` e do `ollama-warm` (os dois `active`);
    3. embed do `nomic` com `keep_alive:-1`: HTTP 200 em 1,04 s;
    4. `ollama ps`: `qwen3-ia` 23 GB, 100% GPU, 65536, e `nomic-embed-text` 851 MB, 100% GPU, os dois Forever.
  - **Saúde do servidor:** boot 21:10:46 do início ao fim; Xid 0; AER `00:1b.4` 133 → 160 corrigíveis; não-corrigível e fatal 0; nenhum `ALERTA`; pico da sessão 387 W.
  - **Processos:** o monitor parou (`MON-FIM 23:49:03`). Nenhum processo meu ficou rodando (conferido às 23:49). Os 2 `ssh 10.100.2.2` abertos às 21:15 e às 21:56 não são desta etapa e não foram tocados.
  - **Report:** `.workspace/reports/MCPOllam2026-10-28-235002-fase8-9-carga-multiagente.report.md`.
  - **Recomendação:**
    - nenhum finalista atende "multiagente + qualidade impecável";
    - o `gemma4` tem qualidade impecável sob carga (S2 25/25, S3 26/26), mas a vazão não escala (~110–130 tok/s somados com 1 a 10 agentes);
    - o `qwen3.5` tem vazão ~6× maior e 0 erro com 10 agentes, mas S2 de ~80–84%;
    - pelo critério de "primazia perfeita", fica o `gemma4:12b` com capacidade declarada de ~1 S2 por minuto no total.
    - **D9-3: `NUM_PARALLEL=1`** (no `qwen3.5` o Ollama ignora valores maiores; no `gemma4` eles não dão vazão) **e `MAX_QUEUE` ≥ 16** (o 8 atual recusaria o 9º pedido em fila).
  - **Gate do código:** `gofmt -l` vazio, `go vet` e `go test -count=1 ./cmd/bakeoff` e `go build ./...` refeitos às 23:52, todos verdes. Sem commit.
  - **Pendências (numeradas):**
    1. D8-3 (owner).
    2. Medir o `gemma4` sem a decodificação especulativa (`gemma4-assistant`), suspeita da falta de escala.
    3. Medir o `qwen3-ia` sob a mesma carga multiagente.
    4. D9-3: `NUM_PARALLEL=1` + `MAX_QUEUE` ≥ 16.
    5. Revalidar a 130–150 W depois da troca da fonte.
    6. Commit das extensões do `bakeoff` (decisão do owner).
    7. Prompt grande de 22–24 mil tokens, se o owner quiser (fatias de ~90 mil caracteres).

- 2026-10-29 15:12 — **8.8b think por chamada: CONCLUÍDA (sem commit).** Decisão do owner (2026-10-29 15:0x): a IA que usa o MCP escolhe, por chamada, se quer raciocínio.
  - **O que foi feito:**
    1. Argumento opcional `think` (boolean, padrão false) em `generate`, `chat`, `summarize_file`, `extract` e `translate`; `embed` e `list_models` não têm. Descrição: "Reason first: slower; for hard analysis. Default false."
    2. `ollama.GenerateRequest.Think` e `ollama.ChatRequest.Think` (novos); `generateWire`/`chatWire` seguem com `Think bool` sem omitempty: sem o argumento vai `"think":false` (como na 8.8), com `think:true` vai `"think":true`. `embedWire` não tem o campo.
    3. Tools de arquivo: o valor entra em `fileJob.think` e vale para todas as chamadas (partes e combinação do map-reduce). `runFile` ganhou o parâmetro `think`.
    4. Linha `[uso]`: acrescenta ` think=on` no fim só quando ligado (e `think:true` em `_meta`); desligado, a linha não muda.
    5. Resposta vazia + `done_reason=length` com think ligado vira erro: "o raciocínio (think) consumiu o limite de tokens e a resposta veio vazia; aumente `num_predict` ou use `think=false`" (`thinkExhausted`, em `usage.go`, usada por generate, chat e `fileJob.call`). O campo thinking continua não decodificado (OLL-010). Sem think, ou com resposta não vazia, o comportamento atual se mantém.
  - **Arquivos:** `src/ollama/api.go`, `src/ollama/types.go`, `src/tools/args.go` (`optThink`), `src/tools/schema.go` (`thinkProp`), `src/tools/generate.go`, `src/tools/chat.go`, `src/tools/filetools.go`, `src/tools/usage.go`, `src/tools/registry_test.go` (budgets), `src/tools/filetools_docs_test.go` (assinatura de `runFile`), `src/tools/think_test.go` e `src/ollama/think_test.go` (testes novos), `TOOLS.md` (regenerado por `go run ./cmd/dumptools`).
  - **Budget do `tools/list`:**
    - `toolsListBudget` (4 tools base): 4400 → **4620** (medido 4390 → **4588**, +198; folga 32 bytes);
    - `toolsListBudgetFull` (com `--allow-root`): 7800 → **8330** (medido 7800 → **8295**, +495; folga 35 bytes);
    - comentário das constantes atualizado ("owner 2026-10-29: think por chamada" + antes → depois).
  - **O que deu certo / o que não deu:**
    1. Deu certo: tudo no padrão existente (`boolProp`, `args.go`); nenhum arquivo proibido tocado (`src/files`, `doctext`, `ocr`, `gateway`).
    2. Não deu certo de primeira: com a descrição sugerida pelo owner ("Reason before answering: slower; may help on hard analysis. Default false.") o payload ficou em 4626 / 8390; **solução:** encurtar para "Reason first: slower; for hard analysis. Default false." (-38 / -95 bytes).
    3. Primeira tentativa de aplicar as edições por script em heredoc falhou por aspas no shell (nada foi alterado); refeita por script em arquivo.
    4. Quebra de compilação em `filetools_docs_test.go` (chamada direta a `runFile`); corrigido com `false`.
  - **Testes novos (httptest):** schema (5 com `think` boolean, 2 sem); `think:true` na rede em generate e chat (e no cliente `ollama`, com e sem stream); sem argumento segue `"think":false`; map-reduce com 2+ partes + combinação todas com `think:true` (e todas `false` sem o argumento); `think=on` na `[uso]` e ausência dele quando desligado; tipo inválido (`"sim"`) é recusado antes de chegar ao servidor; vazio + `length` com think em generate, chat e extract dá o erro claro; sem think e com texto parcial não dá erro. `TestSchemaConformance`, `TestDescriptionsShortAndStatic`, os budgets e `TestCatalogInSync` passam.
  - **Gate (dentro de `src/`):** `go build ./...` e `go vet ./...` limpos; `gofmt -l .` vazio; `go test -count=1 ./...` todos `ok` (nenhuma falha); `go test -race -count=1 ./tools ./ollama ./mcpollama-mcp` → 3 `ok`.
  - **Ao vivo (3 chamadas, uma por vez, binário em `scratchpad\think2\`, `D:\MCP` intocado, token só por arquivo):**
    1. `generate` "17*23" sem think: **391** (correto), `eval_count=4`, `total_ms=280`, 0,4 s de parede;
    2. mesma com `think:true`: **391** (correto), `eval_count=377`, `total_ms=7273`, 7,3 s; `[uso]` com ` think=on` e `fim=stop`. Mesmo acerto, ~94× mais tokens e ~26× mais tempo: para conta simples o think só custa;
    3. `think:true` + `num_predict:50`: erro `isError` com a mensagem do item 5 (o raciocínio gastou os 50 tokens, resposta vazia + `length`), em 1,2 s.
  - **Pendências (numeradas):**
    1. Redeploy do binário em `D:\MCP\mcpollama-mcp.exe` (não feito, proibido nesta etapa) e reiniciar a sessão do Claude Code para a IA ver o argumento.
    2. Commit (decisão do owner; stage por caminho).
    3. Orientar no CLAUDE.md global/README quando usar `think` (ex.: análise difícil, com `num_predict` folgado ≥ 1500); hoje só a descrição curta do schema orienta.
    4. Decidir se o `bakeoff` deve também expor `think` (não alterado).
- 2026-10-29 15:20 — **D8-3 executada: `qwen3-ia` = `qwen3.5:9b` + 8.7 + 8.8b** (loop principal; detalhe também em `CONEXAO-IA.md` §"2026-10-29").
  - **Decisão do owner:** adotar o `qwen3.5:9b` (foco multiagente), com alias `qwen3-ia` e limpeza de `gemma4` e `qwen3-coder`.
  - **Execução:**
    - merge da Fase 9.1–9.5 (`fa8c95e`; o conflito era só no changelog do plano, com as duas metades mantidas; gate completo verde, build Linux ok);
    - mcpollama `0.8.0-fa8c95e` em `D:\MCP` (o 0.7.0 virou `.exe~`);
    - 14:00 `ollama cp qwen3.5:9b qwen3-ia` + restart: os dois residentes a 100% GPU (13 GB a 64k). 2ª via: o binário novo via stdio deu `generate` 0,16 s e `summarize_file` 2,5 s;
    - 14:05 `ollama rm gemma4:12b qwen3-coder:30b-a3b-q4_K_M` (179 GB livres);
    - 14:59 `OLLAMA_MAX_QUEUE` 8 → 32 (owner; com 8, o 10º pedido simultâneo recebia 503).
  - **Raciocínio do `qwen3.5`, testado no loopback:**
    - na API nativa sem `think`, a resposta vem vazia (400 tokens gastos raciocinando);
    - `/v1` sem parâmetro raciocina (5,7 s para "pong"); `reasoning_effort:"none"` desliga, `think:false` no `/v1` não desliga;
    - o Modelfile não aceita `think`.
    - Consequência: o `ia-local` raciocina até o gateway (9.6) poder desligar.
  - **8.8b (owner: "que a escolha seja da AI que vai usar o MCP"):** argumento `think` por chamada nas 5 tools de geração (agente sonnet; gate refeito pelo loop principal como 2ª via).
    - Budget do `tools/list`: 4400→**4620** (medido 4588) e 7800→**8330** (medido 8295).
  - **Pendências:**
    1. Reiniciar as sessões do Claude Code (MCPOllam e GestorERP) para pegar o binário novo.
    2. Autorização do owner para atualizar o `CLAUDE.md` global e o comentário do `ia-local.ps1` (ainda descrevem o MoE antigo).
    3. Gateway (9.6): desligar o raciocínio no `/v1` por padrão e manter a escolha (nome `qwen3-ia-think` ou header).
