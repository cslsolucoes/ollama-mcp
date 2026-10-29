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
