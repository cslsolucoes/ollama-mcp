# Teste de stress do mcpollama (local com mock, ao vivo moderado, servidor no limite)

- **Data:** 2026-10-26, 11:51 a 12:22 (hora local da estação)
- **Pedido do owner:** "faça um teste de stress"; escolheu os três tipos: (1) mcpollama local com mock,
  (2) ponta a ponta ao vivo moderado, (3) servidor no limite, com risco de reboot. Em caso de queda:
  "Parar, investigar logs e tentar 1 vez de novo".
- **Binário:** `mcpollama 0.5.0-19c3b8c` (o mesmo de `D:\MCP\mcpollama-mcp.exe`), com
  `--allow-root E:\MCPOllam`, `--timeout` padrão de 120 s e `--max-prompt-chars` padrão de 128000,
  exceto no teste no limite, que usou 160000.
- **Servidor:** `ia.cslsolucoes.com.br:8443`, `qwen3-ia` (MoE), 3 GPUs (`CUDA_VISIBLE_DEVICES=0,1,2`),
  `num_ctx` 65536, `OLLAMA_NUM_PARALLEL=1`.
- **Evidências:** `MCPOllam2026-10-26-122500-teste-stress.evidencias/`. São saídas brutas do driver
  e do diagnóstico por SSH. O token passa por scrub no driver, e a varredura de segredos deu
  `known_hits=0`. A única ocorrência genérica foi analisada: é a linha de log
  `password check failed for user (claiton)`, que não contém senha.

## Resultado em uma linha

**O mcpollama passou em tudo, sem nenhum defeito.** O servidor Ollama aguentou 5 prompts de 58.783
tokens (~90% do contexto) e 3 gerações de 7.000 tokens, sem reboot e sem Xid. Houve **um reboot
ordenado às 12:07, entre o teste sustentado e o teste no limite**, antes de qualquer prompt grande
ser enviado. O owner confirmou que foi a manutenção física dele (pendência 1, fechada).

**Atualização das 12:55 (§6):** a ativação da 4ª GPU com contexto de 131k **foi reprovada**: 5
reboots em 5 tentativas. A causa é o link PCIe da GPU `03:00.0` (porta `00:1b.4`), **não** a GPU
reativada. A configuração de 3 GPUs/64k foi restaurada por decisão do owner e validada (58.783
tokens em 21,9 s, sem reboot), mas continua usando a placa com o link ruim.

## 1. Stress local com mock (`src/mcpollama-mcp/stress_test.go`, build tag `stress`)

Comando: `go test -tags stress -count=1 -v -run Stress ./mcpollama-mcp` (dentro de `src/`).
O cenário foi escrito por um agente Sonnet e verificado por uma 2ª execução no loop principal:
exit 0, **8 PASS / 0 FAIL**, rajada total de 6,632 s a 75,4 chamadas/s.

| Teste | O que prova | Resultado |
| --- | --- | --- |
| `TestStress1Burst` | 500 `tools/call` concorrentes sobre um Ollama `httptest` | 500 respostas, 0 perdidas, 0 duplicadas; concorrência máxima no backend = **1** (a fila de GPU N=1 se mantém) |
| `TestStress2PingUnderLoad` | `ping` responde com 200 chamadas na fila | p99 de 1 ms (o loop de leitura não bloqueia) |
| `TestStress3MassCancel` | `notifications/cancelled` em massa | as 150 canceladas não respondem e não chegam ao Ollama; as 150 restantes respondem |
| `TestStress4NoLeak` | 3 ciclos de 500 chamadas | goroutines 6 → 6; heap +130 KiB |
| `TestStress5ExtremeInputs` | linhas no limite e JSON inválido | linha de 7,9 MB aceita; a de 8 MB vira erro e o servidor continua; 50 JSONs inválidos geram 50 erros `-32700` |
| `TestStress6MapReduce` | 5 `summarize_file` de 600 mil caracteres em paralelo | nenhum prompt passou do teto `--max-prompt-chars` |

