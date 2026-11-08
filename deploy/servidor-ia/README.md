# Arquivos de referência do servidor de IA

Cópias dos arquivos de configuração do servidor de IA (`ia.cslsolucoes.com.br`, CentOS Stream 10), **coletadas do servidor em 2026-10-30**.
Servem para reconstruir o servidor e para comparar com o que está em produção. O passo a passo de uso está em [INSTALACAO.md](../../docs/INSTALACAO.md) (parte B).

Regras:

- **São cópias, não a fonte.** A produção é o servidor; se alterar lá, atualize aqui.
- **Sem segredos.** O token, a chave do certificado e o token do túnel **não** estão aqui e nunca devem entrar no git.
- **Finais de linha LF** (o repositório força `eol=lf`). Envie ao servidor sem converter para CRLF.
- Ao instalar um arquivo de unit, grave em `.new`, confira `test -s` e só então `mv` + `sync` (unit vazia vira "mascarada" depois de queda de energia).

## Arquivos da raiz

| Arquivo | Destino no servidor | Dono / modo | Descrição |
| --- | --- | --- | --- |
| `ollama.service` | `/etc/systemd/system/ollama.service` | root, 0644 | unit base do Ollama (usuário `ollama`, `Restart=always`). O `Environment PATH` herda caminhos de `/root/.opencode` que não são necessários (conferir) |
| `ollama-10-performance.conf` | `/etc/systemd/system/ollama.service.d/10-performance.conf` | root, 0644 | drop-in de desempenho: `OLLAMA_HOST=127.0.0.1:11434`, contexto 98304 (96k; 65536 até 2026-11-05 12:23), KV `q4_0`, `NUM_PARALLEL=1` (o `qwen3-ia` segue com 1 slot: a arquitetura `qwen35` não aceita pedidos paralelos; owner, 2026-11-04), `MAX_QUEUE=32`, `MAX_LOADED_MODELS=2`, `KEEP_ALIVE=-1`, `CUDA_VISIBLE_DEVICES` por UUID. **Os UUIDs são das GPUs deste servidor: troque pelos de `nvidia-smi -L`** |
| `ollama-warm.service` | `/etc/systemd/system/ollama-warm.service` | root, 0644 | oneshot que carrega o `qwen3.8:27b` a 128k no boot (96k de 2026-11-05 a 2026-11-08; 64k antes) (até 2026-11-05: `qwen3-ia`); `After=ollama.service nvidia-powercap.service` |
| `ollama-warm.sh` | `/usr/local/bin/ollama-warm` | root, 0755 | espera o Ollama e faz um `/api/generate` mínimo (`think:false`, `num_predict:1`). **Desde 2026-11-08 não manda `num_ctx`**: o contexto vem do modelo (antes `num_ctx:98304`) |
| `nvidia-powercap.service` | `/etc/systemd/system/nvidia-powercap.service` | root, 0644 | por GPU, desde 2026-11-08: persistence mode, `-pl 240` (máximo das placas), `-rgc` (clock liberado) e offsets de memória +1500 MHz e núcleo +150 MHz por `/usr/local/bin/oc`. Até 2026-11-08: `-pl 100` e `-lgc 210,1400` (motivo: fonte de 850 W com 4 placas; resolvido com a 2ª fonte de 500 W) |
| `Modelfile.qwen3.8-27b-128k` | (usado no `ollama create`, não é instalado) | | recria o `qwen3.8:27b` com `PARAMETER num_ctx 131072` e `PARAMETER num_gpu 66` embutidos (o gateway tira `num_ctx`/`num_gpu` do pedido do cliente, então o parâmetro tem de estar no modelo). Digest `aaee06c39dcf` → `40e90b71222a` |
| `ferramentas-gpu/` | `/root/pl-test/` (testes; `oc` em `/usr/local/bin/oc`) | | ferramentas de teste e ajuste das GPUs: `oc.c` (offsets NVML, vira `/usr/local/bin/oc`), `memtest.cu`, `cb.cu`, `fan.c`, `burn.cu` e os scripts `ramp.sh`, `osc4.sh`, `core3.sh`, `ctx2.sh`, `ctx3.sh`, `load.sh`, `genpf.py` |
| `nginx-cloudflare-allow.conf` | `/etc/nginx/ollama/cloudflare-allow.conf` | root, 0644 | faixas de IP da borda do Cloudflare (`allow ...;`); incluída pelo `ollama.conf` |
| `nginx-auth.conf.modelo` | `/etc/nginx/ollama/auth.conf` | root, **0600** | modelo do mapa de autenticação. Veja a nota abaixo |
| `cloudflared.service` | `/etc/systemd/system/cloudflared.service` | root, 0644 | túnel do Cloudflare: `cloudflared ... tunnel run --token-file /etc/cloudflared/token` |
| `inventario.txt` | (não é instalado) | | versões e estado do servidor na coleta: SO, kernel, Ollama 0.40.8, driver 610.57.04, nginx 1.26.3, cloudflared 2026.10.6, firewall, modelos |

