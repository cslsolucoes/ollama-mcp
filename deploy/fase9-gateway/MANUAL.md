# Manual de operação do `mcpollama-gateway` (Fase 9)

Para quem opera o servidor de IA da CSL (`10.100.2.2`, `lx-ai01`) por SSH e usa o `/monitor` no navegador.
Escrito a partir do código (`src/cmd/mcpollama-gateway`, `src/gateway/*`), da pasta `deploy/fase9-gateway/`, do plano
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

---

## 1. O que é e por que existe

O Ollama processa **1 pedido por vez** com o `qwen3-ia` (= `qwen3.5:9b`) e tem uma fila interna invisível
(`OLLAMA_MAX_QUEUE`). Com vários agentes ao mesmo tempo, ninguém via quem esperava nem quanto. O gateway é uma **fila
visível, justa e auditável** posta na frente do Ollama, com uma página (`/monitor`) para ver e mexer nela.

```
cliente (mcpollama, ia-local/OpenClaude, curl)
   │  HTTPS :8443, Authorization: Bearer <TOKEN>   (LAN 10.100.2.0/24 ou Cloudflare Tunnel)
   ▼
nginx :8443   TLS, Bearer, 600 pedidos/min por IP (rajada 60), 12 simultâneos por IP,
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
| **mcpollama** (MCP) | Manda `X-Mcpollama-Client` (rótulo, `--client-label`, padrão `<hostname>/<pid>`) e `X-Mcpollama-Tool`. Lê `X-Queue-Ms` e mostra `queue_ms` na linha `[uso]`. Em **503 com `Retry-After`** repete **uma vez** (só se sobrar tempo no `--timeout`). Em **409** (administrador removeu/cancelou) não repete. Não manda prioridade nem `think` (usa o padrão do gateway). |
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
cd E:\MCPOllam\deploy\fase9-gateway
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
cd /e/MCPOllam/deploy/fase9-gateway
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
`2160h`. Fonte: `src/cmd/mcpollama-gateway/config.go` (31 flags). **A unit de produção** é `mcpollama-gateway.service`.

> Na unit, as flags vão em `ExecStart`. Como a flag vence a variável de ambiente, definir `Environment=MCPGW_X=...`
> **não** muda uma flag que já está no `ExecStart`.

| Flag | Env | Padrão | O que faz | Unit de produção |
| --- | --- | --- | --- | --- |
| `--listen ADDR` | `MCPGW_LISTEN` | `127.0.0.1:11500` | Endereço de escuta. Fora do loopback a partida é recusada (confia no nginx para o IP real) | `127.0.0.1:11500` |
| `--ollama HOST:PORT\|URL` | `MCPGW_OLLAMA` | `127.0.0.1:11434` | O Ollama de destino (só `http://`) | `127.0.0.1:11434` |
| `--db FILE` | `MCPGW_DB` | `/var/lib/mcpollama-gateway/gateway.db` | Arquivo SQLite, criado `0600` | `/var/lib/mcpollama-gateway/gateway.db` |
| `--gen-slots N` | `MCPGW_GEN_SLOTS` | `2` | Gerações simultâneas (`/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions`) | **`1`** (o Ollama roda `qwen3.5:9b` com 1 slot) |
| `--embed-slots N` | `MCPGW_EMBED_SLOTS` | `2` | Embeddings simultâneos | `2` |
| `--queue-max N` | `MCPGW_QUEUE_MAX` | `64` | Pedidos esperando, todos os clientes e as duas faixas juntas. Cheio: 503 | padrão (64) |
| `--queue-per-client N` | `MCPGW_QUEUE_PER_CLIENT` | `16` | Esperando por IP + rótulo | padrão (16) |
| `--queue-per-ip N` | `MCPGW_QUEUE_PER_IP` | = `--queue-per-client` | Esperando por IP, todos os rótulos juntos | padrão (16) |
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
| `--gpu-interval D` | `MCPGW_GPU_INTERVAL` | `5s` | Período do `nvidia-smi`. `0` desliga o coletor | padrão (5s) |
| `--nvidia-smi PATH` | `MCPGW_NVIDIA_SMI` | `nvidia-smi` | Binário do `nvidia-smi` | padrão |
| `--aer-path FILE` | `MCPGW_AER_PATH` | `/sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable` | Contador AER corrigível da ponte `00:1b.4`; `-` desliga | padrão |
| `--retention-requests D` | `MCPGW_RETENTION_REQUESTS` | `2160h` (90 dias) | Quanto tempo ficam os pedidos no banco | padrão |
| `--retention-gpu D` | `MCPGW_RETENTION_GPU` | `168h` (7 dias) | Quanto tempo ficam as amostras de GPU | padrão |
| `--max-request-rows N` | `MCPGW_MAX_REQUEST_ROWS` | `3000000` | Teto de linhas de `requests` (mínimo 1000); as mais antigas saem primeiro, de hora em hora | padrão |
| `--non-loopback-listen` | `MCPGW_NON_LOOPBACK_LISTEN` | `false` | Permite `--listen` fora do loopback (não recomendado) | não usado |
| `--debug` | `MCPGW_DEBUG` | `false` | Uma linha de log por pedido (`req <id> <faixa> <rota> status=... class=... wait_ms=...`), nunca com texto | não usado |
| `--version` | (sem env) | — | Imprime a versão e sai | — |