## 2. Ao vivo moderado (driver `stressdrv`, cliente MCP stdio real → `mcpollama-mcp.exe` → Ollama)

Linha de base (11:51, `baseline.txt`): uptime desde 2026-10-23 18:11:55; 84 eventos
Xid/AER/NVRM desde o boot (anteriores ao teste); GPUs Gen1, GPU0 x16 e GPUs 1–3 x1; GPU 3 sem
uso; `NRestarts=0`.

### 2.1 Rajada (`burst.txt`): 20 `generate` simultâneos de ~500 tokens de saída, mais `ping` a cada 2 s

| Métrica | Valor |
| --- | --- |
| Chamadas | **20/20 OK**, 0 erros de protocolo |
| Tempo de parede | p50 69,7 s · p95 111,7 s · máx. 113,5 s |
| Maior espera na fila (`queue_ms`) | 111,7 s |
| Throughput agregado | 89,2 tok/s (a fila serializa; uma chamada isolada roda a ~97 tok/s) |
| `ping` durante a rajada | 56 pings, 0 erros, p50 0 ms, máx. 1 ms |

**Achado A1 (comportamento, não defeito):** o tempo de espera na fila **conta para o
`--timeout`** de 120 s. Numa rajada de ~22 chamadas ou mais desse porte, as últimas estouram o
timeout sem nunca chegar ao Ollama. O comportamento está correto: a mensagem de erro aponta o
`--timeout`. Mas o Claude Code raramente dispara mais que algumas chamadas ao mesmo tempo. Ver
pendência 3.

### 2.2 Sustentado (`sustained.txt`): 10 min de mistura contínua

A mistura repetia `generate` curto, `generate` com ~8 mil caracteres, `summarize_file` de
~27,4 mil tokens e `generate` curto.

| Métrica | Valor |
| --- | --- |
| Duração | 606 s |
| Chamadas | **148/148 OK**, 0 `isError`, 0 erros de protocolo |
| Tempo de parede | p50 3,3 s · p95 15,3 s · máx. 16,4 s |
| Maior espera na fila | 14,7 s |
| Tokens | ~1,07 milhão de entrada (`prompt_eval_count`) e ~16,6 mil de saída |
| `load_ms` máximo | 1 ms (o modelo ficou residente o tempo todo) |

O teste sustentado não mede `ping` (`ping_n=0` nesse modo). A latência de `ping` foi medida na
rajada e no mock.

## 3. Servidor no limite (`limit.txt` e `limit-retry.txt`)

Plano: 5 `generate` com prompt de 154.923 caracteres (58.783 tokens, ~90% de 65.536), seguidos de
3 `generate` com geração longa (7.000 tokens).

### 3.1 1ª tentativa (12:07:22): falha de rede, prompts não enviados

- As 6 primeiras chamadas falharam **na abertura da conexão TCP**
  (`dial tcp 10.100.2.2:8443: connectex ...`), ~21 s cada. **Nenhum prompt grande saiu da estação.**
- A 7ª chamada (12:09:28, geração longa) conectou quando o servidor voltou e estourou o timeout de
  120 s: carga a frio mais 7.000 tokens. O log do Ollama mostra que a geração foi **cancelada do
  lado do servidor** quando o cliente desistiu (`srv stop: cancel task` às 12:11:28,
  `n_tokens = 5707`). Isso prova, ao vivo, que o cancelamento por timeout chega ao Ollama e
  libera a GPU.
- O driver parou (`STOP`), como manda a escolha do owner.

### 3.2 Investigação: 2 conexões SSH root (`diag-out.txt`, `diag2-out.txt`)

