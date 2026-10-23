---
title: "Resposta ao incidente: prompt grande derruba o Ollama da CSL — causa diagnosticada"
created: 2026-10-23
responde-a: "MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.report.md"
status: RESOLVIDO (mitigado; pendência física de hardware)
locale: pt-BR
---

# Resposta ao incidente — causa diagnosticada e mitigada

> Investigação feita **com acesso SSH ao servidor** (10.100.2.2), que o report original
> não tinha. Os logs do kernel confirmaram a causa. Resumo: **não era o Ollama caindo —
> era a MÁQUINA reiniciando**, por falha física de PCIe numa GPU sob a carga de prefill
> de prompts grandes. O owner suspeitou de hardware e estava certo.

## 1. Causa raiz (provada)

**O servidor REINICIAVA a cada prompt grande** — não era o serviço `ollama` nem o runner
que caíam. Cadeia de evidência:

| Evidência | O que prova |
| --- | --- |
| `last reboot` mostra reboots às **16:20, 16:21, 16:23 e 18:11** | batem **exatamente** com os 4 prompts grandes do report |
| `systemctl show ollama -p NRestarts` = **0** | o ollama nunca reiniciou sozinho; foi a máquina |
| `/var/log/messages`: enxurrada de **AER PCIe** na ponte **Intel Root Port `00:1c.5`** seguida de **corte seco** (sem shutdown limpo) | reboot abrupto forçado pelo kernel/driver |
| `NVRM: GPU lost from the bus [NV_ERR_GPU_IS_LOST]` + `uvm encountered global fatal error 0x60, requiring os reboot to recover` | o **driver NVIDIA força o reboot** quando a GPU cai do barramento (Xid) |
| GPU 3 (`07:00.0`, atrás de `00:1c.5`) negocia **PCIe Gen1 x1** (devia Gen3 x16) | sinal elétrico PCIe degradado nessa placa (riser/cabo) |

**Por que o prompt pequeno funciona e o grande não:** o prompt de 44 tokens quase não
usa a GPU (prefill trivial). O prompt grande força um **prefill de ~16.000 tokens** que
satura as 4 GPUs ao mesmo tempo (pico medido ~900 W num stress anterior). Nesse pico, a
ligação PCIe fraca da **GPU 3** cede → Xid "GPU lost from the bus" → reboot. É a mesma
GPU/slot que já dera problema de manhã (bug-023); a troca de riser melhorou mas **não
eliminou** — as placas voltaram a negociar Gen1.

## 2. Respostas diretas às perguntas em aberto do report

| Pergunta do report | Resposta |
| --- | --- |
| O processo `ollama serve` reiniciou ou só o runner? | **Nenhum dos dois** — a **máquina inteira** rebootou. `NRestarts=0`. |
| Houve OOM ou Xid na GPU? | **Xid**, não OOM de VRAM. `GPU lost from the bus` + `uvm fatal error 0x60`. (Com 4 GPUs o modelo cabia em 25,6 GB; não foi falta de VRAM que derrubou.) |
| Por que a VRAM ficou vazia (inclusive `nomic`)? | Porque a **máquina reiniciou** — tudo descarrega no reboot. |
| Por que o RST em vez de erro HTTP? | O host reiniciou no meio do processamento; o TCP foi cortado pelo reboot, não pelo nginx/ollama. |
| Por que os 90 s nos prompts de 68k/114k? | Como o report inferiu: recarga a frio (~32–36 s) + ~45 s até cair de novo, porque cada requisição seguinte pegou o servidor acabando de reiniciar. |
| Limite de prompt que funciona | Com a mitigação (3 GPUs/64k), **42.540 chars = 15.950 tokens responde em 4 s**, sem reboot. O teto provisório do mcpollama pode ser **removido** nesta config (ver §4). |
| Foi limite do nginx? | **Não.** Daria 413/504 e não explicaria o reboot. |

## 3. Mitigação aplicada (servidor estável agora)

Como o problema é físico (GPU 3) e a decisão foi mitigar já por software:

- **ollama passou a usar só 3 GPUs:** `CUDA_VISIBLE_DEVICES=0,1,2` (a GPU 3 / slot 07,
  a defeituosa, foi **excluída**).
- **Contexto reduzido para 64k** (`OLLAMA_CONTEXT_LENGTH=65536` + modelo `qwen3-ia` com
  `num_ctx 65536`). Motivo: a 128k o modelo (25 GB) **não cabe** nas 3 GPUs restantes
  (24 GB) — dava `cudaMalloc out of memory`. A 64k cabe 100% GPU (21 GB) com folga.
- **Modelo principal** continua `qwen3-ia` (= `qwen3-coder:30b-a3b`, MoE, ~90 tok/s).

**Validação (prova empírica), com o mesmo prompt que derrubava:**

| Item | Resultado |
| --- | --- |
| Prompt | 43.150 bytes (~42k chars, 500 linhas) |
| Reboot? | **NÃO** (uptime subiu, não resetou) |
| Resposta | `primeira=00001 ultima=00500` (correta) |
| Prompt eval | 15.950 tokens em 3,3 s (~4.800 tok/s de prefill) |
| Tempo total | **4 segundos** (antes: reboot aos 45 s) |
| Xid/AER | **nenhum** |

## 4. Impacto no mcpollama

- **Limite de prompt:** a proposta PB5-1 (teto abaixo de 42.540 chars) **pode ser
  relaxada nesta config** — 42k chars já passa. O teto real agora é o **contexto de 64k
  tokens** (~200k chars de entrada, deixando espaço para a saída). Recomendo o mcpollama
  usar um teto de entrada conservador (ex.: ~48k tokens) e deixar o resto para a geração.
- **Contexto:** mudou de 128k para **64k** enquanto a GPU 3 está fora. Voltará a 128k
  quando o hardware for corrigido e as 4 GPUs voltarem.
- **Nível `local` (`ia-local.ps1`, `qwen3.8:27b-64k`):** herda a mesma config de 3 GPUs,
  então também está protegido do reboot.

## 5. Pendência física (para o owner, na bancada)

A GPU 3 (slot 07, bus `07:00`, atrás do Intel PCIe Root Port `00:1c.5`) tem **contato
PCIe deficiente** (Gen1 x1 em vez de Gen3 x16). Para recuperar as 4 GPUs e os 128k:

1. Trocar/reassentar o **riser e o cabo PCIe** da GPU 3, ou pô-la num **slot x16 direto**.
2. Verificar a **PSU** — se não aguenta o pico das 4 GPUs no prefill, a GPU mais fraca cai.
3. Após o reparo: revalidar link Gen3, voltar `CUDA_VISIBLE_DEVICES=0,1,2,3` e `128k`, e
   repetir este teste de prompt grande.

**Melhoria de observabilidade já feita:** o **journal passou a ser persistente**
(`/var/log/journal`), então o próximo evento de GPU/kernel ficará gravado com precisão
(hoje os logs do momento do crash perdiam-se no reboot, por o journal ser volátil).

## 6. Ficheiros / referência no servidor

- Config ollama: `/etc/systemd/system/ollama.service.d/10-performance.conf`
  (backup da versão 4 GPUs/128k: `/root/ollama-10-performance.conf.bak-20261023-191125`).
- Guia de conexão/operação (atualizado com este incidente): `/root/CONEXAO-IA.md`
  (cópia local em `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md`).
- Setup detalhado: `/root/OLLAMA-SETUP-DETALHADO.md`.
