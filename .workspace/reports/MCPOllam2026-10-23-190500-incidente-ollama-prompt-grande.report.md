---
title: "Incidente: prompt grande derruba o Ollama da CSL (qwen3-ia)"
created: 2026-10-23
status: mitigado (causa diagnosticada pelo owner; pendência física de hardware)
locale: pt-BR
---

# Incidente: prompt grande derruba o Ollama da CSL

> **Desfecho (2026-10-23 19:16–19:18).** Causa diagnosticada pelo owner, com SSH, em
> `MCPOllam2026-10-23-191600-resposta-incidente-ollama-prompt-grande.report.md`.
>
> **O que caía era a máquina inteira.** O servidor reiniciava porque o PCIe da **GPU 3** falhava (`07:00.0`, atrás do Root Port `00:1c.5`, negociando Gen1 x1). Sob o prefill do prompt grande, o driver registrava `GPU lost from the bus` / `uvm fatal error 0x60` e forçava o reboot.
> - `last reboot` registra 16:20, 16:21, 16:23 e 18:11, os mesmos horários das 4 requisições grandes abaixo;
> - `NRestarts=0` no `ollama`.
>
> **Mitigação:** o `ollama` passou a usar 3 GPUs (`CUDA_VISIBLE_DEVICES=0,1,2`) e `num_ctx` **65536**.
>
> **2ª via, feita pelo caminho do cliente** (estação `10.100.2.30` → nginx `:8443`) às 19:18:39–19:18:43, evidência em `...evidencias/saida-b4-2via.txt`:
> - `/api/ps`: `qwen3-ia` com `context_length` 65536 e `size_vram` 21.660.958.717;
> - o **mesmo prompt de 500 linhas / 42.540 caracteres** → 200, **`prompt_eval_count` 15.959 em 3.199 ms (~4.988 tok/s)**, 1º chunk aos 3.249 ms, total de 3.577 ms, resposta correta `primeira=00001 ultima=00500`;
> - `/api/ps` depois: modelo ainda carregado.
>
> **Correção da minha inferência:** abaixo eu escrevi "queda e reinício do serviço `ollama` (ou do runner)". **Estava errado.** Era o reboot da máquina, e uma falta de memória na GPU também foi descartada. Os fatos medidos (horários, 45 s, RST, VRAM vazia, os 90 s = recarga + queda) continuam corretos e foram confirmados pelo `last reboot`. O texto original foi mantido abaixo como registro.

## Resumo

Em 2026-10-23, durante a validação empírica do servidor (onda B4 do brief do mcpollama), **4 requisições `POST /api/generate` com prompts grandes** foram encerradas pelo servidor sem resposta: o host remoto derrubou a conexão TCP.
- A **primeira caiu a ~45 s**, e depois disso **o modelo `qwen3-ia` deixou de aparecer no `/api/ps`**, apesar de `keep_alive` infinito.
- **A falha se reproduziu**: às 18:12, com o modelo recém-carregado, um prompt de 42.540 caracteres caiu de novo a 45 s, **antes de qualquer cabeçalho HTTP**. Logo depois, **nenhum modelo** estava carregado.
- Requisições pequenas, com prompt de 44 tokens, funcionaram normalmente o tempo todo, a ~97 tok/s.

**A causa não foi diagnosticada.** Os logs do servidor não foram lidos: SSH estava fora do âmbito, e o owner optou por investigar.

## O que eu estava fazendo

- **Tarefa:** onda B4 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md`, a validação empírica do servidor Ollama, aprovada pelo owner. O objetivo era medir latência, tokens/s, o contexto real e, em particular, **como o servidor se comporta com prompts grandes**, porque as tools de arquivo do mcpollama vão mandar arquivos inteiros ao modelo.
- **Ferramenta:** uma sonda em Go (`b4probe`, no scratchpad da sessão, fora do repositório), com a mesma pilha `net/http` + `crypto/x509` que o mcpollama vai usar.
- **Disciplina de carga:** uma chamada de cada vez, nunca em paralelo, respeitando a fila única de GPU.
- **Somente leitura no servidor:** nenhuma escrita, pull ou delete, e nenhum SSH.

**Origem das requisições:** estação `10.100.2.30` → `https://ia.cslsolucoes.com.br:8443`. O DNS da LAN resolve o host para `10.100.2.2`, então o tráfego vai direto ao nginx, sem Cloudflare. Autenticação com `Authorization: Bearer`, token `[omitido]`, lido do `%USERPROFILE%\.openclaude\csl-ollama.env`.