| Evidência | Leitura |
| --- | --- |
| `last -x`: `shutdown system down 12:07 - 12:09`; o boot anterior termina em `12:07`, **não** em `crash` | **desligamento ordenado.** As quedas por PCIe de 23/10 aparecem como `crash` |
| `/var/log/messages`: a última linha antes do boot é de 12:05:46, a última chamada do teste sustentado respondendo `200` | nenhum Xid, AER ou NVRM antes da queda. O sustentado terminou sem erro |
| `/var/log/secure`: às 11:54:07, login `claiton` vindo de 10.100.2.30; às 11:54:18, senha do `sudo` errada; às 11:54:24, `sudo /bin/su -` em `pts/0` | **shell root interativa aberta por uma pessoa.** O Claude não digita senhas; os acessos do Claude usam chave root e não abrem tty |
| `dnf-automatic` não instalado; nenhum timer de reboot; o cron às 12:01 só rodou o `0anacron` | nenhuma causa automática encontrada |
| Topologia PCIe **mudou** entre os boots (ver §3.4) | indica mexida física **ou** enumeração diferente no boot |
| Uptime novo desde 12:09:26; `ollama` ativo às 12:09:36; `qwen3-ia` residente de novo | o servidor voltou sozinho |

**Conclusão: provável intervenção manual, não provada.** O reboot foi ordenado, havia uma shell
root interativa aberta desde 11:54 e a topologia PCIe mudou. Os logs do desligamento se
perderam: o `/var/log/messages` não registrou a sequência de parada e o journal **não está
persistindo entre boots** (achado A2). Por isso não dá para ver quem emitiu o comando.

### 3.3 Nova tentativa (12:13:09): aprovada

| idx | Tipo | `prompt_eval_count` | `eval_count` | Parede |
| --- | --- | --- | --- | --- |
| 0–4 | prompt de 154.923 caracteres | 58.783 cada | 12–17 | 21,2 s · 24,1 s · 24,1 s · 24,2 s · 23,6 s |
| 5–7 | geração longa | 64 | 7.000 cada (`fim=length`) | 88,0 s · 86,7 s · 86,6 s |

- **8/8 OK**, 0 erros, `queue_ms` 0 e `load_ms` 1. O prefill de 58,8 mil tokens leva ~21–24 s
  (~2.450–2.800 tok/s; em 16 mil tokens são ~5.000 tok/s). A geração longa roda a ~80 tok/s.
- **Achado A3:** uma geração de 7.000 tokens leva ~87 s, perto do `--timeout` de 120 s. Se ela
  esperar na fila atrás de outra chamada, ou pegar uma carga a frio (~24–36 s), passa do limite.
  Foi o que aconteceu com a 7ª chamada da 1ª tentativa.

Verificação pela 2ª via, por SSH às 12:19:46, depois da nova tentativa:
- **sem reboot:** uptime ainda desde 12:09:26; `NRestarts=0`;
- **Xid/NVRM = 0** e **AER não-corrigível ou fatal = 0**;
- **6 AER corrigíveis** (`Physical Layer, Receiver ID`) na root port `00:1b.4`, que hoje atende a
  GPU `03:00.0` (índice 1, em uso). Na linha de base, a ponte com problema era `09:07.0` (42 AER
  no histórico do `/var/log/messages`);
- GPUs a 37–43 °C; GPU 3 com 1 MiB, ou seja, segue fora de uso.

### 3.4 Topologia PCIe antes e depois do reboot de 12:07

| | Antes (linha de base de 11:51 e incidente de 23/10) | Depois (12:19) |
| --- | --- | --- |
| Barramentos das GPUs (índices 0–3) | `01`, `04`, `05`, `07` | `01` (`00:01.0`), `03` (`00:1b.4`), `05` (`00:1c.2`), `06` (`00:1c.3`) |
| GPU com defeito | `07:00.0`, atrás de `00:1c.5`, Gen1 x1 | ? A ponte `00:1c.5` e o barramento `09` não aparecem; a NIC Realtek está em `07:00.0` |
| Exclusão no Ollama | `CUDA_VISIBLE_DEVICES=0,1,2` (por **índice**) | igual: o índice 3 é agora `06:00.0`, UUID `GPU-d7166024-…` |

**Achado A4 (risco):** a exclusão da GPU com defeito é feita **pelo índice**, e os barramentos
mudaram. Os registros anteriores não guardaram o **UUID** da placa com defeito, então **não dá
para provar** que o índice 3 de hoje é a mesma placa física. Se outra placa assumiu o índice 3, a
GPU com defeito estaria **em uso**. A nova tentativa sob carga máxima, porém, não teve Xid nem AER
não-corrigível. Ver pendência 2.

