# Instalação do `mcpollama-gateway` (fila + `/monitor`, Fase 9.6) no servidor de IA

O servidor de IA é o `10.100.2.2` (`lx-ai01`). Esta pasta tem tudo o que é preciso para instalar ou reinstalar o gateway, exceto o binário e os segredos.

| Arquivo | O que é |
| --- | --- |
| `mcpollama-gateway.service` | unit do systemd (usuário `mcpgw`, 127.0.0.1:11500, 1 slot de geração, prioridade alta da LAN) |
| `ollama.conf` | `/etc/nginx/conf.d/ollama.conf` com o nginx apontando para o gateway, `/monitor` só na LAN e IP real do cliente sem confiar no `CF-Connecting-IP` vindo da LAN |
| `stage1.sh` | **etapa 1, preparo:** cria o usuário e as pastas e instala o binário (confere o SHA-256). Não mexe no tráfego |
| `mcpgw-definir-senha.sh` | senha do administrador do `/monitor` (usuário `admin`), digitada pelo owner sem eco; grava só o hash |
| `stage2.sh` | **etapa 2, ativação:** liga o gateway, deixa o Ollama só no loopback e aponta o nginx para o gateway (`nginx -t`); confere pelo caminho real; volta atrás sozinho se falhar |
| `rollback.sh` | volta o nginx direto ao Ollama e o Ollama para a LAN, e para o gateway (`rollback.sh <pasta de backup da etapa 2>`) |

**Binário (não versionado):** `D:\MCP\mcpollama-gateway` (Linux amd64). Para gerar de novo, dentro de `src/`:
`$env:GOOS='linux'; $env:CGO_ENABLED='0'; go build -ldflags "-X main.Version=<versão>" -o D:\MCP\mcpollama-gateway ./cmd/mcpollama-gateway; Remove-Item Env:GOOS,Env:CGO_ENABLED`.
O SHA-256 do binário entra como argumento da `stage1.sh`.

## Ordem

1. Copie esta pasta e o binário para `/tmp/deploy96/` no servidor (`scp`).
2. Rode `sed -i 's/\r$//' /tmp/deploy96/*`, porque arquivos vindos do Windows podem ter CRLF.
3. Rode `bash /tmp/deploy96/stage1.sh <sha256 do binário>`.
4. **O owner** roda `bash /root/mcpgw-definir-senha.sh`. Só é preciso na primeira vez ou para trocar a senha.
5. Rode `bash /tmp/deploy96/stage2.sh`, que imprime a pasta de backup usada pelo `rollback.sh`.

Os backups ficam em `/root/srv-backup-20261028/fase96-*`. Detalhes de operação em `CONEXAO-IA.md`.
O plano e as decisões estão em `.workspace/plans/MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md`.