**Conteúdo dos prompts grandes:** texto sintético, sem nenhum dado real:

```
Execucao <nonce numérico único>.
Linha 00001: o servidor processou a requisicao 1 do cliente e devolveu status 200.
Linha 00002: o servidor processou a requisicao 2 do cliente e devolveu status 200.
...
Responda apenas no formato 'primeira=NNNNN ultima=NNNNN' com os numeros da primeira e da ultima linha do texto acima.
```

Corpo JSON: `{"model":"qwen3-ia","prompt":<texto>,"stream":false|true,"options":{"num_predict":24,"temperature":0}}`. Sem `num_ctx` na requisição, então vale o do Modelfile, 131072. O nonce no início impede que o Ollama reaproveite o cache de prompt entre chamadas.

## Linha do tempo

Horários da estação, que estão **alinhados com o relógio do servidor** (ver "Relógio" abaixo). Cada linha é um evento lido da saída da sonda.

### Execução 1: 16:18:25 → 16:23:57

| Hora | Requisição | Resultado |
| --- | --- | --- |
| 16:18:25 | 3× `GET /api/tags` em `:8443`, um por pool de CAs | 1 falha de TLS no cliente (CA não confiada, esperado) e 2× **200** em 8 ms |
| 16:18:25–16:19:28 | 3× `GET https://ia.cslsolucoes.com.br/api/tags` (sem porta) | timeout de conexão de 21 s em `10.100.2.2:443` (esperado de dentro da LAN; **não** chegou ao nginx) |
| 16:19:28 | `GET /api/ps` | **`qwen3-ia:latest` carregado**: `size_vram` 25.562.028.438, `context_length` 131072. Última requisição anterior ao modelo: 15:36:48 |
| 16:19:28–16:19:29 | `GET /api/tags`, `POST /api/show` (`qwen3-ia`) | 200; `num_ctx 131072` |
| 16:19:29 | `GET /api/tags` **sem** token | **401** `{"error":"unauthorized"}` |
| 16:19:29 | `POST /api/pull` (modelo inexistente `modelo-inexistente-b4:none`) | **403** do nginx |
| 16:19:29–16:19:30 | 3× `POST /api/generate`, prompt de 44 tokens, `stream:false` | **200**, 437–536 ms, ~97 tok/s, `load_ms` 0 |
| **~16:19:30 → 16:20:15** | `POST /api/generate`, **500 linhas, 42.540 caracteres**, `stream:false` | **conexão derrubada pelo servidor aos 45.026 ms**. Porta de origem `10.100.2.30:53358`. Erro no cliente: `wsarecv: Foi forçado o cancelamento de uma conexão existente pelo host remoto` (= RST) |
| **16:20:15 → 16:21:45** | `POST /api/generate`, **800 linhas, 68.040 caracteres**, `stream:false` | **derrubada aos 90.049 ms**. Porta de origem `:25791` |
| **16:21:45 → 16:23:15** | `POST /api/generate`, **1.333 linhas, 113.679 caracteres**, `stream:false` | **derrubada aos 90.026 ms**. Porta de origem `:30430` |
| 16:23:15 → 16:23:36 | `POST /api/embed` (`nomic-embed-text`, 1 frase) | **200**, 768 dimensões. `load_ms` 3.230, `total_duration` 9.936 ms, mas **wall 20.896 ms**: ~11 s de espera antes de o Ollama começar a contar |
| 16:23:36–16:23:57 | `POST /api/generate` pelo host sem porta | timeout de conexão de 21 s (não chegou ao servidor) |
| **16:23:57** | `GET /api/ps` | **só `nomic-embed-text`. O `qwen3-ia` NÃO está mais carregado** |
| 16:24:53 | `GET /api/ps` | idem: só `nomic-embed-text` |

### Execução 2 (reteste aprovado pelo owner): 18:10:47 → 18:12:22