~~Observação: o `ollama.service` tem duas linhas `CUDA_VISIBLE_DEVICES`.~~ **Corrigido às 12:26:** a
linha `0,1` está só em `10-performance.conf.bak-20261023-002653`, que o systemd **não** lê. Não há
duplicata ativa; a pendência 5 foi um erro meu de leitura.

## 4. Achados

| # | Achado | Tipo | Onde |
| --- | --- | --- | --- |
| A1 | O tempo na fila conta para o `--timeout`; uma rajada de ~22 ou mais chamadas longas estoura nas últimas | comportamento do mcpollama (correto, mas a documentar) | `server.go` (fila N=1) |
| A2 | O journal não persiste entre boots: o boot `ce285434…` (visível às 11:51) sumiu depois do reboot, embora `/var/log/journal` exista | servidor (observabilidade) | `journald` |
| A3 | Uma geração de 7.000 tokens (~87 s) fica perto do `--timeout` de 120 s; somada a fila ou carga a frio, passa | comportamento (limite configurável) | `--timeout` |
| A4 | GPU com defeito excluída por índice, e a topologia PCIe mudou; não há UUID registrado para confirmar | servidor (risco físico) | `ollama.service` drop-in |

**Defeitos no código do mcpollama: nenhum.**

## 5. Pendências (numeradas)

1. ~~**Owner:** confirmar se reiniciou ou mexeu no servidor às 12:07.~~ **Fechada:** o owner confirmou
   que o reboot foi a manutenção física dele na GPU (AskUserQuestion, 12:26: "Sim, fui eu").
2. **Owner, servidor:** identificar fisicamente a GPU com defeito e passar a excluí-la **pelo UUID**
   (`CUDA_VISIBLE_DEVICES=GPU-…`) em vez do índice. Registrar o UUID no `CONEXAO-IA.md`. Hoje o
   índice 3 é `06:00.0` / `GPU-d7166024-…`.
3. **Decisão do owner (opcional):** contar o `--timeout` só a partir da saída da fila, ou manter
   como está. Hoje o tempo total de espera do cliente fica limitado, e isso também é um valor.
4. **Owner, servidor:** tornar o journal persistente de fato (`Storage=persistent` no
   `journald.conf`, depois `systemctl restart systemd-journald` e `journalctl --flush`) e conferir
   com `journalctl --list-boots` depois do próximo reboot.
5. ~~**Owner, servidor:** remover a linha duplicada `CUDA_VISIBLE_DEVICES=0,1`.~~ **Não procede:** a
   linha está só no `.bak` (ver §3.4).

## 6. Ativação da 4ª GPU (pedido do owner às 12:24: "ativei a outra placa, pode analisar se ficou ok")

### 6.1 Análise com a configuração ainda em 3 GPUs (12:25, `gpu4-out.txt`)

- **GPU 3 no hardware:** reconhecida (`06:00.0`, UUID `GPU-d7166024-…`), ligada **direto** na root
  port `00:1c.3`. A ponte `09:07.0`, com 42 AER no histórico, saiu da topologia. A porta `00:1c.3`
  tinha 0 AER, mas a placa estava ociosa.
- **No Ollama:** a GPU 3 continuava excluída (`CUDA_VISIBLE_DEVICES=0,1,2`; `found 3 CUDA devices`).
  Com 3 GPUs, a memória estava no limite (`cannot meet free memory targets ... 737 MiB less`).
- **Contexto fixado em 3 lugares:** drop-in (`OLLAMA_CONTEXT_LENGTH`), **Modelfile** do `qwen3-ia`
  (`PARAMETER num_ctx 65536`, que vale sobre a variável de ambiente) e `ollama-warm`
  (`"num_ctx":65536`).
- **Decisão do owner (AskUserQuestion):** "4 GPUs e volta a 131072".