**Validações da partida** (erro = sair com código 2): slots, `--queue-*` e `--labels-per-ip` ≥ 1; durações > 0
(`--start-stagger` e `--gpu-interval` podem ser 0); `--max-body-bytes` > 0; retenções > 0 e `--max-request-rows` ≥ 1000;
`--listen` no loopback; `--db` não vazio; `--admin-file` com permissão `0600` (ver Nota N1).

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
| Fila total cheia (`--queue-max`, 64) | **503** `fila do servidor cheia (fila cheia); tente em 5 s` + `Retry-After: 5` | pedido nunca rodou; registrado `rejeitado` |
| IP com 16 esperando (`--queue-per-ip`) ou IP+rótulo com 16 (`--queue-per-client`) | **503** `... (limite de pedidos em espera deste cliente) ...` + `Retry-After: 5` | idem |
| Esperou mais que `--max-wait` (10 min) | **504** `tempo máximo de espera na fila esgotado` | registrado `expirado` |
| Rodando há mais que `--run-timeout` (10 min) | antes dos headers: **504** `tempo máximo de execução no servidor esgotado`; no meio do stream: linha final `{"error":"tempo máximo de execução no servidor esgotado"}` | registrado `erro` / `run_timeout`; o slot volta |
| Administrador **removeu** da fila | **409** `removido da fila pelo administrador` | registrado `removido` |
| Administrador **cancelou** em execução | antes dos headers: **409** `cancelado pelo administrador`; no meio do stream: linha final `{"error":"cancelado pelo administrador"}` | registrado `cancelado_admin` |
| Cliente desistiu (fechou a conexão) | nada (ninguém para ler); sai da fila | registrado `cancelado` / `client_gone`, HTTP 499 no registro |
| Ollama fora ou conexão recusada | **502** `Ollama indisponível` (sem `Retry-After`) | `upstream_unreachable` |

**Escalonamento de partida** (`--start-stagger`, 300 ms): entre o início de duas gerações passam no mínimo 300 ms. Com 1
slot de geração o efeito é pequeno; vale para quando houver mais slots (os prefills não começam no mesmo milissegundo, o
que atenua os picos de energia que derrubam a fonte de 850 W).

**Camadas de limite** (do cliente para dentro): nginx 600 pedidos/min por IP (rajada 60) e **12 simultâneos** por IP
(excedeu → **429 do nginx**, página HTML, não do gateway) → gateway (limites acima, **503 JSON**) → Ollama. Na prática o
teto de 12 do nginx é atingido antes do teto de 16 esperando por IP do gateway.

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

**Certificado (Cloudflare Origin CA).** O Windows não confia nessa CA, então o navegador avisa. Importe a CA uma vez em
cada estação de administração (PowerShell **como administrador**; o arquivo é o que o `ia-local` já usa):

```powershell
certutil -addstore -f Root "$env:USERPROFILE\.openclaude\certs\cloudflare-origin-ca.pem"
```