| Hora | Requisição | Resultado |
| --- | --- | --- |
| 18:10:48 | `GET /api/ps` | só `nomic-embed-text` (o `qwen3-ia` continuava fora desde a execução 1) |
| 18:10:48 → 18:11:24 | `POST /api/generate` curto ("Responda so: ok") | **200**, `load_ms` **36.449** (carga a frio do `qwen3-ia`) |
| 18:11:24 | `GET /api/ps` | **só `qwen3-ia`** (25.562.028.438 bytes, ctx 131072). O `nomic-embed-text` foi **desalojado** pela carga |
| **18:11:24 → 18:12:09** | `POST /api/generate`, **500 linhas, 42.540 caracteres**, **`stream:true`** | **derrubada aos 45.019 ms, antes de receber os cabeçalhos HTTP** (nenhum chunk). Porta de origem `:24760`. Mesmo erro (RST) |
| 18:12:09 → **18:12:22** | `GET /api/ps` | **`models: []`, nenhum modelo carregado**. Esse `GET` simples levou **~13 s** para responder |

### Execução 3 (recarga aprovada pelo owner): 18:28:31 → 18:29:04

| Hora | Requisição | Resultado |
| --- | --- | --- |
| 18:28:31 → 18:29:04 | `POST /api/generate` curto | **200**, `load_ms` **31.963** |
| 18:29:04 | `GET /api/ps` | `qwen3-ia` residente de novo (`expires_at` infinito). **Estado normal restaurado.** |

## Relógio (os horários servem para filtrar os logs)

O `expires_at` do `/api/ps` com `keep_alive` infinito é "hora da última requisição + `math.MaxInt64` ns" (106.751 dias + 23:47:16,854). Subtraindo essa duração:

| `expires_at` devolvido | Última requisição (servidor) | Evento na estação |
| --- | --- | --- |
| `2319-02-02T16:10:53.427` (`nomic`) | 2026-10-23 **16:23:36** | fim do embed: **16:23:36** |
| `2319-02-02T18:16:21.129` (`qwen3-ia`) | 2026-10-23 **18:29:04** | fim da recarga: **18:29:04** |
| `2319-02-02T15:24:05.630` (`qwen3-ia`, antes dos testes) | 2026-10-23 15:36:48 | uso anterior, não meu (o modelo foi modificado às 15:36:22) |

**Conclusão:** os relógios estão alinhados ao segundo. As janelas abaixo valem direto no `journalctl`.

## O que os fatos mostram (e o que não mostram)

**Fatos:**
1. Prompt de 44 tokens: funciona sempre.
2. Prompt de 42.540 caracteres: falha **sempre a ~45 s**, com o modelo já carregado (16:19:30 e 18:11:24). A falha é um **RST do lado do servidor**, sem resposta HTTP de erro: nem 500/502/504 do nginx, nem JSON de erro do Ollama.
3. `stream:true` **não muda nada**. A falha acontece antes de qualquer byte de resposta, ou seja, ainda no processamento do prompt.
4. Depois da falha, **os modelos saem da VRAM**. Na execução 2, nenhum modelo sobrou, e um `GET /api/ps` levou ~13 s para responder.

**Inferências (não provadas, para orientar a leitura dos logs):**
- **Queda e reinício do serviço `ollama` (ou do runner)** ao processar o prompt grande. O que aponta para isso:
  - a VRAM ficou vazia, inclusive do `nomic-embed-text`, que não tinha nada a ver com a requisição;
  - o `/api/ps` lento logo depois;
  - o RST em vez de um erro HTTP.
- **Os 90 s das requisições de 68 mil e 114 mil caracteres** batem com "carga a frio do modelo (~32–36 s) + os mesmos ~45 s até a queda". Se o serviço já tinha caído às 16:20:15, cada requisição seguinte pagou a recarga e caiu de novo, o que explica os 90 s em vez de 45 s.
- **Os ~11 s de espera do embed** (16:23:15–16:23:36) são compatíveis com o serviço ainda subindo ou descarregando após a 3ª queda.
- **Candidatos à causa:**
  - falta de memória na GPU durante o prompt eval de ~10–15 mil tokens (estimativa: 42.540 caracteres a ~3–4 caracteres/token, **não medido**, porque nenhuma requisição grande devolveu contadores), com 4× RTX 3070 de 8 GB e 25,6 GB já ocupados pelo modelo;
  - interação de `OLLAMA_KV_CACHE_TYPE=q4_0` com a atenção ou o lote de prompt;
  - menos provável, algum limite do nginx, porque um limite do nginx devolveria 413/504 e não explicaria a VRAM vazia.

**O que NÃO está estabelecido:**
- o tamanho limite que funciona (entre 44 tokens e 42.540 caracteres);
- se o processo `ollama serve` reiniciou ou só o runner;
- se houve OOM ou Xid na GPU;
- a velocidade de prompt eval com prompt grande.

