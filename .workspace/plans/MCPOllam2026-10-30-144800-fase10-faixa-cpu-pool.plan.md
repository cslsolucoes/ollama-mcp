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
| 10.1 | 9.9 | Inventário + **spike de medição** com uma `ollama-cpu` **temporária** (porta 11435, sem tráfego de produção): prefill e geração por modelo e por `num_thread`, carga a frio, RAM, embed do `nomic` na CPU × GPU, qualidade S2/S3 na CPU × GPU | opus | pendente |
| 10.2 | 9.10 | **Interferência e energia:** carga na CPU junto com carga na GPU; queda de vazão da GPU, temperatura e potência (GPU por `nvidia-smi` + CPU por RAPL como root); define `CPUAffinity`, `CPUQuota` e `num_thread` | opus (mesmo agente da 10.1) | pendente |
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