Para só o seu usuário (sem administrador): `certutil -user -addstore Root "<arquivo>"`. Edge e Chrome usam a loja do
Windows; o Firefox tem loja própria (importe lá ou ative `security.enterprise_roots.enabled`). Reinicie o navegador.

**O que a página mostra** (atualiza ao vivo por SSE, com *polling* de 2 s se o SSE falhar):

| Quadro | Conteúdo |
| --- | --- |
| Cabeçalho | `versão · boot (8 primeiros caracteres) · ativo há · atualizado`; à direita, login |
| Cartões | Em execução; Na fila; Concluídos hoje (status `ok` desde 00:00 do servidor); Erros hoje (status `erro`); Espera p50 e p95 (15 min); **Tokens/s** (tokens gerados nos últimos 15 min ÷ 900 s, é média e não instantâneo); Pedidos/min (15 min) |
| Em execução | Faixa, id (8 caracteres), cliente (passe o mouse: IP + rótulo), tool, modelo, raciocínio, prioridade concedida, rodando há, esperou; botão **Cancelar** (logado) |
| Fila de espera | Posição (a ordem em que vão começar, simulada), faixa, id, cliente, tool, modelo, raciocínio, prioridade atual (já com envelhecimento), esperando há; botões **▲ ▼ ✕** (logado) |
| GPUs | Por GPU: potência (W), memória (MiB), uso (%), temperatura; linha `AER 00:1b.4 (corrigíveis): N`; "registros descartados: N" se o buffer do banco encheu |
| Última hora | Três gráficos: pedidos na fila, vazão (pedidos/min), espera média (ms). **Ficam em memória**: reiniciar o gateway zera |
| Concluídos recentes | Últimos 50 (qualquer status: `ok`, `erro`, `cancelado`, `rejeitado`, `expirado`...), com HTTP, espera, execução, tokens (prompt / gerados) e classe do erro |
| Auditoria | Últimas 20 ações do administrador |
| Eventos | Últimos 10: `gateway_inicio`, `gateway_config`, `gateway_parada`, `boot_novo`, `registros_descartados`, `teto_pedidos` |

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

Ação sobre um pedido que já saiu da fila (começou, terminou ou o cliente desistiu): a página mostra
`não está mais na fila` / `não está mais em execução` (**409**). Toda ação válida grava na **auditoria**: data/hora,
usuário, IP, ação, id do pedido, estado antes e depois e o motivo (até 200 caracteres). A auditoria **nunca expira**.

**Limites do login e do painel:**
- **5 falhas por IP em 15 min** → bloqueio de 15 min (**429** + `Retry-After`); cada novo bloqueio dobra (30 min, 1 h... até 24 h); um dia sem atividade zera. Acertar a senha limpa o IP. Não há bloqueio global: falhas de outros IPs não trancam o administrador.
- No máximo **2 verificações de senha** ao mesmo tempo (PBKDF2 pesa); se ocupado por 10 s: 503.
- **SSE: 4 painéis por IP e 32 no total**; acima disso: 503 `muitos painéis abertos`. Fechar abas antigas resolve.
- Os GET do monitor são abertos a quem alcança a LAN; eles mostram IPs, rótulos, modelos e contadores, **nunca** prompts ou respostas.

---

## 7. API do monitor

Base: `https://ia.cslsolucoes.com.br:8443/monitor/api`. Só da LAN. Todo corpo é JSON UTF-8. Erros: `{"error":"..."}`.
Os GET não têm efeito colateral. Todo POST exige **cookie de sessão + header `X-CSRF-Token`** (o token vem do login ou
de `/session`) e `Content-Type: application/json` quando há corpo; um POST de outra origem (`Origin` ou `Sec-Fetch-Site`
diferentes) dá 403.

| Método | Rota | Parâmetros | Resposta (resumo) |
| --- | --- | --- | --- |
| GET | `/monitor` e `/monitor/` | — | A página HTML (CSP restritiva, `no-store`) |
| GET | `/api/state` | — | Foto: `{now, version, boot_id, uptime_s, queue:{running[], waiting[], slots, in_use, totals}, gpu:{samples[], aer_corr_1b4, error?}, cards, recent[], events[], dropped, admin_enabled}` |
| GET | `/api/stream` | — | SSE: a cada mudança (agrupadas em 150 ms) e a cada 2 s, uma linha `data: <o mesmo JSON de /state>`. 503 + `Retry-After: 5` acima de 4 por IP / 32 total |
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