## O que procurar no servidor (10.100.2.2)

Comandos **só de leitura**, a rodar pelo owner. As janelas já incluem margem.

```bash
# 1) Ollama: queda, OOM, reinício do runner/serviço
journalctl -u ollama --since "2026-10-23 16:19:00" --until "2026-10-23 16:24:30" --no-pager
journalctl -u ollama --since "2026-10-23 18:11:00" --until "2026-10-23 18:13:00" --no-pager
systemctl show ollama -p NRestarts -p ActiveEnterTimestamp -p ExecMainStartTimestamp

# 2) GPU / kernel: Xid, AER, OOM killer
journalctl -k --since "2026-10-23 16:19:00" --until "2026-10-23 18:13:00" --no-pager | grep -iE 'xid|aer|oom|killed process|nvrm'

# 3) nginx: o que ele viu dessas conexões (cliente 10.100.2.30)
#    (caminhos padrão do nginx; o guia não diz onde ficam os logs deste vhost - confirmar em /etc/nginx/conf.d/ollama.conf)
grep -E '2026/10/23 (16:(19|2[0-4])|18:1[12])' /var/log/nginx/error.log
grep '10.100.2.30' /var/log/nginx/access.log | grep -E '23/Oct/2026:(16:(19|2[0-4])|18:1[12])'
grep -nE 'proxy_(read|send|connect)_timeout|client_max_body_size|client_body|limit_req|reset_timedout' /etc/nginx/conf.d/ollama.conf /etc/nginx/nginx.conf
```

**Pistas para correlacionar:**

| Momento | O que foi enviado | Porta de origem |
| --- | --- | --- |
| 16:19:30 | 42.540 caracteres, `stream:false` | `53358` |
| 16:20:15 | 68.040 caracteres, `stream:false` | `25791` |
| 16:21:45 | 113.679 caracteres, `stream:false` | `30430` |
| 18:11:24 | 42.540 caracteres, `stream:true` | `24760` |

Todas a partir de `10.100.2.30`, com `POST /api/generate` e o modelo `qwen3-ia`.

## Estado atual e impacto

- **Servidor:** normal desde 18:29:04 (`qwen3-ia` residente). Se alguém mandar um prompt grande de novo, a falha deve se repetir.
- **Nível `local` atual** (`ia-local.ps1`, modelo `qwen3.8:27b-64k`): **não testado** com prompt grande. Pode ou não ter o mesmo problema.
- **mcpollama:** as tools de arquivo (Fase 4) dependem do limite real de prompt, que hoje é desconhecido. Até a correção, o projeto usa um teto provisório **abaixo de 42.540 caracteres** (proposta PB5-1 no brief), e nenhum teste ao vivo acima disso sem ordem do owner.

## Erros de processo meus

1. A sonda **não consultava o `/api/ps` entre as requisições grandes** da execução 1. Se consultasse, saberia se o modelo caiu já na 1ª (16:20:15) ou só depois. Corrigido no reteste, que consultou antes e depois.
2. **Disparei 3 prompts grandes em sequência** na execução 1, sem parar na 1ª falha, e cada um pode ter derrubado o serviço de novo. O reteste já tinha a regra "parar na 1ª falha".

## Evidências

- **Saída bruta da sonda:** [`saida-b4.txt`](MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.evidencias/saida-b4.txt) (execução 1, 9.689 bytes) e [`saida-b4-stream.txt`](MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.evidencias/saida-b4-stream.txt) (execução 2, 1.383 bytes), copiadas do scratchpad da sessão. Antes da cópia, `scan-segredos-brief.ps1 -BriefDir` deu `known_hits=0` (com o token do `csl-ollama.env` entre os valores procurados) e 0 ocorrências de `Bearer` literal. A execução 3 (recarga) não foi gravada em arquivo; os números dela estão na linha do tempo. A 2ª via pós-mitigação está em [`saida-b4-2via.txt`](MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.evidencias/saida-b4-2via.txt), também varrida antes da cópia (`known_hits=0`).
- **Resumo versionável:** `referencias/Brief_Referencias/Servidor-Ollama-CSL/dados/validacao-empirica.md` §7.
- **Changelog detalhado:** plano do brief, entradas de 2026-10-23 16:25, 18:12 e 18:29.
