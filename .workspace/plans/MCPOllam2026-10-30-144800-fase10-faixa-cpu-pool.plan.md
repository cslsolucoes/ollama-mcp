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