**Arquivo:** `/var/lib/mcpollama-gateway/gateway.db` (+ `-wal` e `-shm`), dono `mcpgw`, modo `0600`, modo WAL. A gravação é
assíncrona e em lote (nunca atrasa a fila); se o buffer de 4096 itens enche, o item é descartado e contado (evento
`registros_descartados`, no máximo 1 por minuto). Os tempos são **epoch em milissegundos**.

| Tabela | Colunas |
| --- | --- |
| `requests` | `id` (PK), `client_ip`, `client_label`, `tool`, `endpoint`, `model`, `lane`, `priority_in`, `priority_final`, `bytes_in`, `enqueued_at`, `started_at` (0 se nunca rodou), `finished_at`, `wait_ms`, `run_ms`, `status`, `http_status`, `error_class`, `prompt_eval_count`, `eval_count`, `load_ms`, `boot_id`, `think` |
| `audit` | `id`, `ts`, `usuario`, `ip`, `acao`, `request_id`, `antes`, `depois`, `motivo` |
| `gpu_samples` | `ts`, `gpu`, `bus_id`, `power_w`, `mem_used_mib`, `util_pct`, `temp_c`, `aer_corr_1b4`, `boot_id` |
| `events` | `ts`, `kind`, `detail` |
| `meta` | `k`, `v` (`boot_id`, `last_prune`, `last_vacuum`) |

Valores de `status`: `ok`, `erro`, `cancelado` (cliente desistiu), `cancelado_admin`, `removido`, `rejeitado` (fila cheia),
`expirado` (`--max-wait`). Valores de `error_class` (texto **fixo** do gateway): `upstream_<código>`, `stream_error`,
`run_timeout`, `client_gone`, `upstream_unreachable`, `upstream_dropped`, `admin_cancel`, `removido`, `expirado`, e para
rejeitados `fila cheia` ou `limite de pedidos em espera deste cliente`. `think`: `on`, `off`, `modelo` ou vazio.

**Retenção** (rodada de manutenção de hora em hora; a limpeza por idade, no máximo uma vez a cada 24 h):

| Dado | Regra |
| --- | --- |
| `requests` | 90 dias (`--retention-requests`) **e** teto de 3.000.000 linhas (`--max-request-rows`), as mais antigas saem primeiro (evento `teto_pedidos`) |
| `gpu_samples` | 7 dias (`--retention-gpu`) |
| `audit` e `events` | **nunca expiram** |
| `VACUUM` | semanal |

**Garantia de privacidade:** o banco guarda **só metadados**. Não existe coluna para prompt, documento, resposta ou token.
O corpo é lido para aplicar as políticas, mas só contadores numéricos (do **último** pedaço da resposta) são mantidos; o
`error_class` nunca carrega texto do Ollama. Um teste com texto-sentinela confirma que ele não aparece no `.db`, no
`-wal`, no `-shm` nem no log, nem nos caminhos de erro (revisão 9.5 e teste da 9.6a).

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
`deploy/fase9-gateway/mcpollama-gateway.service` no repositório: senão, uma reinstalação (`Completo`) sobrescreve a edição.
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
| **429** (página HTML do nginx) | Mais de 12 pedidos simultâneos do mesmo IP, ou mais de 600/min (rajada 60) | Reduzir a concorrência do cliente; é o nginx, não a fila |
| **503** JSON `fila do servidor cheia...` + `Retry-After` | Fila do gateway cheia (64) ou cota por IP/rótulo (16) | Esperar o `Retry-After` (o mcpollama repete 1 vez); ver quem ocupa a fila no `/monitor`; diminuir a rajada ou remover pedidos presos |
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
- sem *request smuggling* (CL/TE duplicados) e sem acesso às 6 rotas administrativas, em 17 variantes de escrita; 0 de 17 chegou ao Ollama;
- o `Authorization` do cliente **não** é repassado ao Ollama; token, prompt e resposta não entram no log, no banco nem em `error_class` (teste de sentinela);
- os contadores vêm só do último pedaço da resposta; o texto do modelo não os engana;
- nenhum slot "vaza": Ollama que cai, cliente que desiste, ações simultâneas do administrador (`-race`, 1.800 pedidos = 1.800 registros);
- `--run-timeout` impede um stream sem fim de prender o slot; teto de linhas e retenção limitam o banco; o cálculo de estatísticas é limitado e com cache;
- `/monitor`: sem XSS (a página usa `textContent`, CSP com hash, `default-src 'none'`), CSRF (token, `Origin`/`Sec-Fetch-Site`, só JSON), cookie `HttpOnly; Secure; SameSite=Strict`, token de sessão novo a cada login, senha com PBKDF2-HMAC-SHA256 (600.000 iterações), comparação em tempo constante;
- login: 5 tentativas por IP com backoff exponencial, sem bloqueio global, e reserva atômica (40 tentativas paralelas gastam 5 verificações);
- SQL parametrizado (0 injeção nos filtros do `/history`);
- as etapas de instalação conferem hashes (pasta `0700` só do root, `SHA256SUMS`, binário conferido de novo), e o token nunca vai em linha de comando.