### 6.2 Aplicação (12:27, `apply-gpu4-out.txt`)

- **Backup** em `/root/gpu4-backup-20261026/`: drop-in, `ollama-warm` e `qwen3-ia.Modelfile`.
- **Mudanças:**
  - `CUDA_VISIBLE_DEVICES` = os 4 UUIDs;
  - `OLLAMA_CONTEXT_LENGTH=131072`;
  - Modelfile recriado com `num_ctx 131072` (`ollama create`);
  - `ollama-warm` com `num_ctx` 131072;
  - `daemon-reload` e restart do serviço.
- **Resultado do carregamento:**
  - `found 4 CUDA devices` (31.358 MiB); previsão de 24.425 MiB para 30.700 MiB livres;
  - 49/49 camadas, `n_ctx = 131072`, **100% GPU**, GPU 3 com 6.151 MiB;
  - carga em 22 s; Xid 0.

### 6.3 Validação no limite com 4 GPUs e 131k: **reprovada, 5 reboots**

| Hora | Execução | Resultado |
| --- | --- | --- |
| 12:28:52 | `limit-4gpu-128k.txt`, 5 × 307.143 caracteres (~115 mil tokens) + 3 gerações longas, `OLLAMA_TIMEOUT=300s` | 4 chamadas com a conexão cortada (60 / 90 / 105 / 195 s), depois o host inacessível. **Reboots às 12:29, 12:31, 12:32 e 12:37** |
| 12:41:08 | `limit-4gpu-128k-retry.txt` (o owner pediu "teste novamente"), driver já corrigido | a 1ª chamada teve a conexão cortada em 75 s; **reboot às 12:42**; o driver parou |

