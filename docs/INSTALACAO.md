# Guia de instalação do zero: mcpollama, servidor de IA e gateway

> Para o administrador da CSL que precisa montar tudo de novo (servidor novo ou depois de um desastre) e configurar as
> estações. Repositório privado: <https://github.com/cslsolucoes/ollama-mcp>. Visão geral do projeto: [README.md](../README.md).
>
> **Sem segredos neste guia.** Onde aparece `<TOKEN>`, `<SENHA>` ou `<IP>`, use o valor real do seu ambiente e **nunca**
> o grave no git, em chat ou em log. Os segredos de produção ficam só no servidor e nos cofres do owner. O acesso SSH
> ao servidor e os caminhos das chaves estão num **guia de operação privado do owner** (`CONEXAO-IA.md`), fora deste
> repositório; este guia não os repete.
>
> Marcações **conferir** indicam o que as fontes do repositório não permitem confirmar. Reúna-as antes de depender delas
> (lista consolidada no fim da seção H).

## Índice

- [A. Visão geral, ordem e hardware](#a-visão-geral-ordem-e-hardware)
- [B. Servidor de IA (Linux)](#b-servidor-de-ia-linux)
  - [B.1 Sistema operacional](#b1-sistema-operacional)
  - [B.2 Driver NVIDIA e persistence mode](#b2-driver-nvidia-e-persistence-mode)
  - [B.3 Limite de potência e clock: `nvidia-powercap.service`](#b3-limite-de-potência-e-clock-nvidia-powercapservice)
  - [B.4 Pré-requisito: Ollama já instalado](#b4-pré-requisito-ollama-já-instalado)
  - [B.5 Pré-aquecimento: `ollama-warm`](#b5-pré-aquecimento-ollama-warm)
  - [B.6 journald](#b6-journald)
  - [B.7 nginx (Linux), completo](#b7-nginx-linux-completo)
  - [B.8 Firewall](#b8-firewall)
  - [B.9 `cloudflared` (túnel)](#b9-cloudflared-túnel)
  - [B.10 NAT no roteador](#b10-nat-no-roteador)
- [C. Gateway de fila e `/monitor`](#c-gateway-de-fila-e-monitor)
- [D. Estação Windows (mcpollama)](#d-estação-windows-mcpollama)
- [E. Clientes MCP](#e-clientes-mcp)
- [F. Conferência final (checklist)](#f-conferência-final-checklist)
- [G. Atualizações e manutenção](#g-atualizações-e-manutenção)
- [H. Solução de problemas](#h-solução-de-problemas)

---

## A. Visão geral, ordem e hardware

### A.1 O que existe

| Peça | Onde roda | Papel |
| --- | --- | --- |
| **Ollama** `0.40.13` | servidor Linux, `127.0.0.1:11434` | serve o modelo `qwen3.8:27b` e os embeddings |
| **nginx** `1.26.3` | servidor, `:8443` (TLS) | única porta pública: TLS (Cloudflare Origin CA), Bearer, limites, bloqueio de rotas administrativas |
| **mcpollama-gateway** | servidor, `127.0.0.1:11500` | fila na retaguarda e o painel `/monitor` |
| **cloudflared** | servidor | túnel para a internet (sem abrir porta no roteador para o público) |
| **mcpollama-mcp** | estação Windows, `D:\MCP\mcpollama-mcp.exe` | servidor MCP (stdio) que o Claude Code, o Cursor e o VS Code chamam |
| **ia-local** (OpenClaude) | estação Windows | cliente OpenAI-compatível opcional, pelo `/v1` |

Caminho de uma chamada: cliente MCP -> `https://ia.cslsolucoes.com.br:8443` -> nginx -> gateway (`:11500`) -> Ollama (`:11434`).

### A.2 Ordem da instalação

1. **Servidor** (parte B): SO, driver NVIDIA, powercap, Ollama (pré-requisito), `ollama-warm`, journald, nginx, firewall, `cloudflared`, NAT.
2. **Gateway** (parte C): instalador no PC; muda o tráfego do nginx para o gateway.
3. **Estação** (parte D): Go, build do `mcpollama-mcp`, CA do Cloudflare, OCR opcional, smoke test.
4. **Clientes** (parte E): Claude Code, Cursor, VS Code, ia-local.
5. **Conferência final** (parte F).

### A.3 Hardware

O servidor atual: **4x NVIDIA GeForce RTX 3070 (8 GB cada, 32 GB de VRAM)**, **94 GB de RAM**, NVMe de ~238 GB, fonte **Cougar 850 W**.
Para reproduzir a configuração:

- GPUs NVIDIA com VRAM somada suficiente para o modelo a 64k de contexto: **cerca de 19,6 GB** para o `qwen3.8:27b` (27,3B denso, Q4_K_M,
  17 GB em disco, `OLLAMA_KV_CACHE_TYPE=q4_0`) mais o `nomic-embed-text` residente junto (≈ 24 GB no total dos 32 GB; com o
  `qwen3.5:9b`, usado até a Fase 12, eram ~13 GB). Na medição do bake-off (2026-11-04/05), o `qwen3.8:27b` ficou a 64k com 66/66
  camadas na GPU, na VRAM somada das 4 placas (`OLLAMA_SCHED_SPREAD=1` espalha as camadas). Implantação pendente do aceite de 2026-11-05.
- **A fonte precisa aguentar as placas.** Com 4 placas, a 850 W houve reboots secos (ver [B.3](#b3-limite-de-potência-e-clock-nvidia-powercapservice)
  e [H.1](#h1-reboots-do-servidor)). Se montar com fonte maior ou menos placas, os limites da B.3 podem ser revistos.
- Rede: IP fixo na LAN (`10.100.2.2` hoje) e DNS interno resolvendo `ia.cslsolucoes.com.br` para ele (split-horizon).

---

## B. Servidor de IA (Linux)

Tudo nesta parte é feito **como root no servidor**. Os arquivos de referência são cópias do servidor de produção
coletadas em 2026-10-30; estão em [`deploy/servidor-ia/`](../deploy/servidor-ia/README.md) (descrição de cada um no README da pasta).

Para enviá-los do PC (alias SSH do owner, `<ALIAS-SSH>`; **conferir** o alias no seu `~/.ssh/config`):

```powershell
scp E:\MCPOllam\deploy\servidor-ia\* <ALIAS-SSH>:/root/servidor-ia/
scp -r E:\MCPOllam\deploy\servidor-ia\nginx <ALIAS-SSH>:/root/servidor-ia/
scp E:\MCPOllam\deploy\gateway\ollama.conf <ALIAS-SSH>:/root/servidor-ia/
```

No servidor, garanta LF nos arquivos (o repositório usa LF, mas confira se o `scp` não os alterou):

```bash
sed -i 's/\r$//' /root/servidor-ia/*.service /root/servidor-ia/*.sh /root/servidor-ia/*.conf /root/servidor-ia/nginx/*
```

### B.1 Sistema operacional

Referência: **CentOS Stream 10** (kernel `6.12.0-278.el10.x86_64`). Qualquer EL10 equivalente serve.

```bash
cat /etc/os-release | grep PRETTY_NAME
uname -r
dnf -y update
```

Como conferir: `PRETTY_NAME="CentOS Stream 10 (Coughlan)"` (ou o equivalente). Reinicie se o kernel mudou.

### B.2 Driver NVIDIA e persistence mode

Versão de referência: driver **610.57.04** (CUDA 13.4). O método original de instalação (repositório da NVIDIA para EL10 ou
`.run`) **não está documentado nas fontes: conferir**. Instale o driver proprietário pelo método que a CSL já usa e reinicie.

```bash
nvidia-smi
nvidia-smi -L                       # anote os UUIDs das GPUs (usados na B.4)
systemctl enable --now nvidia-persistenced.service
systemctl is-active nvidia-persistenced
```

Como conferir:

- `nvidia-smi` lista as 4 placas e `Driver Version: 610.57.04`;
- `nvidia-smi -L` imprime 4 linhas `GPU n: NVIDIA GeForce RTX 3070 (UUID: GPU-...)`;
- `nvidia-persistenced` fica `active`.

O **persistence mode** (`nvidia-smi -pm 1`) é aplicado, por GPU, pela unit da B.3. O `nvidia-persistenced.service` é uma
dependência (`After=`/`Wants=`) dessa unit; ele vem com o pacote do driver (**conferir** o nome da unit no seu método de instalação).

### B.3 Limite de potência e clock: `nvidia-powercap.service`

Arquivo: [`deploy/servidor-ia/nvidia-powercap.service`](../deploy/servidor-ia/nvidia-powercap.service).

```bash
install -m 0644 /root/servidor-ia/nvidia-powercap.service /etc/systemd/system/nvidia-powercap.service.new
test -s /etc/systemd/system/nvidia-powercap.service.new          # NUNCA instale um arquivo vazio (veja o aviso)
mv -f /etc/systemd/system/nvidia-powercap.service.new /etc/systemd/system/nvidia-powercap.service && sync
systemctl daemon-reload
systemctl enable --now nvidia-powercap.service
```

O que a unit faz, para cada GPU (`for i in $(nvidia-smi --query-gpu=index ...)`):

| Comando | Efeito |
| --- | --- |
| `nvidia-smi -i $i -pm 1` | persistence mode ligado |
| `nvidia-smi -i $i -pl 100` | limite de potência de **100 W** (o mínimo aceito pela placa; faixa válida 100 a 240 W; padrão de fábrica 220 W) |
| `nvidia-smi -i $i -lgc 210,1400` | clock **travado** entre 210 e 1400 MHz |

`Type=oneshot` + `RemainAfterExit=yes` (fica "active" depois de rodar); `After=nvidia-persistenced.service`. A unit **não** depende de
`multi-user.target` de propósito: isso fechava um ciclo com o `ollama-warm` e o aquecimento do boot era descartado.

**Por que 100 W e clock travado.** A fonte de **850 W não aguenta as 4 placas**: houve vários reboots secos (sem Xid, sem log) na subida
de ocioso para carga. O power cap limita a média, não os picos de milissegundos das Ampere somados; por isso, além dos 100 W, o clock
foi travado em 210 a 1400 MHz (2026-10-30), o que cortou os picos (a soma das 4 GPUs sob carga caiu de mais de 350 W para ~200 W).
Os 100 W preservam os 32 GB de VRAM até a troca da fonte (decisão do owner).

**Aviso: arquivo de unit vazio vira "mascarado".** Um corte seco de energia já deixou a unit com 0 bytes e o systemd a tratou como
mascarada. Por isso: grave em `.new`, confira `test -s`, `mv` e `sync`, como acima.

**Como mudar quando houver a segunda fonte (ou uma fonte maior):**

1. Faça backup: `cp -p /etc/systemd/system/nvidia-powercap.service /root/nvidia-powercap.service.antes-$(date +%Y%m%d-%H%M)`.
2. Edite a linha `ExecStart`: suba o `-pl` (valores já validados antes do clock travado: 130 e 150 W; 150 W aguentou 10 min de carga máxima, 200 W e 170 W derrubaram) e/ou troque `-lgc 210,1400` por `-rgc` (destrava o clock).
3. `systemctl daemon-reload && systemctl restart nvidia-powercap.service`.
4. **Revalide por degraus** (suba um nível por vez, com carga real e o `/monitor` aberto) antes de deixar permanente.
5. Voltar atrás do clock: `nvidia-smi -rgc` e restaurar o backup da unit.

Como conferir:

```bash
systemctl is-active nvidia-powercap          # active
nvidia-smi --query-gpu=index,power.limit,clocks.max.gr,persistence_mode --format=csv,noheader
```

Esperado: 4 linhas com `100.00 W` e `persistence_mode` `Enabled`. Para o clock, `nvidia-smi -q -d CLOCK` mostra o intervalo travado (**conferir** o campo no seu driver).

### B.4 Pré-requisito: Ollama já instalado

Este guia **não** cobre a instalação do Ollama; ele deve estar instalado e funcionando antes de seguir. O que o projeto espera dele:

| Item | Esperado |
| --- | --- |
| Versão | `ollama version is 0.40.13` (`ollama --version`) |
| Binário e usuário | `/usr/local/bin/ollama`, serviço `ollama.service` como usuário `ollama` |
| Modelos | `qwen3.8:27b` (nome real, **sem alias**) e `nomic-embed-text` |
| Escuta | só `127.0.0.1:11434` (nada em `0.0.0.0`) |

**Modelos.** Desde a release mcpollama 0.10.1 / gateway 0.13.2 (Fase 12, 2026-11-05; implantação pendente do aceite de 2026-11-05) o modelo de
geração é o **`qwen3.8:27b` com o nome real, sem alias** (o nome `qwen3-ia` deixa de existir; antes ele era um alias do `qwen3.5:9b`, criado por `ollama cp`):

```bash
ollama pull qwen3.8:27b
ollama pull nomic-embed-text
ollama list
```

Esperado no `ollama list`: `qwen3.8:27b` (~17 GB em disco) e `nomic-embed-text:latest` (`0a109f422b47`, 274 MB).
**Nome real, sem alias:** o código, o gateway, a lista `--allowed-models` e os clientes usam o nome `qwen3.8:27b`, para que as amostras e as
comparações de cada modelo fiquem separadas pelo nome. Trocar de modelo passa a exigir ajustar o nome nesses pontos (ver [G.3](#g3-trocar-o-modelo)).
O `qwen3.8:27b` raciocina por padrão (não dá para desligar no Modelfile): o mcpollama e o gateway mandam `think:false`/`reasoning_effort:"none"`.
O nome virtual de raciocínio é `qwen3.8:27b-think`. Contas e contagens: pedir passo a passo ou ligar `think`.

**Variáveis do drop-in** [`ollama-10-performance.conf`](../deploy/servidor-ia/ollama-10-performance.conf) que importam para o projeto
(destino: `/etc/systemd/system/ollama.service.d/10-performance.conf`):

| Variável | Valor | Por que importa |
| --- | --- | --- |
| `OLLAMA_HOST` | `127.0.0.1:11434` | só o loopback: todo acesso passa por nginx e gateway (D9-2) |
| `OLLAMA_MAX_QUEUE` | `32` | fila do Ollama (era 8 e recusava com 503 o 10º pedido simultâneo) |
| `OLLAMA_NUM_PARALLEL` | `1` | essa arquitetura roda com 1 slot; a fila de verdade é a do gateway |
| `OLLAMA_CONTEXT_LENGTH` | `65536` | 64k; 4 GPUs com 131072 derrubaram a máquina |
| `OLLAMA_KEEP_ALIVE` | `-1` | modelo residente (carga a frio do `qwen3.8:27b` custa ~33 s; com o `qwen3.5:9b` era ~16 a 22 s) |
| `OLLAMA_MAX_LOADED_MODELS` | `2` | `qwen3.8:27b` + `nomic-embed-text` juntos; um `embed` não descarrega o modelo de geração |

As demais linhas do arquivo (`OLLAMA_KV_CACHE_TYPE=q4_0`, `OLLAMA_FLASH_ATTENTION=1`, `OLLAMA_SCHED_SPREAD=1`,
`CUDA_VISIBLE_DEVICES` por UUID, `OLLAMA_LOAD_TIMEOUT=15m`, `OLLAMA_NO_CLOUD=true`, `OLLAMA_VULKAN=false`, `OLLAMA_ORIGINS=*`,
`OLLAMA_DEBUG=0`) estão no arquivo de referência. **`CUDA_VISIBLE_DEVICES` leva os UUIDs das GPUs do servidor de origem**:
num servidor novo, troque pelos UUIDs de `nvidia-smi -L` (por UUID o índice não muda se a topologia mudar). Depois de editar:
`systemctl daemon-reload && systemctl restart ollama`.

A unit base [`ollama.service`](../deploy/servidor-ia/ollama.service) tem `Restart=always` e um `Environment="PATH=..."` com caminhos de
`/root/.opencode` herdados do servidor de origem; não são necessários para o Ollama (**conferir** antes de reaproveitar tal qual).

Como conferir:

```bash
ollama --version
systemctl is-active ollama
ss -tlnH | grep 11434        # só 127.0.0.1:11434
ollama ps                    # depois do ollama-warm (B.5): qwen3.8:27b e nomic, PROCESSOR 100% GPU
```

### B.5 Pré-aquecimento: `ollama-warm`

Arquivos: [`ollama-warm.service`](../deploy/servidor-ia/ollama-warm.service) e [`ollama-warm.sh`](../deploy/servidor-ia/ollama-warm.sh).

```bash
install -m 0755 /root/servidor-ia/ollama-warm.sh /usr/local/bin/ollama-warm
install -m 0644 /root/servidor-ia/ollama-warm.service /etc/systemd/system/ollama-warm.service
systemctl daemon-reload
systemctl enable --now ollama-warm.service
```

O script espera o Ollama responder em `/api/version` (até 60 tentativas de 2 s) e então faz um `/api/generate` mínimo
(`think:false`, `num_ctx:65536`, `num_predict:1`) para carregar o `qwen3.8:27b` a 64k na VRAM, de modo que o primeiro cliente não espere a carga a frio.

**Ordem no boot:** `nvidia-powercap` primeiro, depois `ollama`, depois `ollama-warm` (a unit declara `After=ollama.service nvidia-powercap.service` e
`Requires=ollama.service`). Carregar o modelo antes do power cap aplicado já coincidiu com um reboot (2026-10-30 16:41).

Como conferir: `systemctl is-active ollama-warm` (`active`, é oneshot) e `ollama ps`, com `qwen3.8:27b` em `100% GPU` e `CONTEXT 65536`.
O `nomic-embed-text` entra no primeiro `embed` (a etapa 2 do gateway também o aquece).

### B.6 journald

Para não perder os segundos antes de um corte seco de energia (o padrão sincroniza a cada 5 min) e manter o histórico entre reboots:

```bash
install -d /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/10-sync.conf <<'EOF'
[Journal]
Storage=persistent
SyncIntervalSec=10s
EOF
systemctl restart systemd-journald
```

O caminho e os valores (`Storage=persistent`, `SyncIntervalSec=10s`) vêm do guia de operação privado do owner. O conteúdo exato do arquivo
na produção **não foi coletado: conferir** com `cat /etc/systemd/journald.conf.d/10-sync.conf` no servidor original.

Como conferir: `journalctl --disk-usage` e `ls /var/log/journal`; depois de um reboot, `journalctl -b -1 -n 20` mostra o boot anterior.

### B.7 nginx (Linux), completo

O nginx é o **único ponto público**. Arquivos de referência coletados do servidor: [`deploy/servidor-ia/nginx/`](../deploy/servidor-ia/nginx/) e,
para o `ollama.conf`, [`deploy/gateway/ollama.conf`](../deploy/gateway/ollama.conf).

#### B.7.1 Instalação e serviço

```bash
dnf -y install nginx policycoreutils-python-utils
nginx -v                       # nginx version: nginx/1.26.3 (pacote nginx-1.26.3-15.el10)
```

A unit coletada, [`nginx/nginx.service`](../deploy/servidor-ia/nginx/nginx.service), é `Type=forking` com `ExecStartPre=/usr/bin/rm -f /run/nginx.pid` (evita falha
de contexto SELinux no PID) e `ExecStartPre=/usr/sbin/nginx -t` (não sobe com configuração inválida); `ExecReload=nginx -s reload`; `KillSignal=SIGQUIT`.
O pacote do EL10 já instala uma unit equivalente em `/usr/lib/systemd/system/nginx.service`; **instale a cópia do repositório só se precisar reproduzi-la**
(**conferir** se a de produção está em `/etc/systemd/system/` ou em `/usr/lib/systemd/system/`):

```bash
# opcional: reproduzir a unit coletada
install -m 0644 /root/servidor-ia/nginx/nginx.service /etc/systemd/system/nginx.service && systemctl daemon-reload
systemctl enable nginx            # NÃO inicie ainda: faltam certificado, token e ollama.conf
```

#### B.7.2 `nginx.conf` principal

Arquivo: [`nginx/nginx.conf`](../deploy/servidor-ia/nginx/nginx.conf). É o padrão do pacote com poucos pontos que importam:

- `user nginx;` e `worker_processes auto;`;
- `error_log /var/log/nginx/error.log notice;` e `access_log ... main` (o formato `main` registra `$remote_addr`, `$http_x_forwarded_for`; **não** registra o cabeçalho `Authorization`);
- `include /etc/nginx/conf.d/*.conf;` dentro de `http {}`: é por aqui que o `ollama.conf` entra;
- **nenhum listener em texto claro** (porta 80 removida): o host só atende a API por HTTPS na 8443. O bloco `server` TLS de exemplo (porta 443) vem comentado.

Se partir do `nginx.conf` do pacote, apague/comente o `server { listen 80; ... }` padrão e deixe o resto. Em `conf.d/` fica **só o `ollama.conf`**
(na produção havia também um `ollama.conf.bak-20261023-011207`, [`nginx/conf.d-lista.txt`](../deploy/servidor-ia/nginx/conf.d-lista.txt); **backups com final `.conf` são
carregados pelo `include` e duplicariam as definições, por isso mantenha cópias fora do `conf.d/` ou com final diferente de `.conf`**).

#### B.7.3 Pastas, certificado e token

Dono e permissões de produção ([`nginx/permissoes.txt`](../deploy/servidor-ia/nginx/permissoes.txt)):

| Caminho | Dono | Modo |
| --- | --- | --- |
| `/etc/nginx/ollama/` | root:root | 0755 |
| `/etc/nginx/ollama/auth.conf` | root:root | 0600 |
| `/etc/nginx/ollama/api-key` | root:root | 0600 |
| `/etc/nginx/ollama/cloudflare-allow.conf` | root:root | 0644 |
| `/etc/nginx/ssl/` | root:root | 0755 |
| `/etc/nginx/ssl/ia.cslsolucoes.com.br.pem` | root:root | 0644 |
| `/etc/nginx/ssl/ia.cslsolucoes.com.br.key` | root:root | 0600 |

```bash
install -d -m 0755 -o root -g root /etc/nginx/ollama /etc/nginx/ssl
```

**Certificado Cloudflare Origin CA.** No painel do Cloudflare, na zona `cslsolucoes.com.br`: **SSL/TLS -> Origin Server -> Create Certificate**.
Gere com chave privada pelo Cloudflare (RSA ou ECDSA; a lista de cifras do `ollama.conf` serve a ambos), hostname `ia.cslsolucoes.com.br`
(inclua outros se precisar) e a validade que a CSL adota (**conferir** a validade usada na produção). O painel mostra o **certificado** e a **chave privada
uma única vez**: copie as duas.

```bash
umask 077
cat > /etc/nginx/ssl/ia.cslsolucoes.com.br.pem      # cole o certificado, Ctrl+D
cat > /etc/nginx/ssl/ia.cslsolucoes.com.br.key      # cole a chave privada, Ctrl+D
chown root:root /etc/nginx/ssl/ia.cslsolucoes.com.br.*
chmod 0644 /etc/nginx/ssl/ia.cslsolucoes.com.br.pem
chmod 0600 /etc/nginx/ssl/ia.cslsolucoes.com.br.key
```

Também no painel, em **SSL/TLS -> Overview**, use o modo **Full (strict)** (**conferir** o modo atual). A chave privada **nunca** vai para o git. Esse certificado só é
confiável para o Cloudflare; as estações precisam do raiz da Origin CA para validar (parte D.3).

**Token (`auth.conf`).** Gere um token forte e guarde-o **só** em arquivos root 0600 e no cofre da CSL:

```bash
umask 077
openssl rand -hex 32            # 64 caracteres hexadecimais; é o <TOKEN>
```

O arquivo [`nginx-auth.conf.modelo`](../deploy/servidor-ia/nginx-auth.conf.modelo) é o modelo do `/etc/nginx/ollama/auth.conf`. O `map` compara o cabeçalho
`Authorization` **inteiro**, então o valor real é **`"Bearer <TOKEN>"`** (com o prefixo `Bearer `); o modelo coletado mostra só `"<TOKEN>"`, como placeholder.
Na instalação à mão:

```bash
cat > /etc/nginx/ollama/auth.conf <<'EOF'
# Rotated by ollama-key-rotate
map $http_authorization $ollama_authorized {
    default 0;
    "Bearer <TOKEN>" 1;
}
EOF
chmod 0600 /etc/nginx/ollama/auth.conf
printf '%s\n' '<TOKEN>' > /etc/nginx/ollama/api-key && chmod 0600 /etc/nginx/ollama/api-key
```

(Troque `<TOKEN>` pelo valor gerado, no editor, sem deixar o token no histórico do shell.) O script de rotação ([B.7.5](#b75-rotação-do-token-ollama-key-rotate)) grava a variante
por expressão regular `"~*^Bearer[[:space:]]+<TOKEN>$"`, que aceita o mesmo cabeçalho sem diferenciar maiúsculas de `Bearer` e com mais de um espaço; as duas formas funcionam.

Guarde também o token em **`/root/API-Key`** (arquivo root 0600, **o token está na 2ª linha**; a 1ª linha é um título livre). O `stage2.sh` do gateway lê
`sed -n 2p /root/API-Key` para a conferência final (o formato do arquivo vem do guia privado e do `stage2.sh`):

```bash
umask 077
printf 'API key do Ollama CSL\n%s\n' '<TOKEN>' > /root/API-Key
```

#### B.7.4 O `ollama.conf`, bloco a bloco

Destino: `/etc/nginx/conf.d/ollama.conf`. Fonte: [`deploy/gateway/ollama.conf`](../deploy/gateway/ollama.conf) (é o que o `Instalar-Gateway.ps1` instala).

| Bloco | O que faz |
| --- | --- |
| `geo $ollama_from_lan` | marca `10.100.2.0/24` como LAN (`1`); o resto é `0` |
| `map ... $ollama_client_ip` | decide o **IP real do cliente**: se veio da LAN, usa `$remote_addr` (não confia no `CF-Connecting-IP`, que a LAN poderia forjar); se não é LAN e há `CF-Connecting-IP`, usa esse cabeçalho (vem do túnel/borda do Cloudflare); senão `$remote_addr`. O loopback **não** conta como LAN, porque o `cloudflared` entrega o tráfego da internet por 127.0.0.1/::1 com o `CF-Connecting-IP` |
| `include /etc/nginx/ollama/auth.conf;` | define `$ollama_authorized` (0 ou 1) pelo Bearer |
| `limit_req_zone` / `limit_conn_zone` | zonas por IP real: **600 pedidos/min** (`rate=600r/m`, `burst=100 nodelay`) e **80 conexões simultâneas** por IP (`limit_conn 80`); passar disso dá **429**. Valem também para tentativas com token errado |
| `server { listen 8443 ssl; listen [::1]:8443 ssl; http2 on; }` | porta pública; `[::1]` é por onde o túnel entra |
| `ssl_*` | TLS 1.2/1.3, cifras ECDHE, sessão em cache, tickets desligados; certificado e chave de `/etc/nginx/ssl/` |
| `add_header` / `server_tokens off` | HSTS, `nosniff`, `X-Frame-Options DENY`, `no-referrer`; esconde a versão do nginx |
| `client_max_body_size 64m` e timeouts | corpo até 64 MB; `client_body_timeout 60s`; `keepalive_timeout 75s` |
| `include cloudflare-allow.conf; allow 127.0.0.1; allow ::1; allow 10.100.2.0/24; deny all;` | só a borda do Cloudflare, o loopback (túnel e testes locais) e a LAN chegam; o resto toma 403 |
| `error_page 401 = @unauthorized` | resposta `401` em JSON com `WWW-Authenticate: Bearer realm="ollama"` |
| `location ~ ^/api/(pull\|push\|create\|delete\|copy\|blobs)(/\|$)` | **403** para as rotas administrativas do Ollama (o gateway também as recusa); respondem 403 mesmo sem token |
| `location /monitor` | só LAN: `if ($http_cf_connecting_ip != "") { return 403; }` recusa tudo que veio pelo Cloudflare/túnel; `allow 10.100.2.0/24, 127.0.0.1, ::1; deny all;`. Proxy para `127.0.0.1:11500` **sem buffer** (SSE) e com `proxy_read_timeout 3600s`. Não exige Bearer (a página tem login próprio para as ações) |
| `location /` | `if ($ollama_authorized = 0) { return 401; }`; proxy para o **gateway** `127.0.0.1:11500`; `Host 127.0.0.1:11434` (o Ollama recusa `Host` não local); `X-Real-IP $ollama_client_ip`; `proxy_buffering off` e `proxy_cache off` (streaming de tokens); `proxy_read_timeout`/`send_timeout 3600s`, `connect_timeout 10s`. **O buffer do corpo do pedido fica ligado** (o gateway conta com o nginx para absorver corpos lentos) |

**Instalação antes do gateway.** O `ollama.conf` do repositório aponta para o gateway (`:11500`), que só existe depois da parte C. Para o nginx já responder
nesta fase, e para o rollback do instalador ter um estado anterior válido, instale uma **variante temporária** apontando direto ao Ollama
(derivada com `sed`; **não é um arquivo do repositório**: conferir o resultado):

```bash
sed 's#http://127.0.0.1:11500#http://127.0.0.1:11434#' /root/servidor-ia/ollama.conf > /etc/nginx/conf.d/ollama.conf
```

Nesse modo, `/monitor` não funciona (não há gateway) e a etapa 2 do gateway depois troca para o `ollama.conf` do repositório.
Se preferir instalar o definitivo já agora, copie o `deploy/gateway/ollama.conf` e aceite que `/api/*` dá 502 até a parte C.

#### B.7.5 Rotação do token: `ollama-key-rotate`

Cópia do script de produção: [`nginx/ollama-key-rotate.sh`](../deploy/servidor-ia/nginx/ollama-key-rotate.sh). **Instalado no servidor como `/usr/local/bin/ollama-key-rotate`**
(caminho informado pelo owner). Instalação num servidor novo:

```bash
install -m 0755 -o root -g root /root/servidor-ia/nginx/ollama-key-rotate.sh /usr/local/bin/ollama-key-rotate
```

O que ele faz (só root; `umask 077`):

1. gera um token novo (`openssl rand -hex 32`);
2. **reescreve** `/etc/nginx/ollama/auth.conf` com o `map` por regex `"~*^Bearer[[:space:]]+<TOKEN novo>$"` (0600);
3. grava o token em `/etc/nginx/ollama/api-key` (0600);
4. roda `nginx -t` e `systemctl reload nginx`, espera 2 s (os workers trocam aos poucos);
5. **imprime o token novo na tela** (`New API key: ...`): copie-o direto para o cofre e **limpe o terminal**.

```bash
/usr/local/bin/ollama-key-rotate
```

O token antigo **deixa de valer de imediato** (depois do reload). O script **não** atualiza o `/root/API-Key`: grave o novo valor na 2ª linha dele
(o `stage2.sh` do gateway lê dali) e atualize os clientes (G.4).

#### B.7.6 `cloudflare-allow.conf`

Arquivo: [`nginx-cloudflare-allow.conf`](../deploy/servidor-ia/nginx-cloudflare-allow.conf) -> `/etc/nginx/ollama/cloudflare-allow.conf` (0644). São as linhas
`allow <faixa>;` das redes da borda do Cloudflare (22 faixas, 15 IPv4 e 7 IPv6 na cópia de 2026-10-30). Para atualizar com as listas oficiais:

```bash
{ curl -fsS https://www.cloudflare.com/ips-v4 ; echo; curl -fsS https://www.cloudflare.com/ips-v6 ; } \
  | sed '/^$/d; s/^/allow /; s/$/;/' > /root/cloudflare-allow.conf.new
wc -l /root/cloudflare-allow.conf.new        # esperado: perto de 22; abra e confira o conteúdo antes de instalar
cp -p /etc/nginx/ollama/cloudflare-allow.conf /root/cloudflare-allow.conf.antes-$(date +%Y%m%d)
install -m 0644 -o root -g root /root/cloudflare-allow.conf.new /etc/nginx/ollama/cloudflare-allow.conf
nginx -t && systemctl reload nginx
```

Como o tráfego pelo túnel chega por loopback, esta lista importa sobretudo para o caminho pelo proxy do Cloudflare diretamente na 8443.

#### B.7.7 SELinux

Coletado em [`nginx/selinux.txt`](../deploy/servidor-ia/nginx/selinux.txt): o SELinux está **Enforcing**, sem módulo local.

- **`httpd_can_network_connect` = on**: necessário para o nginx fazer proxy até `127.0.0.1:11500`/`11434`. Se faltar, o sintoma é 502 com `Permission denied` no `error.log`:

  ```bash
  setsebool -P httpd_can_network_connect 1
  getsebool httpd_can_network_connect          # httpd_can_network_connect --> on
  ```

- `httpd_can_network_relay` fica **off** (não é necessário).
- A porta **8443 já é `http_port_t`** (`semanage port -l | grep http_port_t` lista `80, 81, 443, 488, 8008, 8009, 8443, 9000`), então o nginx pode escutar nela sem ajuste.
- Os arquivos em `/etc/nginx/ssl` e `/etc/nginx/ollama` criados com `install`/`cat` herdam o contexto do diretório; se o nginx não ler algum, rode `restorecon -Rv /etc/nginx`.

#### B.7.8 logrotate

[`nginx/logrotate-nginx`](../deploy/servidor-ia/nginx/logrotate-nginx) -> `/etc/logrotate.d/nginx`: `/var/log/nginx/*.log`, diário, 10 rotações, comprimido com `delaycompress`,
arquivos criados `0640 nginx root`, e no `postrotate` manda `USR1` ao nginx para reabrir os logs.

```bash
install -m 0644 -o root -g root /root/servidor-ia/nginx/logrotate-nginx /etc/logrotate.d/nginx
logrotate -d /etc/logrotate.d/nginx          # simulação; não deve mostrar erro
```

(**Conferir**: o pacote do EL10 pode já trazer um `/etc/logrotate.d/nginx`; mantenha só um.)

#### B.7.9 Subir e conferir o nginx

```bash
nginx -t                                   # syntax is ok / test is successful
systemctl enable --now nginx
systemctl reload nginx
sleep 5                                    # os workers antigos saem aos poucos; confira só depois
ss -tlnH | grep 8443                       # LISTEN em 0.0.0.0:8443 e [::1]:8443
```

Testes no próprio servidor (o token entra pelo stdin, nunca na linha de comando, para não aparecer no `ps`):

```bash
curl -sk -o /dev/null -w 'sem token: %{http_code}\n'  https://127.0.0.1:8443/api/version        # 401
curl -sk -o /dev/null -w 'pull:      %{http_code}\n'  https://127.0.0.1:8443/api/pull           # 403
printf 'Authorization: Bearer %s\n' "$(sed -n 2p /root/API-Key)" \
  | curl -sk -o /dev/null -w 'com token: %{http_code}\n' https://127.0.0.1:8443/api/version -H @-   # 200
curl -sk -o /dev/null -w 'monitor:   %{http_code}\n'  https://127.0.0.1:8443/monitor             # 200 só com o gateway (parte C); sem ele, 502
```

Da LAN (outra máquina): `curl -k -o NUL -w "%{http_code}" https://ia.cslsolucoes.com.br:8443/monitor` -> 200 (com o gateway). Da internet (celular fora do Wi-Fi, depois da B.9):
`https://ia.cslsolucoes.com.br/monitor` -> **403**.

### B.8 Firewall

Estado coletado (zona `public`): **8443/tcp, ssh, cockpit, dhcpv6-client**. A 11434 **não** aparece (o Ollama só escuta no loopback).

```bash
firewall-cmd --permanent --zone=public --add-port=8443/tcp
firewall-cmd --permanent --zone=public --add-service=ssh
firewall-cmd --permanent --zone=public --add-service=cockpit
firewall-cmd --permanent --zone=public --remove-port=11434/tcp 2>/dev/null || true
firewall-cmd --reload
firewall-cmd --zone=public --list-all
```

Como conferir: `ports: 8443/tcp`, `services: cockpit dhcpv6-client ssh`. De outra máquina da LAN, a 11434 deve estar fechada (parte F).
Muitas conexões SSH seguidas podem acionar o fail2ban: use poucas.

### B.9 `cloudflared` (túnel)

Versão de referência: **2026.10.6**. Instalação por **RPM oficial** da release do GitHub, **conferindo o SHA-256 publicado na release**.
Não há repositório do Cloudflare configurado; as atualizações também são por RPM.

```bash
cd /root
# 1) baixe o RPM (cloudflared-linux-x86_64.rpm) da release 2026.10.6 em
#    https://github.com/cloudflare/cloudflared/releases  (use o link da página da release)
# 2) copie o SHA-256 que a release publica e confira:
echo '<SHA256-PUBLICADO-NA-RELEASE>  cloudflared-linux-x86_64.rpm' | sha256sum -c -     # cloudflared-linux-x86_64.rpm: OK
rpm -Uvh cloudflared-linux-x86_64.rpm
cloudflared --version            # cloudflared version 2026.10.6
```

O nome exato do arquivo na release e o método de download usado na produção: **conferir**. O RPM instala `/usr/bin/cloudflared`.

**Túnel gerenciado pelo painel.** No painel do Cloudflare (**Zero Trust -> Networks -> Tunnels -> Create a tunnel -> Cloudflared**), crie o túnel e copie o **token**
que o painel mostra (é só o token, não o comando). Em **Public Hostname** do túnel, crie `ia.cslsolucoes.com.br` apontando para
**`https://localhost:8443`** (o nginx). Como o certificado de origem é da Origin CA (que o SO não confia), a verificação TLS desse destino precisa ser tratada
no painel (por exemplo "No TLS Verify" ou `originServerName`); o ajuste usado na produção **não está documentado: conferir**. Tanto `ia.cslsolucoes.com.br`
quanto `ia.cslsolucoes.com.br:8443` chegam por esse túnel.

```bash
install -d -m 0755 /etc/cloudflared
umask 077
cat > /etc/cloudflared/token          # cole o token do painel, Ctrl+D
chown root:root /etc/cloudflared/token && chmod 0600 /etc/cloudflared/token
install -m 0644 /root/servidor-ia/cloudflared.service /etc/systemd/system/cloudflared.service
systemctl daemon-reload
systemctl enable --now cloudflared
```

A unit [`cloudflared.service`](../deploy/servidor-ia/cloudflared.service): `Type=notify`,
`ExecStart=/usr/bin/cloudflared --no-autoupdate tunnel run --token-file /etc/cloudflared/token`, `Restart=on-failure`, `RestartSec=5s`, `After=network-online.target`.
O token vai por **arquivo** (`--token-file`), nunca na linha de comando.

Como conferir: `systemctl is-active cloudflared`; `journalctl -u cloudflared -n 20 --no-pager` mostra as conexões do túnel registradas (4 na produção); no painel, o túnel
aparece "Healthy". Da internet: `/api/version` sem token -> **401** e `/monitor` -> **403**. No DNS do Cloudflare, o registro de `ia.cslsolucoes.com.br` deve existir
e ser criado pelo túnel (**conferir**); o DNS interno da LAN aponta o mesmo nome para `10.100.2.2` (split-horizon: por isso, dentro da LAN, use a porta 8443).

### B.10 NAT no roteador

A regra de NAT `WAN:8443 -> 10.100.2.2:8443` (no roteador MikroTik do escritório) está restrita a uma **lista de endereços permitidos** (`PERMITIDOS`), por decisão do owner
(2026-10-30): antes estava aberta a qualquer IP e só varredores a usavam. O Cloudflare **não** depende dela, porque entra pelo túnel.

- Crie/atualize a lista `PERMITIDOS` com os IPs públicos autorizados (`<IP>`) e deixe a regra de NAT casando só com essa lista de origem.
- Confira de um IP **fora** da lista: `curl -k https://<IP-PÚBLICO-DA-CSL>:8443/api/version` deve expirar/recusar; de um IP da lista, dar 401.
- A sintaxe exata no RouterOS e o nome do roteador são do guia privado do owner (**conferir**).

---

## C. Gateway de fila e `/monitor`

O gateway é instalado **a partir do PC**, pelo [`deploy/gateway/Instalar-Gateway.ps1`](../deploy/gateway/Instalar-Gateway.ps1). Detalhes de operação: [`MANUAL-GATEWAY.md`](MANUAL-GATEWAY.md)
(não copiados aqui) e [`deploy/gateway/README.md`](../deploy/gateway/README.md).

**Pré-requisitos.** No PC: `go`, `git`, `ssh.exe`, `scp.exe` no PATH e o alias SSH do servidor funcionando (`ssh <ALIAS-SSH> true`; o padrão do instalador é `lx-ai01-root`, e o
parâmetro `-Servidor` o troca). No servidor: tudo da parte B pronto, `python3` instalado (a etapa 2 o usa para ler JSON) e o token na 2ª linha de `/root/API-Key`
(a etapa 2 faz uma chamada real pelo nginx). Instale com a **fila vazia**: a etapa 2 reinicia o Ollama.

```powershell
cd E:\MCPOllam\deploy\gateway
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1                  # instalação completa
```

| Modo (`-Modo`) | Faz | Muda o tráfego? |
| --- | --- | --- |
| `Completo` (padrão) | binário Linux em `D:\MCP\mcpollama-gateway`, pacote com LF e `SHA256SUMS`, envio a `/root/deploy96` (root 0700), etapa 1, senha do admin (se ainda não existe), etapa 2 | **sim** |
| `Preparar` | só a etapa 1 (usuário `mcpgw`, pastas, binário) | não |
| `Atualizar` | troca o binário e reinicia o gateway (corta quem está em execução) | não |
| `Senha` | (re)define a senha do admin | não (reinicie o gateway depois) |
| `Reverter -Backup <pasta>` | roda o `rollback.sh` guardado na pasta de backup | **sim** |

**Senha do admin do `/monitor`.** Se o arquivo `/etc/mcpollama-gateway/admin` ainda não existe, o instalador abre `ssh -t` e roda `mcpgw-definir-senha.sh`: você digita
`<SENHA>` (12 caracteres ou mais) no seu terminal, **sem eco**; só o hash PBKDF2 é gravado (`mcpgw:mcpgw`, 0600). Para trocar depois: `-Modo Senha` e
`systemctl restart mcpollama-gateway`.

**O que a etapa 2 muda** (e faz backup antes, em `/root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS`):

1. instala e inicia a unit `mcpollama-gateway` (127.0.0.1:11500, ainda sem tráfego);
2. muda o `OLLAMA_HOST` do drop-in para `127.0.0.1:11434` (num servidor montado por este guia, já está assim) e **reinicia o Ollama** e o `ollama-warm`;
3. troca o `/etc/nginx/conf.d/ollama.conf` pelo do repositório e faz `nginx -t` + `reload`;
4. confere pelo caminho real e espera ~5 s depois do reload. Se algo falhar depois de mexer no nginx/Ollama, **volta sozinha**.

Saída esperada do fim da etapa 2: `active`, `gateway /api/version: 200`, `gateway /monitor: 200`, `resposta: 'pong' ...`, três linhas `x-request-id`/`x-queue-ms`/`x-queue-position`,
`sem token: 401`, `pull: 403`, `/monitor: 200`, `ETAPA2_OK (backups e rollback em <B>: bash <B>/rollback.sh <B>)`. **Anote a pasta `<B>`.**

**Rollback.** `-Modo Reverter -Backup <B>` (ou, no servidor, `bash <B>/rollback.sh <B>`) devolve o nginx e o drop-in do Ollama ao estado guardado. Num servidor novo montado por
este guia, o estado guardado é o da B.7.4 (nginx direto ao Ollama) **se você usou a variante temporária**; por isso ela é recomendada. O rollback não apaga usuário, binário, unit, senha nem banco.

Como conferir (no servidor): `systemctl is-active mcpollama-gateway` (`active`), `/usr/local/bin/mcpollama-gateway --version`, `curl -s http://127.0.0.1:11500/healthz` (`ok`),
`ss -tlnH | grep -E '11434|11500'` (ambos em 127.0.0.1). Na LAN: `https://ia.cslsolucoes.com.br:8443/monitor` abre o painel.
Tabela de diagnóstico completa: seção 10 do [MANUAL](MANUAL-GATEWAY.md#10-diagnóstico).

### C.1 Fase 12 (gateway 0.13.0): chave HMAC e variáveis (a implantar)

A Fase 12 (erros do modelo → base de treino; detalhes na seção 17 do [MANUAL](MANUAL-GATEWAY.md#17-erros-do-modelo-e-base-de-treino-fase-12-a-implantar)) acrescenta dois passos
a esta parte, **sem comando extra**: o instalador já faz os dois.

1. **Chave HMAC.** A etapa 1 (`stage1.sh`) cria `/etc/mcpollama-gateway/doc.key` **se não existir**: 32 bytes aleatórios em hex (64 caracteres),
   dono `mcpgw:mcpgw`, modo **0600** (o gateway recusa qualquer permissão de grupo ou outros e roda como `mcpgw`). **Nunca sobrescreve** uma chave
   existente e **nunca a imprime**: mostra só o `key_id` (16 hex do sha256 da chave). Anote o `key_id`. Num segundo gateway (fallback), copie a **mesma** chave
   por canal protegido e confira o `key_id` igual. A chave **não entra no backup comum**.
2. **Variáveis da unit.** A unit `mcpollama-gateway.service` traz `Environment=MCPGW_TREINO_DB=/var/lib/mcpollama-gateway/treino.db`,
   `MCPGW_DOC_KEY_FILE=/etc/mcpollama-gateway/doc.key` e `MCPGW_ROLE=primario` (num gateway de fallback, `fallback`). São **variáveis, não flags**, para um rollback
   só do binário (para o 0.12.6, que recusa flag desconhecida mas ignora variável) continuar subindo. A melhora sem treino fica **desligada** (nenhuma `MCPGW_MELHORA_*`).

**Ordem:** gateway 0.13.0 **antes** do mcpollama 0.10.0. Para um servidor já instalado, `Instalar-Gateway.ps1 -Modo Atualizar` troca binário **e unit** e volta sozinho se o gateway
não subir (o nginx e o Ollama não são tocados). Conferência: `ls -l /etc/mcpollama-gateway/doc.key` (`mcpgw mcpgw -rw-------`), `systemctl show mcpollama-gateway -p Environment`
(as três variáveis) e a aba **Erros (treino)** no `/monitor`.

Nota: o limite de conexões simultâneas por IP é **80** (`limit_conn ollama_conn 80` no `ollama.conf`, desde 2026-10-30 22:35; antes 30, com `burst=60`, e a fila do gateway por IP subiu de 16 para 64); o MANUAL do gateway está alinhado com esse valor.

---

## D. Estação Windows (mcpollama)

### D.1 Pré-requisitos

| Item | Detalhe |
| --- | --- |
| Go | o `src/go.mod` declara `go 1.26.0`; a toolchain local é a **go1.27.0** (`C:\Program Files\Go\bin\go`). Baixe de <https://go.dev/dl/> |
| Git | para clonar o repositório privado (precisa de acesso à organização `cslsolucoes` no GitHub) |
| OpenSSH | `ssh.exe` e `scp.exe` (recurso opcional do Windows), necessários só para o instalador do gateway |
| gcc (opcional) | só para o detector de corrida (`go test -race`, exige cgo): o projeto usa um MinGW-w64 em `E:\MCPOllam\deps\gcc-mingw-w64\bin` (`deps\` **não é versionada**; a origem do pacote é **conferir**) |

```powershell
go version                       # go version go1.27.0 windows/amd64 (ou superior a 1.26.0)
git clone https://github.com/cslsolucoes/ollama-mcp.git E:\MCPOllam
```

### D.2 Build

```powershell
New-Item -ItemType Directory -Force D:\MCP | Out-Null
cd E:\MCPOllam\src
go build ./...
go vet ./...
go build -ldflags "-X main.Version=<versao>" -o D:\MCP\mcpollama-mcp.exe ./mcpollama-mcp
D:\MCP\mcpollama-mcp.exe --version
```

`<versao>` é livre (por exemplo a tag ou o commit curto: `git -C E:\MCPOllam rev-parse --short HEAD`). Como conferir: `--version` imprime a versão informada. Para validar o código antes:
`go test -count=1 ./...` (todos os pacotes `ok`). Se o servidor estiver aberto num cliente, o `go build` renomeia o `.exe` em uso para `.exe~`; apague depois de reiniciar o cliente.
Para o gateway Linux (feito pelo instalador): `$env:GOOS='linux'; $env:CGO_ENABLED='0'; go build -o D:\MCP\mcpollama-gateway ./cmd/mcpollama-gateway`.

### D.3 CA do Cloudflare

O certificado do nginx é da **Cloudflare Origin CA**, em que o Windows não confia. O mcpollama soma o PEM ao `SystemCertPool` (flag `--tls-ca`).
O PEM é o **raiz da Origin CA do Cloudflare** (assunto "CloudFlare Origin SSL Certificate Authority"), que a documentação do Cloudflare publica (SSL/TLS -> Origin Server -> Origin CA root certificates).
Salve em:

```text
%USERPROFILE%\.openclaude\certs\cloudflare-origin-ca.pem
```

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.openclaude\certs" | Out-Null
# cole/salve o PEM nesse caminho (UTF-8, LF), depois confira:
certutil -dump "$env:USERPROFILE\.openclaude\certs\cloudflare-origin-ca.pem" | Select-String -Pattern 'Subject|NotAfter'
```

Esperado: assunto com `CloudFlare Origin SSL Certificate Authority`. (Na estação do owner o raiz vale até 2029-08-15; confira a validade da cópia que você baixou.)
Se o certificado de origem do servidor for ECDSA, use o raiz ECC correspondente da página do Cloudflare (**conferir** qual dos raízes corresponde ao certificado emitido).

### D.4 OCR (opcional)

Sem as flags, o PDF digitalizado segue recusado; PDFs com camada de texto, `.docx` e `.pptx` funcionam sem OCR.

| Peça | Local | Detalhe |
| --- | --- | --- |
| `libtesseract64.dll` | `D:\MCP\libtesseract64.dll` | Tesseract **5.3.3**, x64, autocontida (importa só `KERNEL32.dll` e `msvcrt.dll`), 5.965.824 bytes |
| SHA-256 da DLL | flag `--ocr-lib-sha256` | `964B0972348F6283B1721F9131DA59721CE46FB4F7A4E294399B1169C7E71669` |
| `tessdata` | `D:\MCP\tessdata\best` e `D:\MCP\tessdata\fast` | cada pasta com `por.traineddata` e `osd.traineddata` |
| cache do PDF | `D:\MCP\cache\pdfium` | cache de compilação do motor de PDF (flag `--pdf-cache-dir`) |

**A DLL não é versionada e não tem download oficial pronto para Windows.** A cópia da CSL veio de um binário pronto de repositório de terceiro (Delphi), guardado em
`referencias/tessract-group/` na estação do owner (não versionada) e registrada no plano da Fase 7. **Copie a DLL de uma estação que já a tenha (ou do cofre da CSL) e confira o hash;
a origem de download e a receita de compilação não estão documentadas nas fontes: conferir.**

```powershell
(Get-FileHash -Algorithm SHA256 D:\MCP\libtesseract64.dll).Hash
# esperado: 964B0972348F6283B1721F9131DA59721CE46FB4F7A4E294399B1169C7E71669
```

**tessdata.** Obtenha os modelos no repositório oficial do Tesseract: `tessdata_best` e `tessdata_fast` em <https://github.com/tesseract-ocr/tessdata_best> e
<https://github.com/tesseract-ocr/tessdata_fast> (arquivos `por.traineddata` e `osd.traineddata`; o branch padrão é **conferir**).

```powershell
New-Item -ItemType Directory -Force D:\MCP\tessdata\best, D:\MCP\tessdata\fast, D:\MCP\cache\pdfium | Out-Null
foreach ($v in 'best','fast') { foreach ($f in 'por','osd') {
  Invoke-WebRequest "https://github.com/tesseract-ocr/tessdata_$v/raw/main/$f.traineddata" -OutFile "D:\MCP\tessdata\$v\$f.traineddata" } }
Get-ChildItem D:\MCP\tessdata -Recurse -File | Select-Object FullName, Length
```

Tamanhos de referência (cópia atual do owner): `best\por` 8.159.939, `best\osd` 10.562.727, `fast\por` 1.982.756, `fast\osd` 10.562.727 bytes.
Por padrão o OCR usa `<pasta da DLL>\tessdata\best`. Flags do OCR (padrões em `src/mcpollama-mcp/config.go` e no [CLAUDE.md](../CLAUDE.md)): `--ocr-lib`, `--ocr-lib-sha256` (obrigatório com `--ocr-lib`),
`--ocr-tessdata`, `--ocr-workers`, `--ocr-page-timeout` (1m), `--ocr-max-pages` (30), `--ocr-max-mem` (1 GiB), `--ocr-max-megapixels` (40), `--ocr-dpi` (300).
Qualquer problema do OCR na partida (hash divergente, DLL ausente, modelos ausentes) vira `WARNING: OCR disabled: ...` no stderr e o servidor sobe **sem OCR**.

### D.5 Smoke test por stdio

Sem cliente MCP, direto no PowerShell (substitua `<TOKEN>`; o `list_models` precisa do servidor no ar):

```powershell
$init = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'
$list = '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
$call = '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_models","arguments":{}}}'
$init, $list, $call | D:\MCP\mcpollama-mcp.exe --ollama-url https://ia.cslsolucoes.com.br:8443 `
  --tls-ca "$env:USERPROFILE\.openclaude\certs\cloudflare-origin-ca.pem" --api-key <TOKEN>
```

Como conferir: três respostas JSON-RPC; a 1ª com `serverInfo` e `protocolVersion`; a 2ª com as tools (`list_models`, `generate`, `chat`, `embed` e, com `--allow-root`, `summarize_file`, `extract`, `translate`: **7**, catálogo em [TOOLS.md](../TOOLS.md));
a 3ª com `qwen3.8:27b` e `nomic-embed-text`. Erros: `x509` = CA errada/ausente (D.3); `401` = token errado; timeout = servidor ou rota (seção H). O token não deve ficar no histórico:
prefira a variável de ambiente `OLLAMA_API_KEY` (`$env:OLLAMA_API_KEY='<TOKEN>'`, só na sessão).

---

## E. Clientes MCP

Cada cliente chama o mesmo executável. **Os arquivos com o token ficam fora do git** (`.mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` já estão no `.gitignore`).
Reinicie o cliente depois de trocar o binário ou a configuração.

### E.1 Claude Code

`.mcp.json` na **raiz do projeto** (escopo de projeto). Fora do git; token só em argumento:

```json
{
  "mcpServers": {
    "mcpOllama": {
      "command": "D:\\MCP\\mcpollama-mcp.exe",
      "args": [
        "--ollama-url", "https://ia.cslsolucoes.com.br:8443",
        "--tls-ca", "C:\\Users\\<USUARIO>\\.openclaude\\certs\\cloudflare-origin-ca.pem",
        "--allow-root", "<RAIZ1>;<RAIZ2>",
        "--pdf-cache-dir", "D:\\MCP\\cache\\pdfium",
        "--ocr-lib", "D:\\MCP\\libtesseract64.dll",
        "--ocr-lib-sha256", "964B0972348F6283B1721F9131DA59721CE46FB4F7A4E294399B1169C7E71669",
        "--api-key", "<TOKEN>"
      ]
    }
  }
}
```

- `--allow-root` aceita **várias pastas absolutas separadas por `;`** (por exemplo `E:\MCPOllam;E:\GestorERP`). Sem ele, as tools de arquivo não existem (ficam só as 4 base). Arquivos sensíveis (`.mcp.json`, `.env`, chaves, `.git`) são recusados por lista fixa.
- Sem OCR, retire `--ocr-lib` e `--ocr-lib-sha256`. Sem cache do PDF, retire `--pdf-cache-dir` (o 1º PDF leva ~8 s a mais).
- No primeiro uso o Claude Code pede aprovação do servidor do projeto.

Conferência: **reinicie a sessão do Claude Code**, rode `/mcp` (o `mcpOllama` deve aparecer conectado) e peça a tool `list_models`: deve listar `qwen3.8:27b` e `nomic-embed-text`.
Em seguida, teste uma tool de arquivo (`summarize_file` de um arquivo dentro de uma raiz permitida).

### E.2 Cursor e VS Code

**Cursor** (`.cursor/mcp.json`, chave `mcpServers`):

```json
{
  "mcpServers": {
    "mcpOllama": {
      "command": "D:\\MCP\\mcpollama-mcp.exe",
      "args": [
        "--ollama-url", "https://ia.cslsolucoes.com.br:8443",
        "--tls-ca", "C:\\Users\\<USUARIO>\\.openclaude\\certs\\cloudflare-origin-ca.pem",
        "--allow-root", "<RAIZ1>",
        "--api-key", "<TOKEN>"
      ]
    }
  }
}
```

**VS Code** (`.vscode/mcp.json`, chave **`servers`** e `"type": "stdio"`):

```json
{
  "servers": {
    "mcpOllama": {
      "type": "stdio",
      "command": "D:\\MCP\\mcpollama-mcp.exe",
      "args": [
        "--ollama-url", "https://ia.cslsolucoes.com.br:8443",
        "--tls-ca", "C:\\Users\\<USUARIO>\\.openclaude\\certs\\cloudflare-origin-ca.pem",
        "--allow-root", "<RAIZ1>",
        "--api-key", "<TOKEN>"
      ]
    }
  }
}
```

A validação nesses dois é opcional (o foco é o Claude Code). Conferência: o servidor `mcpOllama` aparece conectado e `list_models` responde.

### E.3 ia-local (OpenClaude)

Cliente OpenAI-compatível pelo `/v1`, usado pelo script `%USERPROFILE%\.claude\scripts\ia-local.ps1`. A configuração fica em `%USERPROFILE%\.openclaude\csl-ollama.env` (fora do git):

```ini
CLAUDE_CODE_USE_OPENAI=1
OPENAI_BASE_URL=https://ia.cslsolucoes.com.br:8443/v1
OPENAI_API_KEY=<TOKEN>
OPENAI_MODEL=qwen3.8:27b
```

- `OPENAI_MODEL=qwen3.8:27b` (raciocínio **desligado**, o gateway injeta `reasoning_effort:"none"`) ou `qwen3.8:27b-think` (raciocínio ligado, mais lento; é um nome virtual só do gateway, não existe no Ollama). Volte para `qwen3.8:27b` depois.
- A CA também precisa estar em `%USERPROFILE%\.openclaude\certs\cloudflare-origin-ca.pem` (D.3).
- Como conferir: `powershell -NoProfile -ExecutionPolicy Bypass -File "$env:USERPROFILE\.claude\scripts\ia-local.ps1" -Check`. O script em si pertence ao pack do usuário e **não está neste repositório: conferir**.

---

## F. Conferência final (checklist)

No servidor:

```bash
systemctl is-active ollama ollama-warm nvidia-powercap nginx cloudflared mcpollama-gateway   # 6 linhas "active"
ollama ps                                      # qwen3.8:27b (CONTEXT 65536) e nomic-embed-text, PROCESSOR 100% GPU
nvidia-smi --query-gpu=index,power.limit --format=csv,noheader     # 4 GPUs, 100.00 W
ss -tlnH | grep -E '11434|11500|8443'          # 11434 e 11500 em 127.0.0.1; 8443 pública
firewall-cmd --zone=public --list-ports        # 8443/tcp
```

Testes de rede:

| Teste | Comando | Esperado |
| --- | --- | --- |
| sem token | `curl -sk -o /dev/null -w '%{http_code}' https://ia.cslsolucoes.com.br:8443/api/version` (da LAN) | **401** |
| com token | mesmo, com `-H "Authorization: Bearer <TOKEN>"` (de preferência pelo stdin, como na B.7.9) | **200** |
| rota administrativa | `.../api/pull` | **403** |
| `/monitor` da LAN | `https://ia.cslsolucoes.com.br:8443/monitor` | **200** (página do painel) |
| `/monitor` da internet | `https://ia.cslsolucoes.com.br/monitor` (celular fora do Wi-Fi) | **403** |
| internet sem token | `https://ia.cslsolucoes.com.br/api/version` | **401** |
| 11434 na LAN | `Test-NetConnection 10.100.2.2 -Port 11434` (de um PC da LAN) | `TcpTestSucceeded : False` |
| 8443 na LAN | `Test-NetConnection 10.100.2.2 -Port 8443` | `True` |

Uma tool MCP respondendo (estação): reinicie o Claude Code e chame `list_models` e uma tool de arquivo (E.1). Após o reload do nginx, **espere ~5 s** antes de conferir.

---

## G. Atualizações e manutenção

### G.1 Trocar o binário do mcpollama (estação)

```powershell
cd E:\MCPOllam
git pull
cd src
go test -count=1 ./...
go build -ldflags "-X main.Version=<versao>" -o D:\MCP\mcpollama-mcp.exe ./mcpollama-mcp
```

**mcpollama 0.10.0 (Fase 12):** só depois do gateway 0.13.0 já estar no servidor (C.1). A tool `reportar_erro` aparece sem configuração; `--auto-report` (relatos automáticos) vem **desligado** e só se acrescenta ao `.mcp.json` por decisão do owner.

Reinicie o cliente (Claude Code, Cursor, VS Code); o binário em uso é renomeado para `.exe~` (apague-o depois). Confira com `--version` e `list_models`.
Mudanças em `src/files/`, `src/doctext/`, `src/ocr/` ou `src/gateway/` exigem rodar de novo os testes de ataque descritos no [CLAUDE.md](../CLAUDE.md).

### G.2 Trocar o binário do gateway

`Instalar-Gateway.ps1 -Modo Atualizar` (parte C). Olhe o `/monitor` antes: o restart corta quem está em execução. O banco migra sozinho. Desde a Fase 12 o modo `Atualizar` também troca a unit (se mudou) e volta sozinho se o gateway não subir (C.1). Para voltar ao binário antigo, a etapa 1 guardou uma cópia em
`/root/srv-backup-20261028/fase96-AAAAMMDD-HHMMSS/mcpollama-gateway`.

### G.3 Trocar o modelo

Desde a Fase 12 (2026-11-05) **não há alias**: o modelo de geração é identificado pelo nome real (`qwen3.8:27b`), e o nome aparece na flag
`--model`/`--allowed-models` do mcpollama (padrão em `src/mcpollama-mcp/config.go`), no `--allowed-models` da unit do gateway
(`deploy/gateway/mcpollama-gateway.service`) e no `ollama-warm.sh`. Trocar de modelo exige ajustar o nome nesses pontos e nos clientes
(por exemplo, `OPENAI_MODEL` do `ia-local`); o nome virtual de raciocínio é `<modelo>-think`.

```bash
cp -p /etc/systemd/system/ollama.service.d/10-performance.conf /root/10-performance.conf.antes-$(date +%Y%m%d-%H%M)
ollama pull <NOVO-MODELO>
# ajustar o nome no --allowed-models do gateway e no ollama-warm.sh, e reiniciar os serviços:
systemctl restart ollama && systemctl restart ollama-warm
ollama ps
```

**Confira os pré-requisitos do novo modelo:** se ele raciocina por padrão (`think:false`), se roda em 1 slot, e a VRAM a 64k. Faça um teste de carga antes de adotar (as medidas da Fase 8, com o `qwen3.5:9b`, estão no
[report de carga multiagente](../.workspace/reports/MCPOllam2026-10-28-235002-fase8-9-carga-multiagente.report.md)). Para voltar ao modelo anterior, mantenha-o instalado (`ollama list`) até o aceite e só então remova (`ollama rm`).

### G.4 Rotacionar o token

No servidor: `/usr/local/bin/ollama-key-rotate` (B.7.5). Ele imprime o token novo uma vez. Depois, **na ordem**:

1. grave o token novo na 2ª linha de `/root/API-Key` (o `stage2.sh` do gateway lê dali);
2. atualize **cada cliente**: o `--api-key` do `.mcp.json` (Claude Code), do `.cursor/mcp.json` e do `.vscode/mcp.json`, e o `OPENAI_API_KEY` do `%USERPROFILE%\.openclaude\csl-ollama.env`; reinicie os clientes;
3. confira: o token antigo dá **401** e o novo dá **200**.

O token antigo deixa de valer no reload do nginx; atualize os clientes logo em seguida.

### G.5 Atualizar o `cloudflared`

Não há repositório do Cloudflare: atualize por RPM com a conferência do SHA-256 publicado (B.9).

```bash
cp -p /usr/bin/cloudflared /root/cloudflared.antes-$(cloudflared --version | awk '{print $3}')   # guarda o binário antigo
echo '<SHA256-PUBLICADO>  cloudflared-linux-x86_64.rpm' | sha256sum -c -
rpm -Uvh cloudflared-linux-x86_64.rpm && systemctl restart cloudflared
systemctl is-active cloudflared; cloudflared --version
```

Confira as conexões do túnel no `journalctl -u cloudflared` e, da internet, `/api/version` -> 401 e `/monitor` -> 403.

### G.6 Atualizar o Ollama

O Ollama é pré-requisito (B.4) e a versão de referência é **0.40.13** (atualizada de 0.40.8 em 2026-10-31). Antes de atualizar: anote `ollama list`, faça backup do `10-performance.conf`, avise os usuários (o restart derruba o que está em execução) e reveja a fila no `/monitor`.
Depois: `systemctl restart ollama && systemctl restart ollama-warm`, `ollama ps` (100% GPU) e um teste de carga curto. Uma versão nova pode mudar o comportamento de slots/raciocínio: valide antes de aceitar.

Roteiro usado na atualização 0.40.8 → 0.40.13 (2026-10-31; a instalação é por tarball em `/usr/local`, não por RPM):

```bash
D=/root/ollama-upd-<versão>; mkdir -p $D; cd $D
curl -fsSL -o sha256sum.txt https://github.com/ollama/ollama/releases/download/v<versão>/sha256sum.txt
curl -fsSL -o ollama-linux-amd64.tar.zst https://github.com/ollama/ollama/releases/download/v<versão>/ollama-linux-amd64.tar.zst
grep 'ollama-linux-amd64.tar.zst$' sha256sum.txt | sha256sum -c -        # tem de dar SUCESSO/OK
B=/root/srv-backup-<data>-ollama-<versão antiga>; mkdir -p $B
cp -a /usr/local/bin/ollama $B/ollama
systemctl stop ollama                                                     # com a fila vazia no /monitor
mv /usr/local/lib/ollama $B/lib-ollama
tar --zstd -xf ollama-linux-amd64.tar.zst -C /usr/local                   # traz bin/ollama e lib/ollama/
restorecon -RF /usr/local/bin/ollama /usr/local/lib/ollama
systemctl start ollama && systemctl restart ollama-warm
curl -s http://127.0.0.1:11434/api/version; ollama ps                     # versão nova; qwen3.8:27b 100% GPU, 64k
# rollback: systemctl stop ollama; rm -rf /usr/local/lib/ollama; mv $B/lib-ollama /usr/local/lib/ollama;
#           cp -a $B/ollama /usr/local/bin/; systemctl start ollama && systemctl restart ollama-warm
```

### G.7 Backup

O que guardar (fora do git; cofre e backup cifrados):

| Item | Onde | Observação |
| --- | --- | --- |
| Token | `/root/API-Key`, `/etc/nginx/ollama/{auth.conf,api-key}` | segredo |
| Chave do certificado | `/etc/nginx/ssl/ia.cslsolucoes.com.br.key` | segredo (o certificado pode ser regerado no painel) |
| Token do túnel | `/etc/cloudflared/token` | segredo (regerável no painel) |
| Senha do admin do monitor | `/etc/mcpollama-gateway/admin` | só o hash |
| Banco do gateway | `/var/lib/mcpollama-gateway/gateway.db` | cópia consistente: `sqlite3 ... ".backup '<destino>'"` com o gateway rodando |
| Chave HMAC dos dados de treino (Fase 12) | `/etc/mcpollama-gateway/doc.key` | segredo; **fora do backup comum**; só por canal protegido; a mesma chave no gateway de fallback |
| `treino.db*` (Fase 12) | `/var/lib/mcpollama-gateway/treino.db` | conteúdo de clientes, sem expiração; **fora do backup comum**, ou só cifrado |
| Configurações | `/etc/systemd/system/{ollama.service.d,nvidia-powercap.service,...}`, `/etc/nginx/`, `/etc/systemd/journald.conf.d/` | também estão em `deploy/servidor-ia/` |

> **Atenção: excluir ou cifrar o `content.db*`.** `/var/lib/mcpollama-gateway/content.db` (e os `-wal`/`-shm`) guarda **prompt e resposta completos** dos últimos 7 dias, incluindo
> documentos e dados de clientes (decisão do owner, 2026-10-30; modo 0600, dono `mcpgw`). **Nunca o inclua num backup em claro.**

---

## H. Solução de problemas

### H.1 Reboots do servidor

**Causa: a fonte de 850 W não aguenta as 4 placas** (decisão do owner; não são os risers). Os reboots são desligamentos secos, sem Xid e sem log, na subida de ocioso para carga
(inclusive durante a carga do modelo no boot e em extrações grandes). Mitigações aplicadas: 100 W por GPU e clock travado em 210 a 1400 MHz (B.3), `ollama-warm` depois do powercap, journald com sync de 10 s.
Diagnóstico: `uptime -s`, `journalctl -b -1 -n 50`, `journalctl -k | grep -iE 'xid|aer'` (vazio = sem erro de GPU) e, no gateway, o evento de reinício e os jobs `interrompido_reboot` (MANUAL, seção 8).
Solução definitiva: fonte maior; depois reveja potência e clock (B.3).

### H.2 `/monitor` sem estilo, parado em "conectando..."

Causa conhecida (corrigida em `7a86350`): o binário saiu de uma cópia com o `index.html` em **CRLF**; a CSP por hash fazia o hash dos bytes brutos e o navegador faz o do texto em LF.
Se aparecer de novo: gere o binário a partir de uma cópia com LF (o `.gitattributes` força `eol=lf`; o instalador também converte os scripts para LF) e use `-Modo Atualizar`.

### H.3 `/monitor` exposto pelo túnel

Já corrigido em 2026-10-30: o loopback era liberado e o túnel entregava o `/monitor` à internet. Hoje o `location /monitor` recusa (**403**) qualquer pedido com `CF-Connecting-IP`, e o loopback deixou de contar como LAN.
Teste sempre depois de mexer no `ollama.conf`: da internet `/monitor` deve dar 403 e, da LAN, 200.

### H.4 Outros sintomas rápidos

| Sintoma | Causa provável | Ação |
| --- | --- | --- |
| nginx sobe e tudo dá 502 | gateway parado (nginx aponta para ele) ou Ollama parado; SELinux sem `httpd_can_network_connect` | `systemctl status mcpollama-gateway ollama`; `getsebool httpd_can_network_connect`; rollback se preciso |
| `401` com token certo | `auth.conf` com valor diferente (`Bearer ` ausente) ou nginx não recarregado | conferir B.7.3; `nginx -t && systemctl reload nginx` e esperar ~5 s |
| `429` | mais de 80 conexões ou 600/min do mesmo IP | reduzir a concorrência do cliente |
| `x509: certificate signed by unknown authority` no cliente | CA ausente/errada na estação | D.3 |
| Cliente da LAN dá timeout em `ia.cslsolucoes.com.br` sem porta | split-horizon: dentro da LAN use `:8443` | usar a porta explícita |
| Modelo recarrega a cada pedido | `ollama-warm` parado, Ollama reiniciado, contexto diferente | `ollama ps`; `systemctl status ollama-warm` |
| Unit com 0 bytes / "masked" depois de queda de energia | corte seco durante a gravação | reinstale a unit do repositório (B.3: `.new`, `test -s`, `mv`, `sync`) |

Tabela de diagnóstico completa do gateway: [MANUAL, seção 10](MANUAL-GATEWAY.md#10-diagnóstico).

### H.5 Pontos marcados "conferir" (resumo)

1. Alias SSH do servidor (`<ALIAS-SSH>`) e o roteador/NAT: dados do guia privado do owner.
2. Método de instalação do driver NVIDIA e o nome da unit `nvidia-persistenced` (B.2); campo do `nvidia-smi -q` para o clock (B.3).
3. `PATH` herdado no `ollama.service` (B.4).
4. Conteúdo exato do `/etc/systemd/journald.conf.d/10-sync.conf` (B.6).
5. Localização da unit do nginx e se o `logrotate` do pacote já cobre o nginx (B.7.1, B.7.8).
6. Validade usada no certificado de origem e o modo SSL/TLS do Cloudflare (B.7.3).
7. Variante temporária do `ollama.conf` (`sed` para `:11434`): derivada neste guia, não existe no repositório (B.7.4).
8. Nome do arquivo do RPM, origem do download, ajuste TLS do destino do túnel no painel e criação do DNS (B.9).
9. Sintaxe da regra e da lista `PERMITIDOS` no roteador (B.10).
10. Origem/receita da `libtesseract64.dll` e branch dos `tessdata` (D.4); origem do gcc em `deps\` (D.1); qual raiz da Origin CA (RSA ou ECC) (D.3).
11. O script `ia-local.ps1` não está neste repositório (E.3).
12. O instalador não cria o `/root/API-Key`: ele é pré-requisito da etapa 2 (B.7.3, C).