**Riscos residuais e pendências:**

1. ~~**Nota N1:** arquivo da senha `0640` x código que exige `0600`~~ — **resolvida** (scripts gravam `mcpgw:mcpgw 0600`; provado no servidor);
2. o **rótulo do cliente é livre** (só o IP é confiável); a justiça ancora no IP;
3. `--priority-ips 10.100.2.0/24` dá `alta` a **qualquer máquina da LAN com o token**; isso é decisão do owner (D9-8), e vale só como pedido (o mcpollama não usa);
4. **um único administrador** compartilhado; a auditoria registra o usuário, não a pessoa;
5. os GET do `/monitor` são abertos na LAN (IPs, rótulos, modelos, contadores; nunca conteúdo);
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

**O gateway de hoje** tem 2 faixas (geração e embedding), todas na **GPU**, para **um** Ollama (`127.0.0.1:11434`). Não faz
roteamento por prioridade entre GPU e CPU, nem repasse a outro servidor.

**PLANEJADO e NÃO implementado** (plano, §11 e §11.9; começa depois do aceite 9.7; nenhuma flag abaixo existe no
`config.go`):

- **Faixa CPU/RAM** (uma segunda instância `ollama-cpu` e roteamento por pedido): regras R1–R9 (embedding na CPU se aprovado; `alta` sempre GPU; `baixa` na CPU se o tempo estimado couber; `normal` transborda se a espera na GPU passar de 60 s; roubo de trabalho da fila da CPU pela GPU; CPU fora do ar = tudo na GPU), header `X-Mcpollama-Lane`, headers de resposta `X-Lane` e `X-Routing-Reason`, ações do monitor "mover para GPU/CPU".
- **Pool de backends / repasse a um Ollama secundário** na LAN em caso de sobrecarga (`--backends` com JSON de backends, gatilhos `--overload-wait` e `--overload-queue`, Bearer próprio de cada secundário).
- Etapas 9.9 a 9.16 do plano (medições antes de ligar qualquer coisa).

Hoje, para aguentar mais carga, os únicos botões são `--gen-slots` (inútil enquanto o Ollama atende 1 pedido por vez) e
cotas/limites da fila.

---

## 14. Referências

| O quê | Onde |
| --- | --- |
| Instalador | `E:\MCPOllam\deploy\fase9-gateway\Instalar-Gateway.ps1` |
| Pasta de implantação | `E:\MCPOllam\deploy\fase9-gateway\` (`README.md`, `mcpollama-gateway.service`, `ollama.conf`, `stage1.sh`, `stage2.sh`, `rollback.sh`, `mcpgw-definir-senha.sh`) |
| Flags e padrões | `E:\MCPOllam\src\cmd\mcpollama-gateway\config.go`, `main.go` |
| Fila, proxy, políticas | `E:\MCPOllam\src\gateway\queue\queue.go`, `proxy\proxy.go`, `proxy\policy.go` |
| Monitor, página, login | `E:\MCPOllam\src\gateway\monitor\monitor.go`, `admin.go`, `index.html` |
| Banco e GPU | `E:\MCPOllam\src\gateway\store\store.go`, `gpu\gpu.go` |
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
