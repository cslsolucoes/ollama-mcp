# Manual de operação do `mcpollama-gateway` (Fase 9)

Para quem opera o servidor de IA da CSL (`10.100.2.2`, `lx-ai01`) por SSH e usa o `/monitor` no navegador.
Escrito a partir do código (`src/cmd/mcpollama-gateway`, `src/gateway/*`), da pasta `deploy/gateway/`, do plano
e da revisão adversarial da Fase 9. Onde o plano e o código divergem, vale o código (ver "Notas de conferência", no fim).

> **Nota N1 — resolvida em 2026-10-30 10:50.** A conferência deste manual achou que os scripts gravavam o arquivo da senha
> como `root:mcpgw 0640`, e que o gateway recusa esse modo. Os dois scripts passaram a gravar **`mcpgw:mcpgw 0600`**.
> Provado no servidor com o binário instalado, rodando como `mcpgw`: com `0640` a partida falha (código 2, "use 0600");
> com `0600` o gateway sobe e o `/monitor` responde 200.

## Índice

1. [O que é e por que existe](#1-o-que-é-e-por-que-existe)
2. [Instalação e reinstalação](#2-instalação-e-reinstalação)
3. [Referência de flags](#3-referência-de-flags)
4. [Fila](#4-fila)
5. [Políticas (9.6a)](#5-políticas-96a)
6. [Monitor `/monitor`](#6-monitor-monitor)
7. [API do monitor](#7-api-do-monitor)
8. [Banco SQLite](#8-banco-sqlite)
9. [Operação do dia a dia](#9-operação-do-dia-a-dia)
10. [Diagnóstico](#10-diagnóstico)
11. [Voltar atrás (rollback)](#11-voltar-atrás-rollback)
12. [Segurança](#12-segurança)
13. [Limites e futuro](#13-limites-e-futuro)
14. [Referências](#14-referências)
15. [Notas de conferência](#15-notas-de-conferência)
16. [Pool de backends e secundário (Fase 10.4)](#16-pool-de-backends-e-secundário-fase-104)

---

## 1. O que é e por que existe

O Ollama processa **1 pedido por vez** com o `qwen3-ia` (= `qwen3.5:9b`) e tem uma fila interna invisível
(`OLLAMA_MAX_QUEUE`). Com vários agentes ao mesmo tempo, ninguém via quem esperava nem quanto. O gateway é uma **fila
visível, justa e auditável** posta na frente do Ollama, com uma página (`/monitor`) para ver e mexer nela.

```
cliente (mcpollama, ia-local/OpenClaude, curl)
   │  HTTPS :8443, Authorization: Bearer <TOKEN>   (LAN 10.100.2.0/24 ou Cloudflare Tunnel)
   ▼
nginx :8443   TLS, Bearer, 600 pedidos/min por IP (rajada 100), 80 simultâneos por IP,
   │          403 em /api/pull|push|create|delete|copy|blobs, /monitor só da LAN
   ▼
mcpollama-gateway 127.0.0.1:11500   fila em memória + proxy + SQLite + /monitor
   │
   ▼
Ollama 127.0.0.1:11434   (só loopback desde a etapa 2: ninguém contorna a fila)
```

O que **não** muda para os clientes: a URL (`https://ia.cslsolucoes.com.br:8443`), o Bearer, as rotas do Ollama
(`/api/generate`, `/api/chat`, `/api/embed`, `/v1/...`).

O que muda:

| Cliente | Mudança |
| --- | --- |
| **mcpollama** (MCP) | Manda `X-Mcpollama-Client` (rótulo, `--client-label`, padrão `<máquina>/<usuário>`, ou `<máquina>/<pid>` se não houver usuário) e `X-Mcpollama-Tool`. Lê `X-Queue-Ms` e mostra `queue_ms` na linha `[uso]`. Em **503 com `Retry-After`** repete **uma vez** (só se sobrar tempo no `--timeout`). Em **409** (administrador removeu/cancelou) não repete. Não manda prioridade nem `think` (usa o padrão do gateway). |
| **mcpollama 0.9.2+** (MCP) | Também manda `X-Mcpollama-Station` (`computer=..;user=..;domain=..;mac=..`, **declarado pela estação, não autenticado**). O gateway valida, grava os quatro campos no log de jobs (7 dias) e mostra no **Detalhes** do job; a coluna Cliente segue sendo o rótulo. O cabeçalho não é repassado ao backend. Desligar: `--station-info=false`. |
| **ia-local** (OpenClaude, `/v1`) | Sem rótulo: aparece como `anonimo` (a justiça por IP continua valendo). Por padrão o gateway **desliga o raciocínio** no `/v1`; para ligar, use o modelo `qwen3-ia-think` (seção 5). |
| **Qualquer um** | Respostas ganham `X-Request-Id`, `X-Queue-Ms` (quanto esperou) e `X-Queue-Position` (posição na entrada). O gateway remove `keep_alive`, `num_ctx` e opções de GPU do corpo e decide o raciocínio (seção 5). Pedido fora da lista de modelos dá 403. |

---

## 2. Instalação e reinstalação

O servidor é `10.100.2.2`; o alias SSH usado pelo instalador é `lx-ai01-root` (em `~/.ssh/config`, `root@10.100.2.2`).
Muitas conexões SSH seguidas ao servidor podem acionar o fail2ban: o instalador usa poucas.

### 2.1 Caminho principal: `Instalar-Gateway.ps1` (roda neste PC)

Pré-requisitos no PC: `go`, `git`, `ssh.exe` e `scp.exe` no PATH e o alias SSH funcionando
(`ssh lx-ai01-root true`).

```powershell
cd E:\MCPOllam\deploy\gateway
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1                  # instalação completa
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Atualizar   # só troca o binário
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Senha       # (re)define a senha do admin
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Preparar    # só a etapa 1
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Reverter -Backup /root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS
```

Opções: `-Servidor` (padrão `lx-ai01-root`), `-Versao` (padrão `0.9.0-<commit curto>`), `-SemBuild` (usa o
`D:\MCP\mcpollama-gateway` que já existe), `-Backup` (só para `Reverter`).

| Modo | O que faz | Muda o tráfego? |
| --- | --- | --- |
| `Completo` (padrão) | Gera o binário, monta o pacote, envia, etapa 1, pede a senha se ainda não existe, etapa 2 | **Sim** (etapa 2) |
| `Preparar` | Gera, envia e roda só a etapa 1 (usuário, pastas, binário) | Não |
| `Atualizar` | Gera, envia, etapa 1 (troca o binário) e `systemctl restart mcpollama-gateway` com conferência | Não (reinicia só o gateway) |
| `Senha` | Abre `ssh -t` e roda `/root/mcpgw-definir-senha.sh` (você digita sem eco) | Não |
| `Reverter` | Roda `rollback.sh` guardado na pasta de backup da etapa 2 | **Sim** (volta ao estado antigo) |

O modo `Completo`, na ordem:

1. gera o binário Linux em `D:\MCP\mcpollama-gateway` (`GOOS=linux CGO_ENABLED=0`, `-ldflags "-X main.Version=<versão>"`, ~18 MB);
2. monta o pacote numa pasta temporária com finais de linha **LF** e o manifesto `SHA256SUMS` (hash de todos os arquivos); calcula o hash do próprio `SHA256SUMS`;
3. recria `/root/deploy96` no servidor (**root, 0700**), envia por `scp` e ajusta dono/modo; copia o `mcpgw-definir-senha.sh` para `/root/`;
4. roda `stage1.sh <hash do SHA256SUMS>`;
5. se `/etc/mcpollama-gateway/admin` não existe, roda `ssh -t ... mcpgw-definir-senha.sh`: **você digita a senha (12+ caracteres) no seu terminal, sem eco**;
6. roda `stage2.sh <hash do SHA256SUMS>`; se algo falhar depois de mexer no nginx/Ollama, o rollback é automático.

Nenhum token nem senha passa pelo script PowerShell.

**Saídas esperadas** (resumo; o que não aparece aqui é rotina):

| Passo | Saída esperada |
| --- | --- |
| build | sem erro; arquivo de ~18 MB |
| pacote | `SHA256SUMS = <64 hexadecimais>` |
| etapa 1 | `boot: ...`, `mcpollama-gateway version <versão>`, `backup em /root/srv-backup-20261028/fase96-AAAAMMDD-HHMMSS`, `ETAPA1_OK` |
| senha | `ok: /etc/mcpollama-gateway/admin gravado (NN bytes, so o hash). Usuario: admin` |
| etapa 2 | `backups em .../fase96-ativacao-...`, `active`, `gateway /api/version: 200`, `gateway /monitor: 200`, `resposta: 'pong' eval N thinking 0`, três linhas `x-request-id`, `x-queue-ms`, `x-queue-position`, `sem token: 401`, `pull: 403`, `/monitor: 200`, `monitor/state chaves: [...]`, `ollama ps`, `ETAPA2_OK (backups e rollback em <B>: bash <B>/rollback.sh <B>)` |

**Anote a pasta `<B>`** (`fase96-ativacao-...`): é o argumento do rollback.

**A etapa 2 reinicia o Ollama** (para ele escutar só em 127.0.0.1) e depois o `ollama-warm`. O modelo recarrega
(~16–22 s a frio) e quem estiver gerando é cortado. Instale com a fila vazia.

**Reinstalar (`Completo` de novo):** é seguro e idempotente, mas lembre:
- a etapa 2 faz um **novo backup do estado atual** (que já é o do gateway). Para voltar ao estado **anterior ao gateway**, use o backup `fase96-ativacao-*` **mais antigo**, não o último;
- ela reinstala a unit e o `ollama.conf` do repositório, **sobrescrevendo** edições feitas direto no servidor (ver seção 9);
- reinicia o Ollama de novo.

### 2.2 Alternativa: à mão (`scp` + etapas 1 e 2)

Use se o instalador não puder rodar. O resultado é o mesmo; a diferença é que você monta o pacote e confere os hashes.

**a) No PC (Git Bash), gerar o binário e o pacote com LF e manifesto:**

```powershell
# PowerShell, dentro de E:\MCPOllam\src
$env:GOOS='linux'; $env:CGO_ENABLED='0'
go build -ldflags "-X main.Version=<versão>" -o D:\MCP\mcpollama-gateway ./cmd/mcpollama-gateway
Remove-Item Env:GOOS,Env:CGO_ENABLED
```

```bash
# Git Bash
cd /e/MCPOllam/deploy/gateway
P=$(mktemp -d); ARQ="mcpollama-gateway.service ollama.conf stage1.sh stage2.sh rollback.sh mcpgw-definir-senha.sh"
for f in $ARQ; do sed 's/\r$//' "$f" > "$P/$f"; done      # LF ANTES de calcular os hashes
cp /d/MCP/mcpollama-gateway "$P/mcpollama-gateway"
( cd "$P" && sha256sum mcpollama-gateway $ARQ > SHA256SUMS && sha256sum SHA256SUMS | cut -d' ' -f1 )
# anote o hash impresso: é o <HASH_SUMS> abaixo
```

**b) Enviar para a pasta só do root e ajustar dono e modo:**

```bash
ssh lx-ai01-root 'rm -rf /root/deploy96 && install -d -m 0700 -o root -g root /root/deploy96'
scp "$P"/* lx-ai01-root:/root/deploy96/            # no scp do Windows, use o caminho do Windows da pasta $P
ssh lx-ai01-root 'cd /root/deploy96 && chown root:root * && chmod 0600 * && chmod 0700 *.sh mcpollama-gateway && install -m 0700 -o root -g root mcpgw-definir-senha.sh /root/mcpgw-definir-senha.sh'
```

**c) No servidor, etapa 1, senha, etapa 2:**

```bash
bash /root/deploy96/stage1.sh <HASH_SUMS>
bash /root/mcpgw-definir-senha.sh          # você digita a senha, sem eco (mínimo 12 caracteres)
bash /root/deploy96/stage2.sh <HASH_SUMS>
```

As duas etapas conferem: root, pasta `0700`, dono dos arquivos, o hash do `SHA256SUMS` contra o argumento e
**todos** os arquivos contra o `SHA256SUMS`. A etapa 1 confere o binário de novo depois de copiado, antes do `mv`.
Se qualquer conferência falhar, nada é instalado. CRLF alteraria os hashes: por isso a conversão para LF vem antes.

### 2.3 Como conferir que deu certo

No servidor:

```bash
systemctl is-active mcpollama-gateway                 # active
systemctl is-enabled mcpollama-gateway                # enabled
/usr/local/bin/mcpollama-gateway --version            # mcpollama-gateway version <versão>
ss -tlnH | grep -E '11434|11500'                      # os dois em 127.0.0.1 (nada em 0.0.0.0:11434)
curl -s http://127.0.0.1:11500/healthz                # ok
journalctl -u mcpollama-gateway -n 15 --no-pager
```

Linha esperada no `journalctl` (a versão e os números mudam):

```
mcpollama-gateway <versão> em 127.0.0.1:11500 -> Ollama 127.0.0.1:11434 (geracao=1 embed=2 fila=64 por-ip=16) boot=<uuid>
```

Se aparecer `sem --admin-file: o monitor é somente leitura`, o arquivo da senha não foi informado.

Pelo caminho real (nginx + Bearer; o token **não** vai na linha de comando, vai pelo stdin):

```bash
printf 'Authorization: Bearer %s\n' "$(sed -n 2p /root/API-Key)" | curl -sk https://127.0.0.1:8443/api/generate -H @- \
  -H 'X-Mcpollama-Client: teste/manual' -D - -d '{"model":"qwen3-ia","prompt":"Responda apenas: pong","stream":false}'
```

Esperado: `HTTP/2 200`, os headers `x-request-id`, `x-queue-ms`, `x-queue-position`, e `"response":"pong"`.
Depois, no navegador da LAN: `https://ia.cslsolucoes.com.br:8443/monitor` deve mostrar a linha de status
(`versão ... · boot ... · ativo há ...`) e o pedido de teste em "Concluídos recentes" (cliente `teste/manual`).

Prova por 2ª via de que o histórico foi gravado (seção 8): `sqlite3 -readonly /var/lib/mcpollama-gateway/gateway.db "select client_label,status,http_status from requests order by enqueued_at desc limit 3;"`.

### 2.4 Atualizar só o binário

**Pelo instalador (recomendado):**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Atualizar -Versao <versão>
```

Ele gera o binário, envia o pacote conferido, roda a etapa 1 (que **guarda uma cópia do binário antigo** em
`/root/srv-backup-20261028/fase96-AAAAMMDD-HHMMSS/mcpollama-gateway`, troca por `mv` atômico e mostra a versão) e então roda
`systemctl restart mcpollama-gateway` e confere: `active`, `--version`, `gateway /api/version: 200`, `gateway /monitor: 200`.

Cuidados:
- **O restart corta** quem está em execução e quem espera na fila (o gateway dá até 30 s de tolerância para terminar o que está em curso). Olhe o `/monitor` antes: o ideal é fila e execução vazias. Os clientes recebem erro de conexão; o mcpollama não repete esse caso.
- O modo `Atualizar` **não mexe** na unit nem no nginx. Se a mudança precisa de flag nova ou de unit nova, use `Completo` (`-SemBuild` se o binário já está pronto) ou a seção 9.
- O banco migra sozinho no start (`ALTER TABLE ... ADD COLUMN` só se faltar).

**À mão** (binário já enviado e conferido pela etapa 1, ou copiado por você):

```bash
systemctl stop mcpollama-gateway                                   # SIGTERM, até 30 s de tolerância
cp -p /usr/local/bin/mcpollama-gateway /root/srv-backup-20261028/mcpollama-gateway.$(date +%Y%m%d-%H%M%S)
bash /root/deploy96/stage1.sh <HASH_SUMS>                          # confere e instala (a pasta tem de ter o pacote novo)
systemctl start mcpollama-gateway
systemctl is-active mcpollama-gateway && /usr/local/bin/mcpollama-gateway --version
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:11500/monitor      # 200
```

Voltar ao binário anterior: `systemctl stop mcpollama-gateway; install -m 0755 <cópia> /usr/local/bin/mcpollama-gateway; systemctl start mcpollama-gateway`.

---

## 3. Referência de flags

Cada flag tem uma variável de ambiente `MCPGW_*`. **Ordem: flag > variável de ambiente > padrão.** Booleanos de
ambiente: vazio, `0`, `false`, `no` e `off` são falso; qualquer outro valor é verdadeiro. Durações aceitam `300ms`, `10m`,
`2160h`. Fonte: `src/cmd/mcpollama-gateway/config.go` (38 flags). **A unit de produção** é `mcpollama-gateway.service`.

> Na unit, as flags vão em `ExecStart`. Como a flag vence a variável de ambiente, definir `Environment=MCPGW_X=...`
> **não** muda uma flag que já está no `ExecStart`.

| Flag | Env | Padrão | O que faz | Unit de produção |
| --- | --- | --- | --- | --- |
| `--listen ADDR` | `MCPGW_LISTEN` | `127.0.0.1:11500` | Endereço de escuta. Fora do loopback a partida é recusada (confia no nginx para o IP real) | `127.0.0.1:11500` |
| `--ollama HOST:PORT\|URL` | `MCPGW_OLLAMA` | `127.0.0.1:11434` | O Ollama de destino (só `http://`) | `127.0.0.1:11434` |
| `--db FILE` | `MCPGW_DB` | `/var/lib/mcpollama-gateway/gateway.db` | Arquivo SQLite, criado `0600` | `/var/lib/mcpollama-gateway/gateway.db` |
| `--gen-slots N` | `MCPGW_GEN_SLOTS` | `2` | Gerações simultâneas (`/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions`) | **`1`** (o Ollama roda `qwen3.5:9b` com 1 slot) |
| `--embed-slots N` | `MCPGW_EMBED_SLOTS` | `2` | Embeddings simultâneos | `2` |
| `--queue-max N` | `MCPGW_QUEUE_MAX` | `64` | Pedidos esperando, todos os clientes e as duas faixas juntas. Cheio: 503 | **`128`** (desde 2026-10-30; antes, o padrão 64) |
| `--queue-per-client N` | `MCPGW_QUEUE_PER_CLIENT` | `16` | Esperando por IP + rótulo | **`64`** (antes 16) |
| `--queue-per-ip N` | `MCPGW_QUEUE_PER_IP` | = `--queue-per-client` | Esperando por IP, todos os rótulos juntos | **`64`** (antes 16) |
| `--labels-per-ip N` | `MCPGW_LABELS_PER_IP` | `16` | Rótulos distintos por IP; acima disso viram `<ip>/outros` (variar o rótulo não multiplica a cota) | padrão (16) |
| `--max-wait D` | `MCPGW_MAX_WAIT` | `10m` | Maior espera na fila; depois, **504** | padrão (10m) |
| `--run-timeout D` | `MCPGW_RUN_TIMEOUT` | = `--max-wait` | Maior tempo de execução de um pedido já iniciado; depois o slot é liberado e o cliente recebe 504 (ou uma linha final de erro no stream) | padrão (10m) |
| `--age-after D` | `MCPGW_AGE_AFTER` | `5m` | Um pedido `baixa` que espera isso passa a competir como `normal` | padrão (5m) |
| `--start-stagger D` | `MCPGW_START_STAGGER` | `300ms` | Intervalo mínimo entre o início de duas gerações (suaviza o pico de energia da fonte). `0` desliga. Só vale para a faixa de geração | `300ms` |
| `--priority-ips LIST` | `MCPGW_PRIORITY_IPS` | vazio (ninguém) | IPs/CIDRs (separados por `,` ou `;`) que podem pedir prioridade `alta`. IPv4 mapeado em IPv6 é normalizado | `10.100.2.0/24` |
| `--allowed-models LIST` | `MCPGW_ALLOWED_MODELS` | vazio (qualquer) | Modelos aceitos (`;` ou `,`). Confere o nome **base** (sem `-think`). `:latest` é ignorado na comparação | `"qwen3-ia;nomic-embed-text"` |
| `--default-think BOOL` | `MCPGW_DEFAULT_THINK` | `false` | Raciocínio quando o cliente não diz: API nativa recebe `think:BOOL`; `/v1` recebe `reasoning_effort:"none"` se for `false` | padrão (`false`) |
| `--think-suffix S` | `MCPGW_THINK_SUFFIX` | `-think` | O nome `<modelo>S` (ex.: `qwen3-ia-think`) é reescrito para `<modelo>` com raciocínio ligado. `none` desliga. Sem espaço nem aspas | padrão (`-think`) |
| `--allow-client-keepalive` | `MCPGW_ALLOW_CLIENT_KEEPALIVE` | `false` | Deixa passar o `keep_alive` do cliente (por padrão é removido: um `0` descarregaria o modelo para todos) | padrão (removido) |
| `--allow-client-num-ctx` | `MCPGW_ALLOW_CLIENT_NUM_CTX` | `false` | Deixa passar `options.num_ctx` (por padrão é removido: outro valor recarrega o modelo) | padrão (removido) |
| `--strip-options LIST` | `MCPGW_STRIP_OPTIONS` | `num_gpu,main_gpu,low_vram,use_mmap,use_mlock,numa` | Opções removidas de `options` (`,` ou `;`; `none` = nenhuma). O `num_ctx` sai pela flag acima, não por esta lista | padrão |
| `--max-body-bytes N` | `MCPGW_MAX_BODY_BYTES` | `33554432` (32 MiB) | Teto do corpo da requisição (413 acima). O nginx tem 64 MiB | padrão |
| `--admin-file FILE` | `MCPGW_ADMIN_FILE` | vazio | Linha `usuario:pbkdf2-sha256$...` (modo `0600`). Sem ele o monitor é **somente leitura**. Lido **só na partida** | `/etc/mcpollama-gateway/admin` |
| `--gpu-interval D` | `MCPGW_GPU_INTERVAL` | `1s` | Período da coleta ao vivo: `nvidia-smi` **e** CPU/RAM (o mesmo intervalo vale para os dois coletores). `0` desliga os dois. É também o período do envio periódico do SSE. Nunca há duas coletas ao mesmo tempo: se uma demora mais que o intervalo, o tick atrasado é descartado | `1s` (padrão) |
| `--sample-store-interval D` | `MCPGW_SAMPLE_STORE_INTERVAL` | `5s` | De quanto em quanto tempo uma amostra de GPU e uma de CPU/RAM é **gravada** em `gpu_samples`/`cpu_samples`. A página ao vivo recebe toda amostra (1 s); o banco só 1 a cada 5 s (4 linhas/s de GPU e 1/s de CPU seriam gravação inútil). Tem de ser **>= `--gpu-interval`** (senão a partida falha, código 2) | padrão (5s) |
| `--proc-root DIR` | `MCPGW_PROC_ROOT` | `/proc` | Raiz do procfs lida pelo coletor de CPU/RAM (`stat`, `loadavg`, `meminfo`, `cpuinfo`, `<pid>/comm` e `<pid>/status`). Só se muda em teste, com uma árvore falsa | padrão |
| `--sys-root DIR` | `MCPGW_SYS_ROOT` | `/sys` | Raiz do sysfs lida pelo coletor de CPU/RAM (`class/hwmon`, `class/thermal`, `class/powercap`, `devices/system/cpu`). Só se muda em teste | padrão |
| `--nvidia-smi PATH` | `MCPGW_NVIDIA_SMI` | `nvidia-smi` | Binário do `nvidia-smi` | padrão |
| `--aer-path FILE` | `MCPGW_AER_PATH` | `/sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable` | Contador AER corrigível da ponte `00:1b.4`; `-` desliga | padrão |
| `--retention-requests D` | `MCPGW_RETENTION_REQUESTS` | `2160h` (90 dias) | Quanto tempo ficam os pedidos no banco | padrão |
| `--retention-gpu D` | `MCPGW_RETENTION_GPU` | `168h` (7 dias) | Quanto tempo ficam as amostras de GPU **e de CPU/RAM** | padrão |
| `--max-request-rows N` | `MCPGW_MAX_REQUEST_ROWS` | `3000000` | Teto de linhas de `requests` (mínimo 1000); as mais antigas saem primeiro, de hora em hora | padrão |
| `--log-content BOOL` | `MCPGW_LOG_CONTENT` | `true` | **Log de jobs com o prompt e a resposta COMPLETOS** num banco à parte (`--content-db`). Decisão do owner de 2026-10-30. `false`: o log de jobs, o extrato de GPU e a recuperação continuam; só o texto deixa de ser gravado (e o `content.db` não é criado) | padrão (`true`) |
| `--content-db FILE` | `MCPGW_CONTENT_DB` | `content.db` na pasta de `--db` (`/var/lib/mcpollama-gateway/content.db`) | Arquivo do texto dos jobs, criado `0600` pelo `mcpgw` (um arquivo que já existia volta a `0600` na partida; o `-wal`/`-shm` copiam o modo). Tem de ser outro arquivo que `--db`: o texto **nunca** vai para o banco de metadados. Se ele **não abrir** (corrompido, ou o caminho é uma pasta), o gateway **sobe sem gravar o texto**, grava o evento `conteudo_indisponivel` e deixa o arquivo como está, para inspeção | padrão |
| `--content-max-bytes N` | `MCPGW_CONTENT_MAX_BYTES` | `2147483648` (2 GiB) | Teto TOTAL do texto guardado (soma de prompts e respostas). Passou: apaga os jobs mais antigos até 90% do teto e grava o evento `teto_conteudo`. Mínimo 1 MiB | padrão |
| `--content-max-job-bytes N` | `MCPGW_CONTENT_MAX_JOB_BYTES` | `16777216` (16 MiB) | Teto do prompt e da resposta de UM job, **cada um**. Passou: corta (em fronteira de caractere) e marca `truncado=sim`. Mínimo 1024, e não acima do teto total | padrão |
| `--retention-content D` | `MCPGW_RETENTION_CONTENT` | `168h` (7 dias) | Quanto tempo fica o texto dos jobs (os metadados seguem com `--retention-requests`, 90 dias) | padrão |
| `--retention-job-gpu D` | `MCPGW_RETENTION_JOB_GPU` | `168h` (7 dias) | Quanto tempo fica o extrato de GPU segundo a segundo de cada job (`job_gpu_samples`) | padrão |
| `--monitor-keep-done D` | `MCPGW_MONITOR_KEEP_DONE` | `5m` | Quanto tempo um pedido concluído continua **visível** no `/monitor` ("Concluídos nos últimos 5 min"). Só exibição: não ocupa vaga nem entra nos limites. `0` esconde a lista | padrão |
| `--backends FILE` | `MCPGW_BACKENDS` | vazio | **Pool de backends (10.4).** JSON dos Ollamas **secundários**, modo **`0600`** (senão a partida é recusada, código 2, como o `--admin-file`). Formato e regras na seção 16. **Sem ele o pool é um só backend, `gpu-local` = `--ollama`, e o gateway se comporta como antes** | não usado (sem secundário) |
| `--overload-wait D` | `MCPGW_OVERLOAD_WAIT` | `1m` | **S1**: o primário está sobrecarregado quando a espera estimada de um pedido novo passa disso (EWMA da duração de uma geração × (esperando à frente + executando) ÷ slots, sem contar o próprio pedido). Só age com secundário | padrão |
| `--overload-queue N` | `MCPGW_OVERLOAD_QUEUE` | `0` (= 75% de `--queue-max`) | **S2**: sobrecarga quando MAIS de N pedidos de geração esperam. Não pode passar de `--queue-max` | padrão |
| `--overload-hold D` | `MCPGW_OVERLOAD_HOLD` | `30s` | Histerese: o primário só retoma o trabalho depois de a carga ficar abaixo de **50%** dos dois limites, e o primário saudável, por esse tempo | padrão |
| `--health-interval D` | `MCPGW_HEALTH_INTERVAL` | `5s` | Checagem ativa de cada backend: `/api/version`, `/api/ps` (modelo residente, saiu de 100% GPU?) e `/api/tags` (digests). Também é a leitura do quadro "Modelos" | padrão |
| `--breaker-failures N` | `MCPGW_BREAKER_FAILURES` | `3` | Falhas seguidas (ativas ou passivas) que abrem o disjuntor de um backend | padrão |
| `--breaker-open D` | `MCPGW_BREAKER_OPEN` | `30s` | Quanto tempo o disjuntor fica aberto; depois fica meio-aberto e deixa passar **um** pedido de prova | padrão |
| `--non-loopback-listen` | `MCPGW_NON_LOOPBACK_LISTEN` | `false` | Permite `--listen` fora do loopback (não recomendado) | não usado |
| `--debug` | `MCPGW_DEBUG` | `false` | Uma linha de log por pedido (`req <id> <faixa> <rota> status=... class=... wait_ms=...`), nunca com texto | não usado |
| `--version` | (sem env) | — | Imprime a versão e sai | — |

**Validações da partida** (erro = sair com código 2): slots, `--queue-*` e `--labels-per-ip` ≥ 1; durações > 0
(`--start-stagger` e `--gpu-interval` podem ser 0); `--sample-store-interval` > 0 e >= `--gpu-interval`; `--max-body-bytes` > 0; retenções > 0 e `--max-request-rows` ≥ 1000;
`--listen` no loopback; `--db` não vazio; com `--log-content`: `--content-db` diferente de `--db`, `--content-max-bytes` ≥ 1 MiB, `--content-max-job-bytes` ≥ 1024 e ≤ o teto total; `--retention-content` e `--retention-job-gpu` > 0; `--monitor-keep-done` ≥ 0; `--admin-file` com permissão `0600` (ver Nota N1). pool: `--overload-wait`, `--overload-hold`, `--health-interval`, `--breaker-open` > 0, `--breaker-failures` ≥ 1, `--overload-queue` ≥ 0 e ≤ `--queue-max`; `--backends` legível, `0600` e válido (ver seção 16).

**Subcomando:** `mcpollama-gateway hash-password [-user NOME] [-iterations N]` lê a senha do **stdin** (primeira linha,
mínimo 8 caracteres no código; o script de senha exige 12) e imprime `usuario:pbkdf2-sha256$600000$<sal>$<hash>`.

---

## 4. Fila

Tudo vive **em memória**; o SQLite só guarda o histórico. Reiniciar o gateway **esvazia a fila**.

**Faixas** (independentes, cada uma com seus slots):

| Faixa | Rotas | Slots (produção) |
| --- | --- | --- |
| `geracao` | `/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions` | 1 |
| `embed` | `/api/embed`, `/api/embeddings`, `/v1/embeddings` | 2 |

Rotas **diretas** (sem fila, sem registro no histórico): `/api/tags`, `/api/ps`, `/api/show`, `/api/version`, `/v1/models`
(GET, POST ou HEAD). Rotas administrativas (`/api/pull`, `push`, `create`, `delete`, `copy`, `blobs`) → 403. Qualquer
outra → 404 `endpoint desconhecido`.

**Prioridade:** `alta`, `normal` (padrão), `baixa`. O cliente pede com o header `X-Mcpollama-Priority: alta|normal|baixa`.
`alta` só vale se o IP do cliente está em `--priority-ips` (produção: toda a LAN `10.100.2.0/24`); de outro IP o pedido é
tratado como `normal`, **sem erro**. O mcpollama não manda esse header: ele serve para `curl` e scripts.

**Quem passa na frente** (dentro da faixa), em ordem:
1. maior prioridade efetiva;
2. fixados no topo pelo administrador (o mais recente primeiro);
3. **justiça por IP**: o IP servido há mais tempo (rodízio entre IPs; empate pelo pedido mais antigo);
4. dentro do IP, justiça por **rótulo** (`X-Mcpollama-Client`), e FIFO dentro do mesmo rótulo;
5. empurrados ao fim pelo administrador.

O IP não é falsificável (vem do nginx); o rótulo é livre, por isso a justiça ancora no IP. IPv6 é agrupado por `/64`.
Rótulo válido: `^[A-Za-z0-9._:/@+-]{1,64}$`; fora disso vira `invalido`; vazio vira `anonimo`.

**Envelhecimento:** um `baixa` que espera `--age-after` (5 min) passa a competir como `normal`.

**Limites e o que o cliente recebe:**

| Situação | Resposta | Observação |
| --- | --- | --- |
| Fila total cheia (`--queue-max`; padrão 64, **128 em produção**) | **503** `fila do servidor cheia (fila cheia); tente em 5 s` + `Retry-After: 5` | pedido nunca rodou; registrado `rejeitado` |
| IP com 64 esperando (`--queue-per-ip`) ou IP+rótulo com 64 (`--queue-per-client`); padrão do código 16, **64 em produção** | **503** `... (limite de pedidos em espera deste cliente) ...` + `Retry-After: 5` | idem |
| Esperou mais que `--max-wait` (10 min) | **504** `tempo máximo de espera na fila esgotado` | registrado `expirado` |
| Rodando há mais que `--run-timeout` (10 min) | antes dos headers: **504** `tempo máximo de execução no servidor esgotado`; no meio do stream: linha final `{"error":"tempo máximo de execução no servidor esgotado"}` | registrado `erro` / `run_timeout`; o slot volta |
| Administrador **removeu** da fila | **409** `removido da fila pelo administrador` | registrado `removido` |
| Administrador **cancelou** em execução | antes dos headers: **409** `cancelado pelo administrador`; no meio do stream: linha final `{"error":"cancelado pelo administrador"}` | registrado `cancelado_admin` |
| Cliente desistiu (fechou a conexão) | nada (ninguém para ler); sai da fila | registrado `cancelado` / `client_gone`, HTTP 499 no registro |
| Ollama fora ou conexão recusada | **502** `Ollama indisponível` (sem `Retry-After`) | `upstream_unreachable` |
| Um **secundário** (pool, 10.4) respondeu com redirecionamento (3xx) | **502** `Ollama indisponível`; o `Location` e o `Set-Cookie` do secundário não chegam ao cliente | `upstream_redirect` (revisão 10.5) |

**Escalonamento de partida** (`--start-stagger`, 300 ms): entre o início de duas gerações passam no mínimo 300 ms. Com 1
slot de geração o efeito é pequeno; vale para quando houver mais slots (os prefills não começam no mesmo milissegundo, o
que atenua os picos de energia que derrubam a fonte de 850 W).

**Camadas de limite** (do cliente para dentro): nginx 600 pedidos/min por IP (rajada 100) e **80 simultâneos** por IP
(excedeu → **429 do nginx**, página HTML, não do gateway) → gateway (limites acima, **503 JSON**) → Ollama. Na prática o
teto de 80 do nginx fica acima do de 64 esperando por IP do gateway, que é atingido primeiro (prova de 2026-10-30: 30 pedidos
simultâneos de uma estação deram 30× HTTP 200; antes, com 16/30, deram 17× 200 e 13× 503).

---

## 5. Políticas (9.6a)

Rodam **antes de enfileirar**, em `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`,
`/v1/chat/completions`, `/v1/completions` e `/v1/embeddings`.

**1. Opções removidas** (em silêncio): `keep_alive` de topo, `options.num_ctx` e `options.num_gpu`, `main_gpu`, `low_vram`,
`use_mmap`, `use_mlock`, `numa`. Motivo: o Ollama mantém o modelo residente com o contexto dele (`num_ctx` 65536); qualquer
outro valor recarrega o modelo (~20 s) ou o descarrega para todos. Religar com `--allow-client-keepalive`,
`--allow-client-num-ctx` ou editando `--strip-options`. As chaves são casadas sem diferenciar maiúsculas; **duas grafias da
mesma chave** (`model` e `Model`) dão **400**. JSON inválido ou `model` que não é texto também dá **400**, com mensagem
fixa (sem eco do corpo) e sem entrar na fila. Se nada muda, o corpo segue **byte a byte**.

**2. Allowlist de modelos:** modelo fora de `--allowed-models` → **403** `modelo "X" não permitido neste servidor`.
O nome conferido é o **base**: `qwen3-ia-think` passa porque `qwen3-ia` passa; `qwen3-ia-think-think` dá 403.

**3. Raciocínio.** O `qwen3-ia` raciocina por padrão (lento); o gateway **desliga** quando o cliente não diz.
Precedência: **corpo explícito > header `X-Mcpollama-Think` > nome `<modelo>-think` > `--default-think`**.

| Rota | Corpo explícito | Header `on` / nome `-think` | Header `off` ou nada (padrão) |
| --- | --- | --- | --- |
| Nativa (`/api/generate`, `/api/chat`) | `think` (`true`, `false`, `"high"`) é respeitado | injeta `think:true` | injeta `think:false` |
| `/v1` | `reasoning_effort` ou `reasoning` é respeitado (`"none"` = desligado). `think:false` no corpo do `/v1` **não conta** (o Ollama o ignora lá) | não injeta nada: o modelo raciocina | injeta `"reasoning_effort":"none"` |
| Embeddings | não se aplica | — | — |

O header só vale `on` ou `off` (qualquer caixa); outro valor é ignorado. O header é removido antes de ir ao Ollama.
O nome `qwen3-ia-think` **não existe no Ollama** (não aparece em `/api/tags` nem em `/v1/models`); um cliente que valide o
nome contra a lista não o acharia. O monitor mostra a coluna **Raciocínio**: `ligado`, `desligado`, `padrão do modelo` ou `-`.

**Exemplos** (`<TOKEN>` é o Bearer; use `--cacert` com a CA da Cloudflare Origin, ver seção 6):

```bash
URL=https://ia.cslsolucoes.com.br:8443
CA=~/.openclaude/certs/cloudflare-origin-ca.pem

# Padrão: raciocínio desligado (rápido), prioridade normal
curl --cacert $CA $URL/api/generate -H "Authorization: Bearer <TOKEN>" -H 'X-Mcpollama-Client: meu-script' \
  -d '{"model":"qwen3-ia","prompt":"Responda apenas: pong","stream":false}'

# Ligar o raciocínio (3 jeitos equivalentes na API nativa)
curl --cacert $CA $URL/api/generate -H "Authorization: Bearer <TOKEN>" -d '{"model":"qwen3-ia","prompt":"...","stream":false,"think":true}'
curl --cacert $CA $URL/api/generate -H "Authorization: Bearer <TOKEN>" -H 'X-Mcpollama-Think: on' -d '{"model":"qwen3-ia","prompt":"...","stream":false}'
curl --cacert $CA $URL/api/generate -H "Authorization: Bearer <TOKEN>" -d '{"model":"qwen3-ia-think","prompt":"...","stream":false}'

# /v1 (dialeto OpenAI): sem nada, o gateway injeta reasoning_effort:"none". Para raciocinar:
curl --cacert $CA $URL/v1/chat/completions -H "Authorization: Bearer <TOKEN>" -H 'Content-Type: application/json' \
  -d '{"model":"qwen3-ia-think","messages":[{"role":"user","content":"..."}]}'

# Prioridade alta (só vale se o IP do cliente está em --priority-ips; senão vira normal, sem erro)
curl --cacert $CA $URL/api/chat -H "Authorization: Bearer <TOKEN>" -H 'X-Mcpollama-Priority: alta' -H 'X-Mcpollama-Client: urgente' \
  -d '{"model":"qwen3-ia","messages":[{"role":"user","content":"..."}],"stream":false}'

# Estas opções são removidas em silêncio (não recarregam o modelo):
curl --cacert $CA $URL/api/generate -H "Authorization: Bearer <TOKEN>" \
  -d '{"model":"qwen3-ia","prompt":"oi","stream":false,"keep_alive":0,"options":{"num_ctx":131072,"num_gpu":1,"temperature":0}}'
```

**Como o `ia-local` liga o raciocínio:** o `%USERPROFILE%\.openclaude\csl-ollama.env` tem `OPENAI_MODEL=qwen3-ia`
(raciocínio desligado pelo gateway). Para ligar, troque para `OPENAI_MODEL=qwen3-ia-think` (e volte para `qwen3-ia` depois:
raciocinar a cada chamada deixa o `ia-local` muito mais lento). Não coloque o token em nenhum comando de exemplo.

---

## 6. Monitor `/monitor`

**Endereço:** `https://ia.cslsolucoes.com.br:8443/monitor` (use o **nome**, com a porta 8443; por IP o navegador reclama do
nome do certificado). **Só da LAN `10.100.2.0/24`** (e do próprio servidor): de fora, pelo Cloudflare, o nginx dá **403**.
Ver o monitor **não exige login**; as ações exigem.

**Cloudflare e túnel (conferido em 2026-10-30):** a internet chega pelo túnel `cloudflared`, que roda no próprio servidor,
para `ia.cslsolucoes.com.br` e para `…:8443`. Esses pedidos entram no nginx **como `::1`/`127.0.0.1`** e trazem o
`CF-Connecting-IP`. Por isso, o `ollama.conf`:
- recusa (403) no `/monitor` **todo pedido com `CF-Connecting-IP`**. Antes disso, o loopback liberado deixava o monitor aberto
  à internet; corrigido no commit `2a16888`;
- **não** trata o loopback como LAN: para quem vem pelo túnel, o IP real do cliente (`CF-Connecting-IP`) vai para a fila,
  os limites e a justiça. Da LAN, esse cabeçalho é ignorado, para não permitir IP forjado.

A regra de NAT do `MK-ESCRITORIO-02` que leva WAN:8443 ao servidor vale **só para a lista `PERMITIDOS`** desde 2026-10-30;
o Cloudflare não depende dela.

Conferência rápida, feita no servidor (sai pela internet e volta pelo Cloudflare):

```bash
H=ia.cslsolucoes.com.br; IP=$(dig +short @1.1.1.1 $H A | tail -1)
curl -s -o /dev/null -w '%{http_code}\n' --resolve $H:443:$IP https://$H/monitor       # 403 (bloqueado de fora)
curl -s -o /dev/null -w '%{http_code}\n' --resolve $H:443:$IP https://$H/api/version   # 401 (API exige token)
```

**Certificado (Cloudflare Origin CA).** O Windows não confia nessa CA, então o navegador avisa. Importe a CA uma vez em
cada estação de administração (PowerShell **como administrador**; o arquivo é o que o `ia-local` já usa):

```powershell
certutil -addstore -f Root "$env:USERPROFILE\.openclaude\certs\cloudflare-origin-ca.pem"
```

Para só o seu usuário (sem administrador): `certutil -user -addstore Root "<arquivo>"`. Edge e Chrome usam a loja do
Windows; o Firefox tem loja própria (importe lá ou ative `security.enterprise_roots.enabled`). Reinicie o navegador.

**O que a página mostra** (atualiza ao vivo por SSE, a cada `--gpu-interval` (1 s) e de imediato a cada mudança da fila, com *polling* de 1 s se o SSE falhar):

| Quadro | Conteúdo |
| --- | --- |
| Cabeçalho | `versão · boot (8 primeiros caracteres) · ativo há · atualizado`; à direita, login |
| Cartões | Em execução; Na fila; Concluídos hoje (status `ok` desde 00:00 do servidor); Erros hoje (status `erro`); Espera p50 e p95 (15 min); **Tokens/s** (tokens gerados nos últimos 15 min ÷ 900 s, é média e não instantâneo); Pedidos/min (15 min) |
| Em execução | Faixa, id (8 caracteres), cliente (passe o mouse: IP + rótulo), tool, modelo, raciocínio, prioridade concedida, rodando há, esperou; botão **Cancelar** (logado) |
| Fila de espera | Posição (a ordem em que vão começar, simulada), faixa, id, cliente, tool, modelo, raciocínio, prioridade atual (já com envelhecimento), esperando há; botões **▲ ▼ ✕** (logado) |
| **Backends** (10.4) | Uma linha por backend do pool (o primário `gpu-local` e os secundários): **id**, **tipo** (`gpu`, `cpu`, `remoto`), **estado** (`ok`, `sobrecarregado` (primário), `disjuntor aberto`, `fora`, `degradado` (modelo fora de 100% GPU), `drenado`), **disjuntor** (fechado, aberto, meio-aberto), **slots usados** e livres, **fila** (esperando que esse backend poderia pegar; no primário são todos), **p50** da duração das gerações, **modelo · digest · onde está**, **último erro** (passe o mouse) e, logado, o botão **Drenar / Retomar**. Sem `--backends` há uma linha só |
| GPUs | Por GPU: potência em uso **e o limite aplicado** ("71 W / 100 W"; é o `enforced.power.limit`, o power cap que o `nvidia-powercap.service` aplica), uma barra fina com a fração do limite usada (cor de aviso acima de **90%** do limite; o teto da placa, `power.max_limit`, aparece ao passar o mouse na barra), memória (MiB), uso (%), temperatura; linha `AER 00:1b.4 (corrigíveis): N`; "registros descartados: N" se o buffer do banco encheu. Se o driver não informa o limite, mostra só os W |
| CPU / RAM | Logo abaixo das GPUs, no mesmo visual de cartões. **CPU**: uso total (%, pelo delta de `/proc/stat` entre duas amostras; a primeira amostra após a partida mostra `-`), frequência média (GHz) e temperatura do pacote (°C, hwmon `coretemp` "Package id 0", ou a zona térmica `x86_pkg_temp`). **Carga** 1/5/15 min. **RAM**: usada/total em GB, % e "Ollama: X GB" (soma do `VmRSS` dos processos de nome `ollama`). **Swap**: usado/total. Abaixo, **uma barra por thread** (T0..T11), agrupadas por núcleo físico ("Núcleo 0: T0 T6 · 45 °C"; o núcleo vem de `topology/core_id`, a temperatura de `coretemp` "Core N"). Rodapé: "potência da CPU: indisponível (só root)" quando o RAPL não é legível (no servidor, `/sys/class/powercap/intel-rapl*/energy_uj` é `root 0400`; o gateway roda como `mcpgw` e **não** abre essa permissão). Qualquer leitura que falhe vira `-` e um texto curto de erro no rodapé; o gateway nunca para por isso |
| Modelos | Duas tabelas. **Carregados (VRAM)**, do `/api/ps` do Ollama: nome, VRAM em GB, GPU/CPU (`100% GPU`, ou o split `70%/30% CPU/GPU` calculado de `size_vram`/`size`), contexto, quando expira e se é permitido. **Disponíveis (instalados)**, do `/api/tags`: nome, tamanho, parâmetros, quantização, família e "permitido" sim/não (o mesmo critério do `--allowed-models`; sem a lista, tudo é permitido). A legenda lista os permitidos e explica que os nomes com o sufixo de `--think-suffix` são **virtuais**: `qwen3-ia-think` liga o raciocínio de `qwen3-ia`, e o gateway tira o sufixo. O gateway consulta o próprio Ollama (`--ollama`, loopback, direto, fora da fila) **a cada 5 s**, uma consulta de cada vez; a página só lê o cache, então abrir 30 painéis não gera nenhuma consulta a mais. Se o Ollama não responde, a lista fica vazia e o rodapé mostra um erro curto (`ps: ...` / `tags: ...`). Cada carga ou descarga de modelo vira um evento `modelo_carregado` / `modelo_descarregado` (a primeira leitura depois da partida não gera evento, e um erro de consulta não conta como descarga) |
| Última hora | Três gráficos: pedidos na fila, vazão (pedidos/min), espera média (ms). **Ficam em memória**: reiniciar o gateway zera |
| Concluídos recentes | Últimos 50 (qualquer status: `ok`, `erro`, `cancelado`, `rejeitado`, `expirado`...), com HTTP, espera, execução, tokens (prompt / gerados) e classe do erro |
| Auditoria | Últimas 20 ações do administrador |
| Eventos | Últimos 10: `gateway_inicio`, `gateway_config`, `gateway_parada`, `boot_novo`, `registros_descartados`, `teto_pedidos`, `modelo_carregado`, `modelo_descarregado`, `teto_conteudo`, `teto_conteudo_falhou`, `jobs_interrompidos`, `conteudo_indisponivel` |

**Login do administrador:** usuário `admin` (um só) e a senha definida na instalação (seção 2.1, modo `Senha`).
Sessão: cookie `HttpOnly; Secure; SameSite=Strict` de **8 horas**, no máximo 8 sessões ao mesmo tempo (a mais antiga cai).
Sem `--admin-file` a página mostra "somente leitura". Se a página pede login e nada acontece, veja a mensagem em cima
do quadro de cartões.

**Ações e o efeito no cliente:**

| Ação | Onde | O que faz | O que o cliente vê |
| --- | --- | --- | --- |
| **▲ Promover** | pedido esperando | Sobe um nível (`baixa`→`normal`→`alta`); entra no **fim** do novo nível (não fura quem já esperava lá). Já em `alta`: vai ao **topo** da faixa | nada; só começa antes |
| **▼ Despromover** | pedido esperando | Desce um nível; já em `baixa`: vai ao **fim** da faixa | nada; começa depois |
| **✕ Remover** | pedido esperando | Tira da fila (pede confirmação e motivo opcional) | **409** `removido da fila pelo administrador`; o mcpollama não repete |
| **Cancelar** | pedido em execução | Fecha a conexão com o Ollama e libera o slot na hora (confirmação e motivo) | **409** `cancelado pelo administrador` (ou, se já transmitia, linha final de erro no stream) |
| **Drenar / Retomar** (10.4) | backend, quadro Backends | O backend **não recebe pedidos novos**; o que está rodando termina. Serve para manutenção. Drenar o primário manda o trabalho ao secundário elegível (se não houver, espera). **Retomar** desfaz. Pede confirmação e motivo opcional; vai para a auditoria (`drain` / `undrain`, `request_id` = id do backend) | nada; o pedido espera ou roda em outro backend |

Ação sobre um pedido que já saiu da fila (começou, terminou ou o cliente desistiu): a página mostra
`não está mais na fila` / `não está mais em execução` (**409**). Toda ação válida grava na **auditoria**: data/hora,
usuário, IP, ação, id do pedido, estado antes e depois e o motivo (até 200 caracteres). A auditoria **nunca expira**.

**Limites do login e do painel:**
- **5 falhas por IP em 15 min** → bloqueio de 15 min (**429** + `Retry-After`); cada novo bloqueio dobra (30 min, 1 h... até 24 h); um dia sem atividade zera. Acertar a senha limpa o IP. Não há bloqueio global: falhas de outros IPs não trancam o administrador.
- No máximo **2 verificações de senha** ao mesmo tempo (PBKDF2 pesa); se ocupado por 10 s: 503.
- **SSE: 4 painéis por IP e 32 no total**; acima disso: 503 `muitos painéis abertos`. Fechar abas antigas resolve.
- Os GET do monitor são abertos a quem alcança a LAN; eles mostram IPs, rótulos, modelos, contadores e o extrato de GPU de cada job, **nunca** prompts ou respostas: o texto só sai para o **administrador logado** (ver a subseção abaixo).

**Concluídos nos últimos 5 min (Fase 10.L).** Desde a `0.12.3` é uma **seção própria**, depois de "Fila de espera" (ordem do painel: Em execução, Fila de espera, Concluídos,
Backends, GPUs, CPU/RAM, Modelos, Última hora, abas); a tabela mantém cada pedido que terminou por
`--monitor-keep-done` (5 min), para os que entram e saem rápido demais para serem vistos. As linhas aparecem
**esmaecidas**, com o status final em cor (verde "concluído"; vermelho erro, cancelado, removido, expirado ou interrompido), a hora do
fim, quanto rodou, o pico de W e de clock, um contador "some em mm:ss" e o botão **Detalhes**. É só exibição: o pedido
concluído não ocupa vaga, não entra nos limites nem nos cartões "Em execução" e "Na fila". A lista vem do log de jobs
(`recent_done` no `/state` e no SSE).

**Aba "Log em tempo real" (Fase 10.L).** Terceira aba, ao lado de "Concluídos recentes" e "Eventos". Atualiza **pelo SSE**, sem
recarregar: o job aparece ao entrar na fila (`na fila`), vira `em execução` e termina no estado final; os mais novos ficam
no topo, no máximo 200. Colunas: hora de entrada, status (com cor), cliente, tool, modelo, raciocínio, esperou, rodou, tokens
(prompt / gerados), "Resposta" (sim, não ou parcial), pico de W e de clock do job e o botão **Detalhes**.
- **Detalhes** abre o painel do job: metadados completos, o **extrato das GPUs segundo a segundo** (tabela e gráficos SVG de
  W e de clock por GPU, mais uma linha de CPU) e, **só para o administrador logado**, o prompt e a resposta completos. Sem
  sessão, o painel diz "entre como admin para ver o conteúdo". O painel de um job em execução se atualiza enquanto ele roda
  (a resposta parcial é gravada a cada ~2 s). "Fechar" fecha.
- **Exportar log** (topo da aba): baixa o log em JSONL ou CSV, de tudo (até 7 dias) ou da última hora / 24 h. A caixa
  "incluir prompt e resposta" só vale logado (sem sessão o servidor responde 401).
- **Tamanhos sem login:** nas rotas abertas (`/state`, o stream, `/history`, `/jobs`, `/jobs/{id}` e o export sem conteúdo)
  `bytes_in` e `resp_bytes` saem **arredondados para cima em KiB**, e o hash do prompt não aparece (revisão 10.L). O tamanho
  exato e o `prompt_sha256` ficam com o administrador (`/jobs/{id}` logado e `/jobs/{id}/content`).
- Estados do job: `na_fila`, `em_execucao`, `ok`, `erro`, `cancelado`, `cancelado_admin`, `removido`, `rejeitado`, `expirado`,
  `interrompido_reboot` (estava em andamento quando o **servidor reiniciou**) e `interrompido_gateway` (o processo do gateway
  morreu, sem reboot).

---

## 7. API do monitor

Base: `https://ia.cslsolucoes.com.br:8443/monitor/api`. Só da LAN. Todo corpo é JSON UTF-8. Erros: `{"error":"..."}`.
Os GET não têm efeito colateral. Todo POST exige **cookie de sessão + header `X-CSRF-Token`** (o token vem do login ou
de `/session`) e `Content-Type: application/json` quando há corpo; um POST de outra origem (`Origin` ou `Sec-Fetch-Site`
diferentes) dá 403.

| Método | Rota | Parâmetros | Resposta (resumo) |
| --- | --- | --- | --- |
| GET | `/monitor` e `/monitor/` | — | A página HTML (CSP restritiva, `no-store`) |
| GET | `/api/state` | — | Foto: `{now, version, boot_id, uptime_s, queue:{running[], waiting[], slots, in_use, totals}, gpu:{samples[], aer_corr_1b4, error?}, cards, recent[], events[], jobs[] (log de jobs, até 200), recent_done[] (terminaram nos últimos `keep_done_ms`), keep_done_ms, content_enabled, dropped, admin_enabled, backends[] (10.4: o quadro Backends, uma linha por backend: id, type, primary, host, state, breaker, drained, slots, used, free, queue, p50_ms, model, digest, processor, last_error, last_check, version, order, rules)}` |
| GET | `/api/jobs` | `desde`, `ate` (RFC 3339 ou epoch ms), `status` (lista separada por vírgula), `cliente` (exato), `limite` (padrão 200, máx. 1000), `offset` | **Log de jobs** (Fase 10.L): `{rows:[{...campos de /history..., responded, resp_bytes, backend, peak_power_w, peak_clock_mhz, samples}]}`, mais novo primeiro, **inclui `na_fila` e `em_execucao`**. Só metadados. Cache de 1 s |
| GET | `/api/jobs/{id}` | — | `{job, samples:[{ts, job_id, gpu, power_w, power_limit_w, clock_mhz, util_pct, mem_used_mib, temp_c}], content_enabled}`. O extrato tem 1 linha por GPU por segundo (`gpu` = -1 é a linha da CPU). **Sem o texto.** Só com a sessão do administrador (e pedido do próprio site) vem também `content:{prompt_bytes, resp_bytes, prompt_sha256, truncado, resp_final, stored_bytes}`: o hash e os tamanhos exatos permitiriam confirmar que um documento conhecido foi enviado (revisão 10.L) |
| GET | `/api/jobs/{id}/content` | — | **Só administrador logado** (cookie de sessão; sem sessão = **401**; pedido de outro site, `Sec-Fetch-Site` cross-site/same-site ou `Origin` alheio = **403**): `{prompt, response, prompt_bytes, resp_bytes, prompt_sha256, prompt_truncated, resp_truncated, truncado, resp_final, created_at, updated_at}`. `resp_final=false` = resposta parcial (em andamento ou job interrompido). 404 se o texto não existe (apagado) ou `--log-content=false` |
| GET | `/api/jobs/export` | `formato=jsonl\|csv` (padrão `jsonl`), `desde`, `ate`, `status`, `cliente`, `conteudo=1` | Baixa o log (arquivo `log-jobs-AAAAMMDD-HHMMSS.jsonl\|csv`), até 100.000 linhas. **Só metadados**; com `conteudo=1` inclui prompt e resposta e **exige a sessão do administrador** (401 antes do primeiro byte; 403 de outro site). No CSV, células que começam com `= + - @`, TAB, CR ou LF ganham um `'` na frente (injeção de fórmula). Sem `conteudo=1`, `bytes_enviados` e `bytes_resposta` saem arredondados para cima em KiB. `status` aceita só os status conhecidos (o resto é ignorado) |
| GET | `/api/stream` | — | SSE: a cada mudança (agrupadas em 150 ms) e a cada `--gpu-interval` (1 s), uma linha `data: <o mesmo JSON de /state>`. 503 + `Retry-After: 5` acima de 4 por IP / 32 total |
| GET | `/api/series` | — | `{every_s:5, points:[{t, waiting, running, finished, wait_avg_ms}]}`, até 720 pontos (1 h), só em memória |
| GET | `/api/history` | `desde`, `ate` (RFC 3339 ou epoch em ms), `cliente` (rótulo exato), `modelo` (exato), `limite` (padrão 100, máx. 1000), `offset` | `{rows:[{id, client_ip, client_label, tool, endpoint, model, lane, priority_in, priority_final, bytes_in, enqueued_at, started_at, finished_at, wait_ms, run_ms, status, http_status, error_class, prompt_eval_count, eval_count, load_ms, boot_id, think}]}`, mais novo primeiro (tempos em epoch ms). 400 se a data é inválida |
| GET | `/api/stats` | `janela=15m\|1h\|24h\|7d` (padrão `15m`) | `{window, total, ok, errors, by_status{}, by_class{}, wait_p50_ms, wait_p95_ms, run_p50_ms, run_p95_ms, per_minute, prompt_tokens, eval_tokens, tokens_per_sec, truncated}`. Cache de 5 s. 400 se a janela é outra |
| GET | `/api/audit` | `limite` (padrão 50, máx. 1000) | `{entries:[{ts, usuario, ip, acao, request_id, antes, depois, motivo}]}`, mais nova primeiro |
| GET | `/api/session` | — | `{admin_enabled, logged_in}` e, se logado, `user` e `csrf` |
| POST | `/api/login` | `{"user":"admin","password":"<SENHA>"}` | 200 `{user, csrf}` + cookie `mcpgw_session`. 401 senha errada; **429** + `Retry-After` bloqueado; 403 origem; 415 tipo; 503 sem `--admin-file` |
| POST | `/api/logout` | — (CSRF) | 200 `{"ok":"saiu"}` |
| POST | `/api/queue/{id}/promote` | `{"reason":"..."}` opcional | 200 `{ok, acao, id, antes, depois}`; 409 se não está mais esperando |
| POST | `/api/queue/{id}/demote` | idem | idem |
| POST | `/api/queue/{id}/remove` | idem | idem (o cliente recebe 409) |
| POST | `/api/running/{id}/cancel` | idem | idem; 409 `não está mais em execução` |
| POST | `/api/backends/{id}/drain` | `{"reason":"..."}` opcional | **10.4.** Drena o backend `{id}` (o id do `backends.json`, ou `gpu-local`): não recebe pedidos novos, o que roda termina. 200 `{ok, acao, id, antes, depois}`; **409** se já está drenado; **404** backend desconhecido; 400 id inválido; 401/403 como as outras ações |
| POST | `/api/backends/{id}/undrain` | idem | **10.4.** Retoma o backend. 409 se já está ativo |

O `{id}` são 32 caracteres hexadecimais (o `id` completo de `/state`, não os 8 da página). Outros códigos das ações:
400 id inválido, 401 sem sessão, 403 CSRF/origem, 503 sem `--admin-file`.

Exemplo de leitura e de uma ação por `curl` (a senha entra pelo teclado, não pela linha de comando):

```bash
URL=https://ia.cslsolucoes.com.br:8443/monitor/api; CA=~/.openclaude/certs/cloudflare-origin-ca.pem
curl -s --cacert $CA "$URL/state" | python3 -m json.tool | head -40
curl -s --cacert $CA "$URL/history?limite=5&cliente=meu-script"
read -rs -p "Senha: " PW; echo
printf '{"user":"admin","password":"%s"}' "$PW" | curl -s --cacert $CA -c /tmp/mcpgw.jar -H 'Content-Type: application/json' -d @- "$URL/login"; unset PW
# a resposta traz "csrf":"<CSRF>"; use-o abaixo
curl -s --cacert $CA -b /tmp/mcpgw.jar -H 'X-CSRF-Token: <CSRF>' -H 'Content-Type: application/json' \
  -d '{"reason":"teste"}' "$URL/queue/<ID de 32 hex>/promote"
rm -f /tmp/mcpgw.jar
```

---

## 8. Banco SQLite

**Dois bancos (Fase 10.L).** Ambos em `/var/lib/mcpollama-gateway/`, dono `mcpgw`, modo `0600` (com `-wal` e `-shm`), modo WAL:

| Arquivo | O que guarda | Retenção |
| --- | --- | --- |
| `gateway.db` | **só metadados**: pedidos, auditoria, amostras de GPU/CPU, o extrato de GPU por job, eventos | 90 dias (pedidos); 7 dias (amostras e extrato) |
| `content.db` | **o texto**: prompt e resposta completos de cada job (tabela `job_content`) | **7 dias**; teto de 2 GiB; 16 MiB por parte |

**Escrita durável do log de jobs.** Cada job é gravado em 3 tempos, todos **síncronos** (`synchronous=FULL`, num segundo
pool de conexões, fora do gravador em lote): ao **entrar na fila** (`na_fila`, com o prompt no `content.db`), ao **começar**
(`em_execucao`, com `started_at` e o `backend`) e ao **terminar** (estado final, tokens, "houve resposta"). Em transmissão
(stream), a resposta recebida é acrescentada ao `content.db` **a cada ~2 s** (e no fim), então uma queda de energia perde
no máximo os últimos ~2 s de texto. Enquanto há job rodando, o extrato de GPU (1 linha por GPU por segundo, no mesmo
ritmo do coletor de `--gpu-interval`) também é gravado de forma durável.

**Na partida**, todo job que ficou em `na_fila`/`em_execucao` é de um processo que morreu: vira `interrompido_reboot` (o
`boot_id` mudou: o servidor reiniciou) ou `interrompido_gateway` (mesmo boot: só o gateway morreu), o `finished_at` passa a ser
o **último segundo do extrato de GPU** daquele job (a melhor evidência de quando caiu) e o evento `jobs_interrompidos`
(`reboot=N reinicio_gateway=M total=T ids=...`) registra quantos foram. A resposta parcial já recebida fica no `content.db`.

**Gravação em lote do resto.** A gravação dos demais dados é
assíncrona e em lote (nunca atrasa a fila); se o buffer de 4096 itens enche, o item é descartado e contado (evento
`registros_descartados`, no máximo 1 por minuto). Os tempos são **epoch em milissegundos**.

| Tabela | Colunas |
| --- | --- |
| `requests` | `id` (PK), `client_ip`, `client_label`, `tool`, `endpoint`, `model`, `lane`, `priority_in`, `priority_final`, `bytes_in`, `enqueued_at`, `started_at` (0 se nunca rodou), `finished_at`, `wait_ms`, `run_ms`, `status`, `http_status`, `error_class`, `prompt_eval_count`, `eval_count`, `load_ms`, `boot_id`, `think`, **`responded`** (houve resposta, 0/1), **`resp_bytes`** (bytes de texto gerado recebidos), **`backend`** (onde rodou; `gpu-local`; vazio se nunca começou). Linhas anteriores à 10.L têm esses 3 campos NULL |
| `job_gpu_samples` | **Extrato por job (10.L)**: `ts`, `job_id`, `gpu` (**-1 = linha da CPU**), `power_w`, `power_limit_w`, `clock_mhz` (`clocks.gr`; -1 = não informado), `util_pct`, `mem_used_mib`, `temp_c`. Índices `(job_id, ts)` e `ts`. Só há linhas enquanto há job em execução; 7 dias (`--retention-job-gpu`) |
| `job_content` (**no `content.db`**) | `job_id` (PK), `created_at`, `updated_at`, `prompt` (BLOB), `prompt_bytes` (tamanho enviado), `prompt_sha256` (do prompt inteiro), `prompt_truncated`, `response` (BLOB), `resp_bytes` (recebido), `resp_truncated`, `resp_final` (0 = parcial/em andamento/interrompido). Para embeddings a resposta é só a contagem e a dimensão ("embeddings: 2 vetor(es) de 768 dimensões"): **o vetor nunca é gravado** |
| `audit` | `id`, `ts`, `usuario`, `ip`, `acao`, `request_id`, `antes`, `depois`, `motivo` |
| `gpu_samples` | `ts`, `gpu`, `bus_id`, `power_w`, `mem_used_mib`, `util_pct`, `temp_c`, `aer_corr_1b4`, `boot_id`, `clock_mhz` (clock gráfico; NULL nas linhas anteriores à 10.L), `power_limit_w` (limite aplicado em W; `NULL` nas linhas antigas, gravadas antes do campo existir; -1 = o driver não informou) |
| `cpu_samples` | `ts`, `util_pct`, `load1`, `load5`, `load15`, `ram_total_mib`, `ram_used_mib`, `ram_avail_mib`, `swap_used_mib`, `swap_total_mib`, `temp_c`, `freq_mhz`, `ollama_rss_mib`, `power_w`, `boot_id`, `threads_json` (índice em `ts`). -1 = não legível. `threads_json` é o detalhe por thread, curto: `{"t":[[id,uso_%,core_id],...],"c":[[core_id,temp_c],...]}` |
| `backend_samples` (**10.4**) | `ts`, `backend_id`, `estado` (`ok`, `sobrecarregado`, `disjuntor_aberto`, `fora`, `degradado`, `drenado`), `slots_usados`, `fila`, `p50_ms`. Uma linha por backend a cada `--sample-store-interval`. Índice em `ts`. Só ids, estados e contadores: **nunca** URL nem token |
| `events` | `ts`, `kind`, `detail` |
| `meta` | `k`, `v` (`boot_id`, `last_prune`, `last_vacuum`) |

Valores de `status`: `na_fila` e `em_execucao` (em andamento, 10.L), `ok`, `erro`, `cancelado` (cliente desistiu), `cancelado_admin`, `removido`, `rejeitado` (fila cheia),
`expirado` (`--max-wait`), `interrompido_reboot` e `interrompido_gateway` (10.L, ver acima). Valores de `error_class` (texto **fixo** do gateway): `upstream_<código>`, `stream_error`,
`run_timeout`, `client_gone`, `upstream_unreachable`, `upstream_dropped`, `admin_cancel`, `removido`, `expirado`, e para
rejeitados `fila cheia` ou `limite de pedidos em espera deste cliente`. `think`: `on`, `off`, `modelo` ou vazio.

**Retenção** (rodada de manutenção de hora em hora; a limpeza por idade, no máximo uma vez a cada 24 h):

| Dado | Regra |
| --- | --- |
| `requests` | 90 dias (`--retention-requests`) **e** teto de 3.000.000 linhas (`--max-request-rows`), as mais antigas saem primeiro (evento `teto_pedidos`) |
| `job_gpu_samples` | 7 dias (`--retention-job-gpu`) |
| `job_content` (`content.db`) | 7 dias (`--retention-content`) **e** teto total de 2 GiB (`--content-max-bytes`): passou, apaga os jobs mais antigos até 90% (em segundo plano, em transações de 500 jobs: o pedido que passou do teto não espera) e grava o evento `teto_conteudo`. Se o apagamento falha (disco cheio, banco travado ou corrompido), grava `teto_conteudo_falhou` (no máximo 1 por minuto) e só tenta de novo depois de 1 min; 16 MiB por parte (`--content-max-job-bytes`), cortado com `truncado=sim`. Hora em hora; o arquivo devolve as páginas livres ao disco (`auto_vacuum` incremental), mas pode ficar um pouco maior que o total lógico |
| `gpu_samples`, `cpu_samples` e `backend_samples` (10.4) | 7 dias (`--retention-gpu`); gravam 1 linha a cada `--sample-store-interval` (5 s), não a cada coleta |
| `audit` e `events` | **nunca expiram** |
| `VACUUM` | semanal |

**Migração (10.L):** a abertura também acrescenta `requests.responded`, `requests.resp_bytes`, `requests.backend`, `gpu_samples.clock_mhz` e cria `job_gpu_samples`, cada passo só se faltar (rodar duas vezes não muda nada). A ordem de gravação mudou: o registro do pedido passa a existir **desde a entrada na fila** e o fim vira um `UPSERT` (a linha e o `rowid` são os mesmos). Voltar a um binário antigo: ele ignora as colunas novas, mas **não** conhece os estados `na_fila`/`em_execucao`/`interrompido_*`.

**Migração (10.4):** a abertura acrescenta `requests.failover_rule` (a regra que mandou o job a um secundário: `F2:S1`, `F2:S2`, `F2:S3`, `F2:S1+S2`, `F2:hist`; vazio = rodou no primário) e cria `backend_samples`, cada passo só se faltar (idempotente, testado com 3 aberturas seguidas). `requests.backend` (da 10.L) passa a trazer o id real do backend onde o job rodou (`gpu-local` ou o id do `backends.json`); o `Started` do log de jobs grava o backend do ticket, e o `Finish` repete (o UPSERT mantém o valor gravado no início).

**Migração (anterior):** ao abrir um banco antigo, o gateway cria `cpu_samples` e acrescenta `gpu_samples.power_limit_w` (`ALTER TABLE ... ADD COLUMN`), cada passo só se faltar; abrir de novo não muda nada, e os dados existentes ficam intactos. Para voltar a um binário antigo basta ignorar a tabela e a coluna novas (o binário antigo não as lê).

**Garantia de privacidade (alterada na 10.L, decisão do owner de 2026-10-30).** O `gateway.db` continua **só com
metadados**: não existe coluna para prompt, documento, resposta ou token, e o `error_class` nunca carrega texto do Ollama.
O **texto** passou a existir, mas **somente no `content.db`**: prompt e resposta completos, por 7 dias, visíveis só ao
administrador logado (ver a seção 12). O **token do cliente** (`Authorization`) não é gravado em nenhum dos dois bancos nem no
log. Os testes-sentinela provam as duas metades: o texto-sentinela está no `content.db` e **não** está no `.db`, no `-wal`, no
`-shm` nem no log; o token-sentinela não está em arquivo nenhum.

**Como consultar um job interrompido** (no servidor; `-readonly`):

```bash
DB=/var/lib/mcpollama-gateway/gateway.db; CDB=/var/lib/mcpollama-gateway/content.db
# O que aconteceu nos últimos reinícios
sqlite3 -readonly -header -column "$DB" "select datetime(ts/1000,'unixepoch','localtime') quando, kind, detail from events where kind in ('boot_novo','gateway_inicio','jobs_interrompidos') order by ts desc limit 10;"
# Os jobs interrompidos, com o último segundo vivo, o backend e se já havia resposta
sqlite3 -readonly -header -column "$DB" "select substr(id,1,8) id, datetime(enqueued_at/1000,'unixepoch','localtime') entrou, status, client_label, tool, bytes_in, responded, resp_bytes, backend, datetime(finished_at/1000,'unixepoch','localtime') ultimo_segundo from requests where status like 'interrompido%' order by enqueued_at desc limit 20;"
# O extrato de GPU dos últimos 10 segundos antes da queda (use o id completo; W por placa)
sqlite3 -readonly -header -column "$DB" "select datetime(ts/1000,'unixepoch','localtime') hora, gpu, power_w, power_limit_w, clock_mhz, util_pct, temp_c from job_gpu_samples where job_id='<ID de 32 hex>' order by ts desc, gpu limit 40;"
# O que foi enviado e a resposta parcial (texto sensível: só o root, no servidor)
sqlite3 -readonly "$CDB" "select prompt_bytes, resp_bytes, resp_final, prompt_truncated, resp_truncated, cast(prompt as text) from job_content where job_id='<ID de 32 hex>';"
sqlite3 -readonly "$CDB" "select cast(response as text) from job_content where job_id='<ID de 32 hex>';"
```

Pelo monitor: aba "Log em tempo real" → filtre/ordene → **Detalhes** (o texto exige login). Para guardar o log: **Exportar log**
(JSONL ou CSV; com "incluir prompt e resposta" só logado) ou `GET /monitor/api/jobs/export` (seção 7).

**Consultas úteis** (no servidor; use `-readonly` para não interferir no gateway; se o `sqlite3` não estiver instalado,
use `/monitor/api/history` e `/stats`):

```bash
DB=/var/lib/mcpollama-gateway/gateway.db
Q() { sqlite3 -readonly -header -column "$DB" "$1"; }

# Últimos 20 pedidos
Q "select datetime(enqueued_at/1000,'unixepoch','localtime') quando, client_label, tool, model, think, status, http_status, wait_ms, run_ms, eval_count, load_ms from requests order by enqueued_at desc limit 20;"
# Quem usa mais (últimas 24 h)
Q "select client_ip, client_label, count(*) n, round(avg(wait_ms)) espera_ms, round(avg(run_ms)) rodou_ms, sum(eval_count) gerados from requests where enqueued_at > (strftime('%s','now')-86400)*1000 group by 1,2 order by n desc limit 20;"
# Erros por classe (24 h)
Q "select status, error_class, count(*) n from requests where status<>'ok' and enqueued_at > (strftime('%s','now')-86400)*1000 group by 1,2 order by n desc;"
# Pedidos que recarregaram o modelo (load_ms alto)
Q "select datetime(enqueued_at/1000,'unixepoch','localtime') quando, client_label, load_ms from requests where load_ms > 5000 order by enqueued_at desc limit 20;"
# Raciocínio ligado
Q "select datetime(enqueued_at/1000,'unixepoch','localtime') quando, client_label, model, run_ms from requests where think='on' order by enqueued_at desc limit 20;"
# Ações do administrador
Q "select datetime(ts/1000,'unixepoch','localtime') quando, usuario, ip, acao, substr(request_id,1,8) id, antes, depois, motivo from audit order by id desc limit 20;"
# Eventos (inícios, reboots, descartes)
Q "select datetime(ts/1000,'unixepoch','localtime') quando, kind, detail from events order by ts desc limit 20;"
# GPU na última hora: pico de potência e temperatura por placa
Q "select gpu, max(power_w) pico_w, max(temp_c) temp_max, max(aer_corr_1b4) aer from gpu_samples where ts > (strftime('%s','now')-3600)*1000 group by gpu;"
# Potência x limite aplicado por placa na última hora (limite NULL/-1 = não informado)
Q "select gpu, round(max(power_w),1) pico_w, max(power_limit_w) limite_w from gpu_samples where ts > (strftime('%s','now')-3600)*1000 group by gpu;"
# CPU/RAM na última hora: pico de uso, temperatura, RAM e RAM do Ollama
Q "select round(max(util_pct),1) cpu_pico, round(max(temp_c),1) temp_max, max(ram_used_mib) ram_pico_mib, max(ollama_rss_mib) ollama_mib from cpu_samples where ts > (strftime('%s','now')-3600)*1000;"
# Tamanho e esquema (prova de que não há coluna de texto de conteúdo)
ls -lh /var/lib/mcpollama-gateway/; sqlite3 -readonly "$DB" .schema
```

Pela API: `curl -s --cacert <CA> "https://ia.cslsolucoes.com.br:8443/monitor/api/history?limite=20"` e `/stats?janela=24h`.

---

## 9. Operação do dia a dia

```bash
systemctl status mcpollama-gateway --no-pager
journalctl -u mcpollama-gateway -f                    # ao vivo (Ctrl+C sai)
journalctl -u mcpollama-gateway --since "1 hour ago" --no-pager
ollama ps                                             # modelo residente, 100% GPU, contexto
curl -s http://127.0.0.1:11500/healthz                # ok
```

**Backups:** `/root/srv-backup-20261028/fase96-*`:
- `fase96-AAAAMMDD-HHMMSS/` (etapa 1): cópia do binário anterior;
- `fase96-ativacao-AAAAMMDD-HHMMSS/` (etapa 2, modo `0700`): `ollama.conf` (nginx), `10-performance.conf` (Ollama) e `rollback.sh`. É o argumento do rollback (seção 11).

Fazer cópia do banco com o gateway rodando: `sqlite3 /var/lib/mcpollama-gateway/gateway.db ".backup '/root/srv-backup-20261028/gateway-$(date +%Y%m%d).db'"` (cópia consistente em WAL).

**Trocar a senha do administrador:**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Senha
```

(à mão: `ssh -t lx-ai01-root 'bash /root/mcpgw-definir-senha.sh'`.) Você digita a senha duas vezes, sem eco (mínimo 12
caracteres); só o hash PBKDF2 é gravado em `/etc/mcpollama-gateway/admin`. **O arquivo é lido só na partida**: depois de
trocar, rode `systemctl restart mcpollama-gateway` (isso também encerra as sessões abertas e esvazia a fila). Não digite a senha
em `echo` nem em argumento de comando (ficaria no histórico do shell).

**Mudar uma flag:** edite a unit no servidor, recarregue e reinicie:

```bash
cp -p /etc/systemd/system/mcpollama-gateway.service /root/srv-backup-20261028/mcpollama-gateway.service.$(date +%Y%m%d-%H%M%S)
nano /etc/systemd/system/mcpollama-gateway.service        # ajuste as linhas do ExecStart
systemctl daemon-reload && systemctl restart mcpollama-gateway
systemctl is-active mcpollama-gateway; journalctl -u mcpollama-gateway -n 5 --no-pager
```

O evento `gateway_config` (tabela `events`) registra a configuração de cada partida. **Replique a mudança** em
`deploy/gateway/mcpollama-gateway.service` no repositório: senão, uma reinstalação (`Completo`) sobrescreve a edição.
Se a partida falhar por flag inválida, o serviço entra em laço (`Restart=always`): `journalctl` mostra
`mcpollama-gateway: <erro>`; desfaça a edição.

**Adicionar IPs com prioridade `alta`:** na unit, `--priority-ips` aceita lista separada por `,` ou `;`:
`--priority-ips "10.100.2.0/24;10.100.3.15"`. Depois `daemon-reload` + `restart`. Não coloque `127.0.0.1` (um pedido sem
`X-Real-IP` vindo do loopback seria tratado como prioritário).

**Outros ajustes comuns:** `--debug` (uma linha de log por pedido), `--default-think=true` (raciocínio ligado por padrão),
`--gen-slots` (só aumente se o Ollama passar a atender mais de 1 pedido em paralelo), `--max-wait`/`--run-timeout`.

---

## 10. Diagnóstico

| Sintoma | Causa provável | O que fazer |
| --- | --- | --- |
| **401** `{"error":"unauthorized"}` em `/api/...` | Bearer ausente ou errado (barrado no nginx) | Conferir o token do cliente (`--api-key`, `OPENAI_API_KEY`). Nunca colar o token em chat ou log |
| **401** em `/monitor/api/queue/...` | Sem sessão de administrador (ou sessão vencida, 8 h) | Entrar de novo na página |
| **403** em `/api/pull`, `push`, `create`, `delete`, `copy`, `blobs` | Esperado: endpoint administrativo bloqueado no nginx e no gateway | Gerenciar modelos só no servidor, pelo `ollama` local |
| **403** `modelo "X" não permitido neste servidor` | Modelo fora de `--allowed-models` | Usar `qwen3-ia` ou `nomic-embed-text` (ou `qwen3-ia-think`) |
| **403** no `/monitor` | IP fora de `10.100.2.0/24` (inclusive via Cloudflare) | Acessar da LAN; ver `/monitor` por túnel só se o owner decidir |
| **403** `origem não permitida` / `token CSRF ausente ou inválido` | Ação do monitor feita por outro site/ferramenta, ou sem o header | Usar a página, ou o `curl` da seção 7 |
| **409** `removido da fila pelo administrador` / `cancelado pelo administrador` | O administrador agiu no `/monitor` | Ver a auditoria (motivo); o cliente pode repetir |
| **429** (página HTML do nginx) | Mais de 80 pedidos simultâneos do mesmo IP, ou mais de 600/min (rajada 100) | Reduzir a concorrência do cliente; é o nginx, não a fila |
| **503** JSON `fila do servidor cheia...` + `Retry-After` | Fila do gateway cheia (128 em produção) ou cota por IP/rótulo (64 em produção) | Esperar o `Retry-After` (o mcpollama repete 1 vez); ver quem ocupa a fila no `/monitor`; diminuir a rajada ou remover pedidos presos |
| **503** `muitos painéis abertos` | Mais de 4 abas SSE do mesmo IP | Fechar painéis antigos |
| **502** JSON `Ollama indisponível` | Ollama reiniciando ou parado | `systemctl status ollama`, `ollama ps`; aguardar a recarga (~20 s) |
| **502** com página HTML do nginx (e **tudo** dá 502) | **Gateway parado** (o nginx aponta para ele) | `systemctl status mcpollama-gateway`, `journalctl -u mcpollama-gateway -n 50`; `systemctl restart`; se não voltar, rollback (seção 11) |
| **504** `tempo máximo de espera na fila esgotado` | Ficou mais de 10 min esperando | Fila sobrecarregada: ver o `/monitor`; reduzir carga ou promover o pedido |
| **504** `tempo máximo de execução no servidor esgotado` (ou linha final de erro no stream) | Geração passou de 10 min (`--run-timeout`) | Prompt/saída enorme ou modelo travado; reduzir a tarefa; cancelar no monitor; investigar o `ollama ps` |
| Serviço não sobe, `tem permissão -rw-r-----: use 0600` | Arquivo da senha com permissão de grupo/outros | `chown mcpgw:mcpgw /etc/mcpollama-gateway/admin; chmod 0600 /etc/mcpollama-gateway/admin; systemctl restart mcpollama-gateway` |
| Serviço não sobe, outro erro | Flag inválida, porta ocupada, banco sem permissão | `journalctl -u mcpollama-gateway -n 30`; o erro diz o campo |
| Login do monitor **429** | 5 falhas do seu IP em 15 min (bloqueio, que dobra a cada vez) | Esperar o `Retry-After`; senha esquecida: modo `Senha` (a seção 9) e `restart` |
| Login do monitor **503** `administração desabilitada` | Gateway subiu sem `--admin-file` | Conferir a unit e o arquivo `/etc/mcpollama-gateway/admin` |
| `/monitor` abre mas sem botões | Não está logado, ou `somente leitura` | Entrar com o usuário `admin` |
| Página "sem conexão com o servidor" | Gateway parado, ou o nginx sem `location /monitor` | `systemctl status`; conferir `ollama.conf` |
| GPUs vazias ou erro `nvidia-smi:` / `aer:` no rodapé das GPUs | `nvidia-smi` sem acesso sob o sandbox da unit, ou AER ausente | Ver o texto do erro; não afeta a fila. Pendência 9.3 da revisão |
| Modelo recarregando a cada pedido (`load_ms` alto, ~16–22 s) | Algo mudou o contexto/residência: reinício do Ollama, `ollama-warm` parado, cliente que ainda manda `num_ctx` (o gateway remove), ou o Ollama foi tocado direto | `ollama ps` antes e depois; consulta de `load_ms` (seção 8); `systemctl status ollama-warm` |
| `ia-local` lento, raciocinando | Está usando `qwen3-ia-think` ou mandando `reasoning_effort` | Na coluna Raciocínio do `/monitor` deve estar `desligado`; ajustar `OPENAI_MODEL=qwen3-ia` no `csl-ollama.env` |
| Cliente aparece como `anonimo` | Não manda `X-Mcpollama-Client` (ex.: `ia-local`, `curl`) | Normal; a justiça por IP vale. Para o mcpollama, `--client-label` |
| `prioridade alta` não surte efeito | IP fora de `--priority-ips` (o pedido vira `normal` em silêncio), ou a fila estava vazia | Conferir a coluna "Prioridade" no monitor; ajustar `--priority-ips` |
| Pedido demora mesmo com fila vazia | Modelo carregando, prompt grande, ou raciocínio ligado | Ver `load_ms`, `prompt_eval_count`, coluna Raciocínio |

---

## 11. Voltar atrás (rollback)

**Pelo instalador:**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Reverter -Backup /root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS
```

**À mão, no servidor:** `bash <B>/rollback.sh <B>`, onde `<B>` é a pasta `fase96-ativacao-*` (a etapa 2 imprime o caminho:
`ETAPA2_OK (backups e rollback em <B>: ...)`). O `rollback.sh` roda **da cópia guardada no backup** (pasta só do root).
Liste: `ls -d /root/srv-backup-20261028/fase96-ativacao-*`. **Para voltar ao estado anterior ao gateway use o mais antigo**
(cada reinstalação cria um backup novo do estado já com gateway).

O que ele faz, em ordem:
1. restaura o `ollama.conf` do nginx (nginx volta a apontar direto ao Ollama) e o `10-performance.conf` do Ollama (volta a escutar na LAN, `0.0.0.0:11434`);
2. `nginx -t && systemctl reload nginx`;
3. `daemon-reload`, reinicia `ollama` e `ollama-warm` e aquece o `nomic-embed-text` (imprime `nomic_ok`);
4. `systemctl disable --now mcpollama-gateway`;
5. mostra `ss -tlnH` e `ollama ps`, e termina com `ROLLBACK_FEITO`.

**Não remove** o usuário `mcpgw`, o binário, a unit, o arquivo da senha nem o banco: reinstalar depois é rápido.
O Ollama volta a reiniciar (recarga do modelo, ~20 s).

**Como provar que voltou:**

```bash
systemctl is-enabled mcpollama-gateway; systemctl is-active mcpollama-gateway    # disabled / inactive
ss -tlnH | grep -E '11434|11500'          # 11434 em 0.0.0.0 (ou *), nada em 11500
printf 'Authorization: Bearer %s\n' "$(sed -n 2p /root/API-Key)" | curl -sk https://127.0.0.1:8443/api/version -H @- -D - | grep -ci x-request-id   # 0: sem gateway
curl -sk -o /dev/null -w '%{http_code}\n' https://127.0.0.1:8443/monitor          # deixa de ser a página do monitor
ollama ps
```

**Rollback parcial** (só tirar o gateway do caminho, mantendo o Ollama no loopback; previsto no plano, **não** é script):
no `ollama.conf` do servidor troque, no `location /`, `proxy_pass http://127.0.0.1:11500;` por `http://127.0.0.1:11434;`,
rode `nginx -t && systemctl reload nginx`. Perde fila, políticas e monitor, mas não reinicia o Ollama.

Se a **etapa 2 falhar depois de mexer no nginx/Ollama**, o rollback é automático (`trap`). Falha **antes** disso (por exemplo
o gateway não subir): o nginx ainda aponta para o Ollama (nada mudou para os clientes); investigue com a seção 10 e rode de novo.

---

## 12. Segurança

**Garantido** (revisão adversarial 9.5, Opus 5.5: 30 vetores executáveis, **0 achados Altos**, 5 Médios e 7 Baixos, todos corrigidos com teste de regressão; reexecutada na 9.6a sem regressões):

- o IP do cliente só vale a partir do `X-Real-IP` do nginx local; forjado, duplicado ou de fora é ignorado;
- o `/monitor` não é alcançável pela internet, nem pelo túnel do Cloudflare (403 para pedidos com `CF-Connecting-IP`; provado em 2026-10-30, ver a seção 6);
- sem *request smuggling* (CL/TE duplicados) e sem acesso às 6 rotas administrativas, em 17 variantes de escrita; 0 de 17 chegou ao Ollama;
- o `Authorization` do cliente **não** é repassado ao Ollama; o token não entra no log, em banco algum nem em `error_class`; prompt e resposta não entram no log nem no `gateway.db` (**mudança da 10.L:** o texto é gravado, mas só no `content.db`, ver abaixo) (teste de sentinela);
- os contadores vêm só do último pedaço da resposta; o texto do modelo não os engana;
- nenhum slot "vaza": Ollama que cai, cliente que desiste, ações simultâneas do administrador (`-race`, 1.800 pedidos = 1.800 registros);
- `--run-timeout` impede um stream sem fim de prender o slot; teto de linhas e retenção limitam o banco; o cálculo de estatísticas é limitado e com cache;
- `/monitor`: sem XSS (a página usa `textContent`, CSP com hash, `default-src 'none'`), CSRF (token, `Origin`/`Sec-Fetch-Site`, só JSON), cookie `HttpOnly; Secure; SameSite=Strict`, token de sessão novo a cada login, senha com PBKDF2-HMAC-SHA256 (600.000 iterações), comparação em tempo constante;
- login: 5 tentativas por IP com backoff exponencial, sem bloqueio global, e reserva atômica (40 tentativas paralelas gastam 5 verificações);
- SQL parametrizado (0 injeção nos filtros do `/history`);
- as etapas de instalação conferem hashes (pasta `0700` só do root, `SHA256SUMS`, binário conferido de novo), e o token nunca vai em linha de comando.

**Sigilo do conteúdo dos jobs (Fase 10.L, decisão do owner de 2026-10-30).** O gateway **passou a gravar o prompt e a resposta
completos** de cada job (inclusive `extract`, `summarize_file` e `translate` com o conteúdo de documentos e dados de clientes).
Isto substitui a regra antiga "nenhum texto no banco". O que protege:
- **onde:** só no `content.db` (`/var/lib/mcpollama-gateway/content.db`, `0600`, dono `mcpgw`), nunca no `gateway.db`; o `ProtectSystem=strict`
  do serviço só deixa o `mcpgw` escrever em `/var/lib/mcpollama-gateway`. Quem lê o disco como root lê o texto: trate o
  servidor e **os backups dessa pasta** como dado sigiloso (o `content.db` não deve ir para backup compartilhado);
- **quem vê no monitor:** só o administrador logado (cookie de sessão `HttpOnly; Secure; SameSite=Strict`); sem sessão,
  `GET /api/jobs/{id}/content` e a exportação com `conteudo=1` dão **401**. O resto do log (metadados, extrato de GPU) segue visível na LAN;
- **por quanto tempo:** 7 dias (`--retention-content`), teto total de 2 GiB (apaga os mais antigos) e 16 MiB por parte;
- **como sai na tela:** sempre por `textContent` (um prompt com `</script><img onerror=...>` aparece como texto; os JSON escapam `<`, `>` e `&`);
- **como desligar:** `--log-content=false` (o log de jobs, o extrato e a recuperação continuam; o texto não é gravado nem o `content.db` criado). Para
  apagar o que já existe: pare o gateway e remova `content.db*`;
- **embeddings:** só a contagem e a dimensão, nunca o vetor.
O token do cliente nunca é gravado.

**Riscos residuais e pendências:**

1. ~~**Nota N1:** arquivo da senha `0640` x código que exige `0600`~~ — **resolvida** (scripts gravam `mcpgw:mcpgw 0600`; provado no servidor);
2. o **rótulo do cliente é livre** (só o IP é confiável); a justiça ancora no IP;
3. `--priority-ips 10.100.2.0/24` dá `alta` a **qualquer máquina da LAN com o token**; isso é decisão do owner (D9-8), e vale só como pedido (o mcpollama não usa);
4. **um único administrador** compartilhado; a auditoria registra o usuário, não a pessoa;
5. os GET do `/monitor` são abertos na LAN (IPs, rótulos, modelos, contadores, extrato de GPU; **nunca o texto dos jobs**, que exige o administrador logado);
6. o login e o SSE contam por IP completo (não `/64`); hoje o `/monitor` só atende a LAN IPv4. Se for exposto a IPv6, usar `fairIP`;
7. o gateway **não tem prazo de leitura do corpo**; depende do `proxy_request_buffering on` do nginx (padrão; o `ollama.conf` não o desliga). Não desligue;
8. um pedido sem `X-Real-IP` vindo do loopback conta como `127.0.0.1`: **não** ponha `127.0.0.1` em `--priority-ips` e mantenha `proxy_set_header X-Real-IP` em todo `location` que aponta ao gateway;
9. `--run-timeout` (10 min) e o teto de 3 milhões de linhas são **valores de partida**, a validar com o volume real (aceite 9.7);
10. do 9.6a, **a validar ao vivo** (uma chamada de cada vez): `think:false` e `reasoning_effort:"none"` no `qwen3.5:9b` com Ollama v0.40.8, `qwen3-ia-think` nas duas APIs e que o `options` limpo não recarrega o modelo (`ollama ps` antes e depois);
11. a página nunca foi aberta num navegador real na 9.5; o aceite visual é da 9.7;
12. o cookie `Secure` exige HTTPS (sempre o caso na 8443);
13. `Retry-After` fixo em 5 s; `X-Queue-Position` é a posição na entrada, não atualizada; 502 sem `Retry-After` quando o Ollama reinicia;
14. `nvidia-smi` sob `ProtectSystem=strict` e o AER ausente podem aparecer como erro no rodapé das GPUs.

---

## 13. Limites e futuro

**O gateway de hoje (`0.12.3`, em produção desde 2026-10-30 22:35)** tem 2 faixas (geração e embedding), ambas na **GPU**, e um
**pool de backends** (seção 16). Em produção o pool tem **um único backend, o `gpu-local`** (o Ollama do próprio servidor,
`127.0.0.1:11434`): sem `--backends` tudo se comporta como antes da 10.4. Com `--backends`, o gateway repassa o trabalho que
**ainda não começou** a Ollamas **secundários** quando o primário está sobrecarregado ou sem saúde (`--overload-wait` e
`--overload-queue` regulam o repasse).

**Decidido e descartado**

- **Faixa CPU/RAM** (segunda instância `ollama-cpu` com roteamento por pedido): **descartada** pelo owner (D9-C1; Fase 10.1/10.2).
  A medição deu `qwen3-ia` a ~4,6 tok/s de geração e ~25 tok/s de prefill na CPU, com interferência de -4 a -10% na GPU: o saldo de
  vazão era ~0. As regras R1 a R9, os headers `X-Mcpollama-Lane`, `X-Lane` e `X-Routing-Reason` e as ações "mover para GPU/CPU"
  do plano (§11 e §11.9) **não foram implementados** e nenhuma flag deles existe no `config.go`.

**Planejado, sem data**

- **Secundário real:** um segundo servidor com 4 GPUs RTX 2060 está planejado, **sem data** (etapa 10.S). Até lá o pool tem só o
  `gpu-local`. A revisão adversarial do pool (10.5) já foi feita (seção 16).
- Etapas 9.9 a 9.16 do plano (medições antes de ligar qualquer coisa).

Hoje, para aguentar mais carga, os botões são `--gen-slots` (inútil enquanto o Ollama atende 1 pedido por vez), as
cotas/limites da fila e, quando houver o secundário, o `--backends`.

---

## 14. Referências

| O quê | Onde |
| --- | --- |
| Instalador | `E:\MCPOllam\deploy\gateway\Instalar-Gateway.ps1` |
| Pasta de implantação | `E:\MCPOllam\deploy\gateway\` (`README.md`, `mcpollama-gateway.service`, `ollama.conf`, `stage1.sh`, `stage2.sh`, `rollback.sh`, `mcpgw-definir-senha.sh`) |
| Flags e padrões | `E:\MCPOllam\src\cmd\mcpollama-gateway\config.go`, `main.go` |
| Fila, proxy, políticas | `E:\MCPOllam\src\gateway\queue\queue.go`, `proxy\proxy.go`, `proxy\policy.go` |
| Monitor, página, login | `E:\MCPOllam\src\gateway\monitor\monitor.go`, `admin.go`, `index.html` |
| Banco e GPU | `E:\MCPOllam\src\gateway\store\store.go`, `gpu\gpu.go`, `cpu\cpu.go` |
| Plano (decisões D9-*, changelog, §11) | `E:\MCPOllam\.workspace\plans\MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md` |
| Revisão adversarial | `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md` |
| Contexto do servidor | `E:\MCPOllam\CLAUDE.md`; `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md` |
| No servidor | `/usr/local/bin/mcpollama-gateway`, `/etc/systemd/system/mcpollama-gateway.service`, `/etc/mcpollama-gateway/admin`, `/var/lib/mcpollama-gateway/gateway.db`, `/etc/nginx/conf.d/ollama.conf`, `/etc/systemd/system/ollama.service.d/10-performance.conf`, `/root/deploy96/`, `/root/srv-backup-20261028/fase96-*` |
| CA do nginx (cliente) | `%USERPROFILE%\.openclaude\certs\cloudflare-origin-ca.pem` |

---

## 15. Notas de conferência

Divergências entre fontes, resolvidas a favor do **código** (e dos scripts como estão hoje), conferidas em 2026-10-30.

| Nº | Divergência | Resolução / o que fazer |
| --- | --- | --- |
| **N1** | **Permissão do arquivo da senha.** `admin.go:90` (`LoadAdminFile`) recusa a partida se `perm & 0o077 != 0`, e o `--help` diz "mode 0600". Mas `stage2.sh` e `mcpgw-definir-senha.sh` gravam `root:mcpgw` **`0640`** (o grupo precisa ler porque o serviço roda como `mcpgw`). Por leitura do código: o gateway sai com código 2 (`--admin-file ... tem permissão -rw-r-----: use 0600`) e o systemd reinicia em laço; a etapa 2 para em `systemctl is-active` (antes do `trap`) com o nginx **ainda no Ollama**, e ela mesma refaz o `chmod 0640` se rodada de novo. **Não executado em Linux nesta conferência.** | **Resolvida em 2026-10-30 10:50 (opção A, sem mexer no código):** `stage2.sh` e `mcpgw-definir-senha.sh` gravam `mcpgw:mcpgw` modo `0600`. A pasta `/etc/mcpollama-gateway` é `root:mcpgw 0750`, então o `mcpgw` lê o próprio arquivo. Prova no servidor com o binário `0.9.0-2cb599d` rodando como `mcpgw` na porta de teste 11599: `0640` → código 2 ("use 0600"); `0600` → `/monitor` 200. A etapa 2 também corrige o modo de uma senha gravada pelo script antigo. |
| N2 | O `README.md`/stage1 antigos usavam `/tmp/deploy96` e o hash do **binário**. | Hoje: `/root/deploy96` (root `0700`) e o hash do **`SHA256SUMS`**, em `stage1.sh`, `stage2.sh` e `Instalar-Gateway.ps1` (conferidos). O manual segue o estado atual. |
| N3 | Os scripts apontam `bash /root/mcpgw-definir-senha.sh`; o arquivo só existe em `/root/` depois que o **instalador** o copia (ou o passo b da seção 2.2). | Documentado nos dois caminhos. O modo `Senha` só funciona depois de uma rodada de `Preparar`/`Completo`/`Atualizar`. |
| N4 | `Instalar-Gateway.ps1 -Modo Atualizar` **não** instala a unit nem o nginx (só a etapa 1 e o restart). | Mudança de flag/unit exige `Completo -SemBuild` ou a edição manual da seção 9. |
| N5 | Troca de senha pelo modo `Senha` **não reinicia** o gateway, e o arquivo é lido só na partida. | Documentado: `systemctl restart mcpollama-gateway` depois. |
| N6 | Plano §9.4: "5 falhas por IP **e 30 no total**" no login. | Substituído na 9.5: não há teto global; 5 por IP com backoff exponencial (15 min até 24 h). Vale o código. |
| N7 | Plano §3: "`alta` só vem do administrador"; §4.3: "pedido em execução não é interrompido na v1". | Valem a decisão **D9-8** e o código: `alta` por IPs autorizados (`--priority-ips`); **cancelar em execução existe**. |
| N8 | Plano §6 fala em hash **bcrypt**. | O código usa **PBKDF2-HMAC-SHA256** (600.000 iterações); o plano §9.4 já registra a troca. |
| N9 | Plano §4.2 não lista `/session`, `/login`, `/logout`, `/series` nem `/running/{id}/cancel`; o `history` aceita também `limite` e `offset`. | Tabela da seção 7 segue o código. |
| N10 | `--gen-slots`: padrão do código `2`; plano e unit usam `1`. | Padrão `2` na tabela; produção `1`. |
| N11 | Revisão 9.5 (pendência 3) cita `allow 10.100.0.0/16` no nginx; o `ollama.conf` atual tem `10.100.2.0/24`. | Vale o `ollama.conf` (`/24`). |
| N12 | `hash-password`: mínimo de 8 caracteres no código; `mcpgw-definir-senha.sh` exige 12. | O script é mais rígido; vale 12 na prática. |
| N13 | `CLAUDE.md` descreve o gateway como "ainda não implantado". | O manual não afirma o estado do servidor; confira com a seção 2.3. |
| N14 | O mcpollama envia só `X-Mcpollama-Client` e `X-Mcpollama-Tool` (conferido em `src/ollama/api.go`); `X-Mcpollama-Priority` e `X-Mcpollama-Think` não são enviados por ele. | Usados por `curl`/scripts; o mcpollama usa os padrões do gateway. |
| N15 | O `stage2.sh` espera `pull: 403` e `sem token: 401` pelo nginx; isso decorre do `location ~ ^/api/(pull...)` (403 antes da autenticação) e do `location /` (401 sem Bearer). | Confirmado no `ollama.conf`. |
| N16 | Os limites de SSE (4 por IP, 32 total) e de login (5, 15 min, 24 h, 2 verificações) são **constantes** do código, sem flag. | Mudar exige novo binário. |


---

## 16. Pool de backends e secundário (Fase 10.4)

**Em uma frase.** O gateway continua com **uma fila só**; cada geração que **ainda não começou** pode rodar no **primário**
(`gpu-local` = `--ollama`) ou, se o primário estiver sobrecarregado ou doente, num **Ollama secundário** da LAN que serve o
**mesmo modelo com o mesmo digest**. Nada muda de lugar depois que o pedido começa.

**Sem `--backends` o pool é um backend só (`gpu-local`) e tudo se comporta como antes da 10.4.** O monitor mostra uma linha
no quadro Backends e o log de jobs diz `gpu-local`. Isso é verificado por teste (a estrutura implícita e um pool explícito de 1
produzem a mesma fila passo a passo; e a suíte antiga da fila, do proxy, do monitor e do log de jobs roda sem alteração).

### 16.1 O arquivo `backends.json`

Passado em `--backends` (ou `MCPGW_BACKENDS`). **Modo `0600`** (dono `mcpgw`); com `0640`/`0644` a partida é **recusada** (código
2). É lido **só na partida**: para mudar, edite e reinicie o gateway. **É a única fonte das URLs**: nenhum pedido, cabeçalho
ou ação do monitor pode criar ou mudar um backend (nada de SSRF pelo cliente). Aceita no máximo 8 backends.

```json
{
  "backends": [
    {
      "id": "sec-4x2060",
      "url": "https://10.100.2.60:8443",
      "tipo": "remoto",
      "token_file": "/etc/mcpollama-gateway/sec-4x2060.token",
      "tls_ca": "/etc/mcpollama-gateway/cloudflare-origin-ca.pem",
      "slots": 2,
      "modelos": ["qwen3-ia"],
      "classes": ["alta", "normal", "baixa"],
      "max_prompt_tokens": 16000,
      "ordem": 1
    }
  ]
}
```

| Campo | Obrigatório | O que é |
| --- | --- | --- |
| `id` | sim | `a-z 0-9 . _ -`, até 32, único. **`gpu-local` é reservado** (o primário). Aparece no monitor, no SQLite e nos eventos |
| `url` | sim | `http(s)://host[:porta]`, sem usuário, caminho, query nem fragmento; porta entre 1 e 65535 (revisão 10.5). Recusados: esquemas que não sejam http/https, `0.0.0.0` e endereços de link-local/multicast (onde moram os serviços de metadados de nuvem). O arquivo é lido **uma vez**, na partida: trocá-lo depois não muda nada até reiniciar |
| `tipo` | sim | `gpu`, `cpu` ou `remoto`. Entra nas regras: **`alta` nunca vai para `cpu`**, e `cpu` aceita por padrão só `normal` e `baixa` |
| `token_file` | não | Arquivo (modo `0600`) com o **Bearer próprio do secundário**, uma linha, ASCII visível. Só com `https://` (ou loopback): o token não vai em texto puro pela rede. **Nunca** aparece em log, evento, banco, monitor ou erro; se o secundário o ecoar numa resposta (versão, nome do modelo), o gateway o troca por `[redigido]` antes do monitor (revisão 10.5). O modo e o "arquivo comum" são conferidos de novo no arquivo **aberto** (um arquivo trocado entre a checagem e a leitura é recusado). O **dono** do arquivo não é conferido (igual ao `--admin-file`) |
| `tls_ca` | não | PEM da CA do secundário (só com `https://`). Usa o pool do sistema **mais** essa CA (o mesmo código do cliente mcpollama) |
| `slots` | não (1) | Gerações simultâneas nesse backend (1 a 64) |
| `modelos` | não | Modelos que ele serve (`:latest` é ignorado). Vazio = qualquer modelo cujo digest bata com o do primário |
| `classes` | não | Prioridades aceitas (`alta`, `normal`, `baixa`). Vazio = todas (`gpu`/`remoto`) ou `normal`+`baixa` (`cpu`) |
| `max_prompt_tokens` | não (sem limite) | Teto do prompt nesse backend. O gateway **estima** os tokens como `bytes do corpo ÷ 3` (pessimista para português) |
| `ordem` | não (0) | Preferência no repasse: menor primeiro; empate pelo `id` |

### 16.2 Como acrescentar um secundário

1. **No secundário:** instale o Ollama e traga **o mesmo modelo, com o mesmo digest** do primário
   (`ollama pull`/cópia do mesmo blob). Compare: `curl -s http://127.0.0.1:11434/api/tags | python3 -m json.tool | grep -A3 qwen3-ia`
   nos dois; o campo `digest` tem de ser **igual**. Digest diferente = o secundário **nunca** recebe o modelo (regra F4),
   mesmo que o nome seja igual.
2. **Rede e TLS:** o secundário só na LAN da CSL, atrás de nginx com TLS e Bearer (como o primário). Pegue a CA (PEM) se o
   certificado não for de uma CA do sistema.
3. **No servidor do gateway (root):**
   ```bash
   install -d -m 0750 -o root -g mcpgw /etc/mcpollama-gateway
   printf '%s\n' '<TOKEN DO SECUNDARIO>' | install -m 0600 -o mcpgw -g mcpgw /dev/stdin /etc/mcpollama-gateway/sec-4x2060.token
   install -m 0600 -o mcpgw -g mcpgw /dev/null /etc/mcpollama-gateway/backends.json
   nano /etc/mcpollama-gateway/backends.json        # o JSON da seção 16.1
   ```
   Modo e dono: **`mcpgw:mcpgw 0600`** nos dois arquivos (o gateway roda como `mcpgw`).
4. **Na unit:** acrescente `--backends /etc/mcpollama-gateway/backends.json` ao `ExecStart` (ou
   `Environment=MCPGW_BACKENDS=...`, se a flag não estiver no `ExecStart`), `systemctl daemon-reload` e
   `systemctl restart mcpollama-gateway`. O fila é esvaziada no reinício (aviso aos clientes).
5. **Conferir:** `journalctl -u mcpollama-gateway -n 20` deve dizer `backend secundário sec-4x2060 (remoto) em <host>, 2 slot(s), com token próprio`;
   o `/monitor` mostra o quadro **Backends** com duas linhas e o digest do modelo nas duas. Se a linha do secundário estiver em
   `fora`, veja o "Último erro" (passe o mouse).
6. **Teste sem risco:** **Drene** o primário no monitor (logado) e mande um pedido: ele deve rodar no secundário
   (`backend=sec-4x2060`, `failover_rule=F2:S3`, no log de jobs). **Retome** o primário em seguida.
7. **Rollback:** tire a flag do `ExecStart` e reinicie: volta ao `gpu-local` só. Nada no banco precisa ser desfeito.

### 16.3 Saúde e disjuntor

- **Checagem ativa** a cada `--health-interval` (5 s): `GET /api/ps` e `/api/tags` (sempre: no primário essa leitura é também
  o quadro **Modelos**, que mostra o erro com o Ollama fora) e, se elas responderem, `/api/version` (revisão 10.5, N1). Um
  backend mudo custa 2 × 3 s por rodada. Registra a versão, o
  modelo **residente**, **onde** ele está (`100% GPU` ou dividido com a CPU) e os digests. Um backend `gpu`/`remoto` com o modelo
  fora de 100% GPU fica `degradado` (e, no primário, conta como S3). *Modelo não residente não é falha*: o Ollama descarrega
  por ociosidade e a carga a frio (16 a 22 s) é normal; só aparece como informação.
- **Checagem passiva (revisão de segurança, MEDIUM):** o resultado de cada pedido. Conta como falha **só** a falha de
  **conexão com o backend** (`upstream_unreachable`: recusa, dial, TLS; o proxy nunca a grava quando foi o cliente que desistiu).
  **5xx, conexão cortada (`upstream_dropped`), timeout, erro no meio do stream, 4xx e desistência do cliente são NEUTROS**:
  dependem do pedido ou do cliente (um pedido malformado faz o Ollama devolver 500; um cliente pode derrubar a própria
  conexão), e se contassem **um cliente abriria o disjuntor do primário para todos e forçaria o repasse** (DoS). **Revisão
  10.5:** um erro do proxy **depois** de a conexão ser feita (o backend fecha antes de responder) também é neutro, porque um
  corpo de pedido pode provocá-lo; o registro continua `upstream_unreachable`. Um **3xx** de um secundário vira **502
  `upstream_redirect`** (neutro), sem `Location` nem `Set-Cookie` para o cliente. Quem abre o
  disjuntor é o que nenhum cliente controla: a falha de conexão e a **checagem ativa**. Testado: 50 pedidos de um cliente que geram 500
  e 100 desistências no meio do stream deixam o disjuntor **fechado**; 3 recusas de conexão ou 3 checagens ativas falhas abrem.
- **Disjuntor** (por backend): `--breaker-failures` (3) falhas seguidas, ativas ou passivas → **aberto** por `--breaker-open`
  (30 s; o backend não recebe início nenhum) → **meio-aberto**: deixa passar **um** pedido de prova; se der certo, **fechado**;
  se falhar, aberto de novo. **A checagem ativa que dá certo depois do tempo aberto também fecha** (revisão 10.5, B1: sem isso
  um secundário meio-aberto, que não é elegível, nunca recebia a prova e ficava fora do pool até reiniciar). Sucesso de um
  pedido zera a contagem. Eventos `breaker_open` e `breaker_close`.
- Eventos `backend_down` (a checagem ativa falhou) e `backend_up` (voltou).
- Com **só o primário** (sem secundário) o disjuntor é contado e mostrado, mas **não segura** os pedidos: o gateway segue
  mandando ao único Ollama, como sempre.

### 16.4 Sobrecarga do primário e repasse

O primário está **sobrecarregado** quando qualquer um vale (só avaliado com secundário):

| Gatilho | Condição |
| --- | --- |
| **S1** | espera estimada de um pedido novo > `--overload-wait` (60 s). Estimativa = EWMA (α = 0,2) da duração das gerações que terminaram bem no primário × (esperando à frente + executando no primário) ÷ slots do primário, **sem contar o próprio pedido** (esperando à frente = esperando − 1); 0 enquanto o pedido couber num slot livre, ou sem amostras. Exemplo: 1 slot, EWMA 40 s, primário livre e 1 pedido chegando: espera 0; primário ocupado e ninguém à frente: 40 s; 2 à frente e 1 executando: 120 s |
| **S2** | mais de `--overload-queue` (75% de `--queue-max`) gerações **na fila de verdade**: as que esperam e não cabem nos slots livres do primário (esperando − slots livres) |
| **S3** | primário sem saúde: checagem ativa falhando, modelo fora de 100% GPU, disjuntor aberto ou meio-aberto, ou drenado |
| **S4** | energia: **só o gancho**, desligado (`EnergyHook` nulo). Ligado, seguraria novos inícios no primário e repassaria |

Quando o primário **tem slot livre e está bom**, o pedido roda nele (F1). Quando não: se há sobrecarga, o gateway procura o
primeiro secundário (por `ordem`) **com slot livre** e **elegível** para algum pedido que espera, e dá a ele o pedido que a fila
serviria a seguir **entre os elegíveis** (justiça e limites por IP continuam os da fila única: F8). **Elegível** (F2–F4):
sem drenar e saudável; `modelos` inclui o modelo (se informado); a **classe** de prioridade é aceita (`alta` nunca em `cpu`); o
prompt cabe em `max_prompt_tokens`; e o **digest do modelo no secundário é igual ao do primário** (os dois lidos do `/api/tags`; o do
primário é o último conhecido, então vale mesmo com o primário fora). Sem digest conhecido dos dois lados, **não** é elegível.

- **F5:** sem secundário elegível o pedido **continua na fila** (nada de erro), até `--max-wait`.
- **F6 (volta):** a sobrecarga só desliga depois de `--overload-hold` (30 s) com a carga **abaixo de 50%** dos dois limites e o
  primário saudável; um pico no meio reinicia a contagem. Eventos `overload_on` e `overload_off`. Os que já estão no secundário
  terminam lá.
- **F7:** **não há repasse durante a execução.** Se o backend cair no meio, o cliente recebe o erro; o mcpollama pode refazer (seção 16.7).
- Um primário **doente** (S3) cede a vez ao secundário elegível; se não houver, o pedido ainda vai ao primário (como hoje). Com o disjuntor
  do primário **aberto**, nada começa nele; o primeiro pedido depois dos 30 s é a prova.
- **Embeddings rodam só no primário** (o espaço de vetores não pode mudar de um backend para outro).
- O `--start-stagger` vale só para os inícios no **primário** (protege a fonte dele); um secundário é outra máquina.
- O job registra **onde** rodou (`requests.backend`) e **por quê** (`requests.failover_rule`).

### 16.5 Drenar e retomar (manutenção)

No `/monitor`, logado, o botão **Drenar** de um backend (ou `POST /monitor/api/backends/{id}/drain`): ele não recebe início novo,
o que roda termina, e o estado fica `drenado`. **Retomar** (`.../undrain`) desfaz. Só administrador, com CSRF e mesma origem; cada
ação grava auditoria (`drain`/`undrain`) e um evento (`backend_drenado`/`backend_retomado`). **Drenar o primário** manda o trabalho
ao secundário elegível (S3); sem secundário elegível os pedidos esperam. O estado de drenar fica **em memória**: reiniciar o gateway
volta tudo ao normal.

### 16.6 Eventos e consultas

Eventos novos: `overload_on`, `overload_off`, `backend_down`, `backend_up`, `breaker_open`, `breaker_close`, `backend_drenado`,
`backend_retomado` e `gateway_pool` (ids e limites, na partida).

```bash
DB=/var/lib/mcpollama-gateway/gateway.db
# onde rodaram os jobs de hoje e por que regra
sqlite3 -readonly "$DB" "SELECT backend, COALESCE(failover_rule,'-'), count(*) FROM requests WHERE enqueued_at > (strftime('%s','now','start of day')*1000) GROUP BY 1,2;"
# o quadro do pool na última hora (estado, slots usados, fila, p50)
sqlite3 -readonly "$DB" "SELECT datetime(ts/1000,'unixepoch','localtime'), backend_id, estado, slots_usados, fila, p50_ms FROM backend_samples WHERE ts > (strftime('%s','now','-1 hour')*1000) ORDER BY ts DESC LIMIT 60;"
# sobrecargas, quedas e disjuntores
sqlite3 -readonly "$DB" "SELECT datetime(ts/1000,'unixepoch','localtime'), kind, detail FROM events WHERE kind IN ('overload_on','overload_off','backend_down','backend_up','breaker_open','breaker_close','backend_drenado','backend_retomado') ORDER BY ts DESC LIMIT 30;"
```

### 16.7 Fallback do mcpollama (cliente)

Para o caso em que o **servidor inteiro** cai (o gateway cai junto e não consegue repassar), o mcpollama tem um fallback no
cliente, **desligado por padrão**:

| Flag | Env | O que faz |
| --- | --- | --- |
| `--ollama-url-fallback URL` | `OLLAMA_HOST_FALLBACK` | Um segundo servidor (outro gateway ou um Ollama da LAN). Com erro de **conexão** ou **503 de fila cheia** (com `Retry-After`) do primário, o mcpollama tenta o fallback **uma vez**, sem esperar o `Retry-After` do primário. A linha `[uso]` ganha `backend=fallback` (e o `_meta` também). Timeout, 4xx, 5xx e cortes no meio **não** disparam o fallback. "Erro de conexão" é só o que acontece **antes** de o pedido sair (dial, DNS, recusa, proxy): uma resposta malformada ou um GOAWAY do HTTP/2 chegam depois de o primário ler o pedido e **não** disparam (revisão 10.5, FB1: senão o trabalho rodava duas vezes) |
| `--fallback-api-key TOKEN` | `OLLAMA_FALLBACK_API_KEY` | O Bearer **do fallback**; o token do primário **nunca** vai para ele. Redigido em log e erro como o do primário. Só com `https://` (ou loopback) |
| `--fallback-tls-ca FILE` | `OLLAMA_FALLBACK_TLS_CA` | CA (PEM) do fallback, somada ao pool do sistema. O `--insecure` do primário **não** vale para o fallback |

Sem essas flags nada muda. Se o fallback também falhar, vale o erro do primário. As descrições das tools não mudaram.

### 16.8 Limites conhecidos

- O estado de **drenar**, a EWMA e o disjuntor ficam em memória (reiniciar zera).
- A estimativa de tokens é por bytes (÷ 3): uma boa aproximação, não o tokenizador do modelo. **Manipulável** (revisão 10.5,
  R5): um prompt de dígitos (o Qwen faz 1 token por dígito) tem ~3× a estimativa, então um `max_prompt_tokens` de 1.000 aceita
  ~3.000 tokens reais. O efeito é um prefill mais longo naquele backend, limitado pelo `--run-timeout`. A cota segura é bytes ÷ 1
  (decisão do owner, ver o plano).
- O digest é **declarado pelo próprio secundário** (`/api/tags`): é uma checagem contra engano de configuração, não uma
  fronteira de segurança. Um secundário mal-intencionado pode declarar o digest certo e servir outro modelo; a proteção é só
  confiar no host do `backends.json` (root, `0600`), com TLS e token próprio.
- O monitor aberto mostra o `host:porta` de cada secundário (e ele aparece no "último erro"). É informação da LAN, sem
  credencial; se o owner preferir, dá para mostrar só com login (pendência da 10.5).
- Digest igual não prova tempo igual: um secundário com outra GPU responde mais devagar; `max_prompt_tokens` e `classes` são os botões.
- O gateway roda **no servidor primário**: se ele cair, não repassa. Para isso existe o fallback do cliente.
- **Revisão adversarial do pool (10.5) feita** em 2026-10-30 (report `MCPOllam2026-10-30-190200-revisao-adversarial-10-5-pool`).
  O aceite numa instância de teste (10.7) foi feito, com todos os cenários PASS; o plano da Fase 10 está congelado (2026-10-30) e lista as pendências sem data.