**Erro meu de processo:** a 1ª execução causou 4 reboots em vez de 1. A regex que detecta queda no
driver só reconhecia mensagens em inglês, e os erros do mcpollama são em português ("o servidor
encerrou a conexão", "não foi possível falar com"). **Solução:** no modo limite, o driver passa a
parar em **qualquer** erro. Foi o que funcionou às 12:41, quando parou depois de 1 queda.

**Causa, pelo journal (agora persistente; `diag5-out.txt`):**

| Boot que caiu | RxErr na root port `00:1b.4` antes do corte | demais portas |
| --- | --- | --- |
| 12:09–12:28:28 | sem registro: o corte veio na 1ª chamada, antes de o journal gravar em disco | só as 3 linhas da partida |
| 12:29–12:30:06 | **51** (`Physical Layer, RxErr`), até o último segundo | 3 (partida) |
| 12:31–12:31:37 | 7; **caiu durante `load_tensors`** (carga do modelo, sem prompt) | 3 (partida) |
| 12:32–12:33:21 | **279**, até o último segundo | 3 (partida) |
| 12:37–12:40:44 | sem registro: o journal grava a cada 5 min (`SyncIntervalSec` padrão) e o último registro é anterior à chamada de 12:41 | — |

- **A culpada é a GPU `03:00.0`** (índice 1, UUID `GPU-13d97d49-7bdf-79b2-7994-28c4dc0e2ac8`),
  ligada à root port `00:1b.4` (Intel `8086:06ac`). **Não é a GPU 3** que o owner ativou: a porta
  dela, `00:1c.3`, não teve nenhum erro em operação.
- Com 4 GPUs e 131k passam mais dados pelo PCIe (carga das camadas e prefill), e o link de
  `00:1b.4` cede. Na configuração de 3 GPUs e 64k ele já dava erros corrigíveis (3 às 12:10–12:11).

### 6.4 Restauração (decisão do owner: "Restaurar o backup exato"; 12:52, `restore-out.txt`)

- **Arquivos:** drop-in, `ollama-warm` e Modelfile voltaram ao backup. A versão de 4 GPUs ficou
  salva como `10-performance.conf.4gpu-131k-falhou`.
- **Config:** `CUDA_VISIBLE_DEVICES=0,1,2` (índice) e contexto 65536 nos 3 lugares.
- **Carga:** `found 3 CUDA devices`, 49/49 camadas, `n_ctx = 65536`, 100% GPU, carga em 20 s.
- **Validação** (`restore-validate.txt`): 1 prompt de 58.783 tokens → **OK em 21,9 s**.
- **2ª via** (SSH 12:53, `post2-out.txt`): sem reboot (uptime desde 12:42:12), `NRestarts=0`, Xid 0.
  **`00:1b.4` = 8 erros corrigíveis** depois de uma única carga e um único prompt; `00:1c.2` = 0 e
  `00:1c.3` = 0.

### 6.5 Achados e pendências desta etapa

- **A5 (risco físico, principal):** o link PCIe da GPU `03:00.0`, na porta `00:1b.4`, está ruim.
  - Com 4 GPUs e 131k, derruba a máquina sempre (5/5).
  - Com 3 GPUs e 64k, aguenta, mas acumula RxErr (8 em 1 chamada).
  - **A configuração restaurada ainda usa essa placa**, por decisão do owner.
- **A6:** o `ollama-warm` não roda na partida: `nvidia-powercap.service: Found ordering cycle ...
  Job ollama-warm.service/start deleted`. O modelo só carrega na 1ª chamada.
- **A7:** o journal agora persiste entre boots (5 boots listados), mas grava em disco a cada 5 min.
  Os segundos antes de um corte seco se perdem.
- **Pendências:**
  - **P7-6 (owner, físico):** trocar o riser ou o slot da GPU `03:00.0` (`GPU-13d97d49-…`, porta
    `00:1b.4`) e só então revalidar 4 GPUs/131k. O teste de referência é
    `stressdrv -mode limit -n 5 -prompt-tokens 115000`, que agora para na 1ª queda.
  - **P7-7 (decisão do owner):** enquanto o reparo não vem, excluir a `03:00.0` e usar `01`, `05` e
    `06` por UUID com 64k, ou manter a configuração restaurada.
  - **P7-8 (servidor):** quebrar o ciclo de ordenação `nvidia-powercap` ↔ `ollama-warm`.
  - **P7-9 (servidor, opcional):** `SyncIntervalSec=10s` no journald, para guardar o fim de um corte seco.
  - **P7-10:** registrar no `CONEXAO-IA.md` (servidor e cópia local) o mapa UUID ↔ barramento ↔ porta
    e este episódio. Exige autorização do owner para mexer nos documentos do servidor.
  - **P7-2 atualizada:** a exclusão por índice continua, e o índice 1 é a placa ruim; quando houver
    exclusão, fazer por UUID.

## 7. Nova repetição com 4 GPUs e 64k (pedido do owner às 13:03: "repita o teste")

- **Estado encontrado** (`check3-out.txt`, `check4-out.txt`):
  - o owner editou o drop-in às 12:57:17 para `CUDA_VISIBLE_DEVICES=0,1,2,3` com
    `OLLAMA_CONTEXT_LENGTH=65536` e reiniciou o `ollama`, **sem `systemctl daemon-reload`**
    (`NeedDaemonReload=yes`). O serviço seguia com `0,1,2` e `found 3 CUDA devices`;
  - sem reboot nem mudança física desde 12:42; a porta `00:1b.4` estava com 13 AER corrigíveis.
- **Aplicação** (`apply4x64-out.txt`):
  - `daemon-reload` e restart → `CUDA_VISIBLE_DEVICES=0,1,2,3`;
  - previsão de 22.558 MiB para 30.700 MiB livres; 49/49 camadas; `n_ctx = 65536`; **100% GPU**;
  - carga em 22 s, com 5,6–6,3 GB por GPU;
  - nenhum AER novo durante a carga.
- **Teste no limite** (`limit-4gpu-64k.txt`; driver que para na 1ª queda): **8/8 OK**.
  - 5 prompts de 58.783 tokens em **18,3 / 19,8 / 19,9 / 20,0 / 19,9 s**, mais rápido que com 3 GPUs
    (21–24 s).
  - 3 gerações de 7.000 tokens em 88,7 / 87,1 / 87,1 s.
- **2ª via** (SSH 13:10, `post3-out.txt`):
  - **sem reboot** (uptime desde 12:42:12), `NRestarts=0`;
  - **Xid 0**, nenhuma mensagem `Uncorrected` ou `Fatal`;
  - `00:1b.4` de 13 para **16 corrigíveis**: 3 erros espaçados (13:05:49, 13:07:38, 13:08:49);
    `00:1c.2` = 0; `00:1c.3` (GPU reativada) = **0**;
  - GPUs entre 34 e 45 °C.
- **Conclusão:** **4 GPUs com 64k ficou OK** e está em uso. O que derrubava a máquina era a
  combinação **4 GPUs + 131k**. O link `00:1b.4` (GPU `03:00.0`) continua acumulando erros
  corrigíveis sob carga, cerca de 1 por chamada pesada, e é o ponto a vigiar. A **P7-6 (reparo físico)**
  continua valendo antes de qualquer nova tentativa com 131k.
- **Observações:**
  - `CUDA_VISIBLE_DEVICES=0,1,2,3` inclui todas as placas, então a mudança de índice deixa de ser
    risco enquanto as 4 estiverem em uso. A P7-2 fica suspensa;
  - o `ollama-warm` (`num_ctx 65536`) e o Modelfile (`num_ctx 65536`) estão coerentes com o drop-in.

## 8. Limite de potência e carga máxima contínua (pedidos do owner às 13:19 e 13:23)

### 8.1 Potência de 170 W para 200 W ("aumente 170w para 200w e repita os testes")

- **Estado encontrado** (`pl-read-out.txt`):
  - `nvidia-powercap.service` aplica `-pl 170` nas 4 GPUs. O owner tinha mudado de 240 W para
    170 W às 12:57, com o comentário "pos Xid 79 ... na placa PCI 01:00 sob carga a 240W";
  - faixa da placa: mínimo 100 W, padrão 220 W, máximo 240 W.
- **Aplicação** (`pl-apply-out.txt`): backup `.170w`, `-pl 200`, `daemon-reload` e restart da unit.
  Resultado: 200 W nas 4 GPUs.
- **Teste** (`w200-limit.txt`, `power-200w.csv`):
  - o 1º prompt de 58.783 tokens começou a carregar às 13:20:26;
  - no único ponto de carga gravado (13:20:30), as GPUs estavam em **178 + 159 + 156 + 151 ≈ 644 W**;
  - em seguida, **corte seco e reboot às 13:21** (`diag7-out.txt`);
  - o último registro do journal é de 13:20:20, sem Xid e sem AER. O driver parou na 1ª queda.
- **Inferência (não provada):** um corte instantâneo, sem erro registrado, no salto de potência
  indica a **fonte (PSU) desarmando**. Com 170 W, o mesmo teste tinha passado 8/8.
- **Decisão do owner (AskUserQuestion):** "Voltar a 170 W e repetir".

### 8.2 Erro meu na reversão: unit esvaziada (vista como "mascarada")

- **O que aconteceu:**
  - o backup `.170w`, criado às 13:20, uns 10 s antes do corte, ficou com **0 bytes** depois do
    reboot: o XFS ainda não tinha gravado o conteúdo em disco;
  - a reversão copiou esse arquivo vazio por cima da unit, e o systemd passou a tratá-la como
    `masked` (`pl-state-out.txt`);
  - eu atribuí o mascaramento ao owner antes de investigar; **estava errado**.
- **Solução** (`pl-fix-out.txt`):
  - unit recriada a partir da cópia íntegra de 200 W, com `-pl 170` e o comentário de volta;
  - `sync`, `daemon-reload` e restart → `loaded`/`enabled`, **170 W nas 4 GPUs**;
  - backup `.170w` regravado (670 bytes);
  - a busca por arquivos zerados nas últimas 4 h encontrou só 2 arquivos de sistema que normalmente
    ficam vazios (`tuned`, `foomatic`). Os arquivos editados hoje estão íntegros.
- **Lição:** depois de gravar no servidor, rodar `sync`; antes de restaurar um backup, conferir o
  tamanho dele.

### 8.3 Carga máxima contínua por 10 min a 170 W ("repita o teste de stress por 10 min forçando ao máximo")

- **Cadeia interrompida:** a cadeia limite → rajada → sustentado a 170 W tinha passado 1 prompt de
  58,8 mil tokens, com carga a frio de 32 s. Foi interrompida para dar lugar a este teste.
- **Modo `max`** criado no driver: 2 workers mandam prompts **únicos** (nonce) de ~58 mil tokens sem
  intervalo, com `num_predict` 256. Sempre há uma chamada na fila e o prefill fica contínuo. Para na
  1ª falha.
- **Resultado** (`w170-max10m.txt`): **22 chamadas OK** de 13:27 a 13:31, com ~20 s de prefill e
  ~20 s de fila cada. A 23ª teve a conexão cortada. **Reboot às 13:32**, depois de **~5 min** de
  carga máxima (`diag8-out.txt`).
- **Potência e temperatura** (`power-170w.csv`, 60 amostras de 13:27:04 a 13:31:59):

| GPU | W máx. | W média | °C máx. |
| --- | --- | --- | --- |
| 0 (`01:00`) | 167,2 | 146,9 | **88** |
| 1 (`03:00`, porta `00:1b.4`) | 155,0 | 131,6 | 70 |
| 2 (`05:00`) | 158,1 | 135,8 | 83 |
| 3 (`06:00`) | 151,7 | 133,8 | 70 |
| **Soma das GPUs** | **623,5** | **548,1** | — |

- **Kernel antes do corte** (`diag8-out.txt`):
  - **42 linhas de AER corrigível na porta `00:1b.4`**, em frequência crescente. Inclui
    `Data Link Layer` e `Multiple Correctable`, não só `Physical Layer`;
  - 0 Xid; 0 erros não-corrigíveis;
  - o journal do Ollama vai até 13:31:48, com o prefill a 85% e ~4.000 tok/s; depois, corte seco.
- **Leitura:**
  - **4 GPUs com 64k a 170 W aguenta uso real**, com intervalos entre chamadas (8/8 no limite, 148
    chamadas no sustentado de 3 GPUs);
  - **não aguenta carga máxima contínua**: caiu em ~5 min com ~550 W médios só nas GPUs, sem
    contar CPU e placa-mãe;
  - o padrão é "cai instantâneo a ~644 W, cai em minutos a ~550 W, aguenta com intervalos". Isso é
    compatível com uma **fonte no limite**, por sobrecorrente ou sobretemperatura;
  - o link `00:1b.4` piorando sob carga é um 2º suspeito. Os dois sobem juntos com a carga e **não
    dá para separar** sem um teste que isole um deles;
  - a GPU 0 chegou a **88 °C**, perto do limite de throttling da RTX 3070 (~93 °C).

### 8.4 Pendências desta etapa

- **P8-1 (owner):** informar a potência e o modelo da fonte, para confirmar ou descartar a hipótese
  de PSU.
- **P8-2 (decisão):** teste que isole a causa:
  - (a) 4 GPUs a ~130 W (~450 W) por 10 min: menos potência, com a `00:1b.4` incluída;
  - (b) 3 GPUs sem a `03:00.0` a 170 W por 10 min: sem o link ruim, ~450 W.
  - Se (a) passar e (b) também, a potência é o fator principal. Se (a) cair e (b) passar, a causa é
    o link `00:1b.4`.
- **P8-3:** a refrigeração da GPU 0 (88 °C) merece atenção.
- **P7-6 continua:** reparo físico do link `00:1b.4`.
- **Estado do servidor às 13:33:**
  - no ar desde 13:32:25, com 170 W, 4 GPUs e 64k;
  - **sem modelo carregado**, porque o `ollama-warm` não roda na partida (P7-8);
  - em uso normal, com intervalos, o histórico de hoje é estável; em carga máxima contínua, cai.