### Nota sobre `nginx-auth.conf.modelo`

O arquivo é um **modelo**: o valor do token aparece como `"<TOKEN>"`. No `auth.conf` real, o `map` compara o cabeçalho `Authorization` **inteiro**, então o valor é
**`"Bearer <TOKEN>"`** (com o prefixo `Bearer `):

```nginx
map $http_authorization $ollama_authorized {
    default 0;
    "Bearer <TOKEN>" 1;
}
```

Gere o token com `openssl rand -hex 32`, grave o arquivo como root com modo 0600 e **nunca** o versione. O script `ollama-key-rotate` (em `nginx/`) reescreve o arquivo com a variante
por expressão regular `"~*^Bearer[[:space:]]+<TOKEN>$"`, equivalente para o cliente.

## Reverter (mudanças de 2026-11-08)

Contexto 128k (volta ao modelo original a 96k):

```bash
ollama stop qwen3.8:27b
ollama cp qwen3.8:27b-orig qwen3.8:27b      # a tag local qwen3.8:27b-orig guarda o modelo original (digest aaee06c39dcf)
cp /root/ollama-warm.bak-20261108-1705 /usr/local/bin/ollama-warm   # volta a mandar num_ctx 98304
```

Power cap e overclock (volta ao estado anterior; backups feitos no servidor antes de cada edição):

```bash
cp /root/nvidia-powercap.service.bak-20261108-1618 /etc/systemd/system/nvidia-powercap.service   # ou o -1633, mais recente
systemctl daemon-reload; systemctl restart nvidia-powercap
```

A unit antiga voltava a `-pl 100` e `-lgc 210,1400`. Só faça isso com a 2ª fonte desligada ou se houver reboots secos sob carga.

## Subpasta `nginx/`

Arquivos do nginx coletados na mesma data. O `ollama.conf` do nginx **não** está aqui: está em [`../gateway/ollama.conf`](../gateway/ollama.conf), porque o instalador do gateway o implanta.

| Arquivo | Destino no servidor | Dono / modo | Descrição |
| --- | --- | --- | --- |
| `nginx/nginx.conf` | `/etc/nginx/nginx.conf` | root, 0644 | configuração principal (padrão do pacote, sem listener em texto claro; `include /etc/nginx/conf.d/*.conf`) |
| `nginx/nginx.service` | `/etc/systemd/system/nginx.service` (conferir se a de produção é esta ou a do pacote) | root, 0644 | unit `Type=forking`, com `nginx -t` antes de subir |
| `nginx/conf.d-lista.txt` | (não é instalado) | | listagem de `/etc/nginx/conf.d/`: `ollama.conf` e um backup `.bak` (mantenha backups fora do `conf.d/` ou sem final `.conf`) |
| `nginx/logrotate-nginx` | `/etc/logrotate.d/nginx` | root, 0644 | rotação diária, 10 cópias, comprimida, `USR1` no `postrotate` |
| `nginx/ollama-key-rotate.sh` | `/usr/local/bin/ollama-key-rotate` | root, 0755 | gera um token novo, reescreve `auth.conf` e `api-key` (0600), `nginx -t`, `reload`, e **imprime o token na tela** |
| `nginx/permissoes.txt` | (não é instalado) | | dono e modo de `/etc/nginx/ollama/` e `/etc/nginx/ssl/` (a chave `.key` e o `auth.conf` em 0600) |
| `nginx/selinux.txt` | (não é instalado) | | SELinux `Enforcing`; `httpd_can_network_connect` on; 8443 já é `http_port_t`; sem módulo local |

## Arquivos que **não** estão aqui (por serem segredo)

| Caminho no servidor | O que é |
| --- | --- |
| `/root/API-Key` | token (na 2ª linha), fonte lida pelo `stage2.sh` do gateway |
| `/etc/nginx/ollama/api-key` | cópia do token usada pelo nginx/rotação (0600) |
| `/etc/nginx/ssl/ia.cslsolucoes.com.br.key` e `.pem` | chave e certificado da Cloudflare Origin CA (a `.key` em 0600) |
| `/etc/cloudflared/token` | token do túnel (0600) |
| `/etc/mcpollama-gateway/admin` | hash da senha do admin do `/monitor` (0600, `mcpgw`) |
