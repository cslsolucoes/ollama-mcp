# Instalação do `mcpollama-gateway` (fila + `/monitor`, Fase 9.6) no servidor de IA

O servidor de IA é o `10.100.2.2` (`lx-ai01`, alias SSH `lx-ai01-root`). O manual completo está em [`docs/MANUAL-GATEWAY.md`](../../docs/MANUAL-GATEWAY.md).

## Instalador (recomendado)

```powershell
cd E:\MCPOllam\deploy\gateway
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1                   # instalação completa
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Atualizar    # só troca o binário
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Senha        # (re)define a senha do admin
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Preparar     # só a etapa 1
powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Reverter -Backup /root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS
```

O modo completo faz, nesta ordem:
1. gera o binário Linux em `D:\MCP\mcpollama-gateway`;
2. monta o pacote com finais de linha LF e o manifesto `SHA256SUMS`;
3. envia o pacote para `/root/deploy96`, pasta só do root (0700);
4. roda a etapa 1;
5. pede a senha do `admin`, se ainda não existir; você digita no seu terminal, sem eco;
6. roda a etapa 2.

Se a ativação falhar, a volta atrás é automática.

## Arquivos

| Arquivo | O que é |
| --- | --- |
| `Instalar-Gateway.ps1` | instalador, executado neste PC |
| `mcpollama-gateway.service` | unit do systemd: usuário `mcpgw`, 127.0.0.1:11500, 1 slot de geração, prioridade alta da LAN |
| `ollama.conf` | `/etc/nginx/conf.d/ollama.conf`: nginx → gateway; `/monitor` só na LAN; IP real do cliente, sem confiar no `CF-Connecting-IP` vindo da LAN |
| `stage1.sh` | **etapa 1, preparo:** usuário, pastas e binário. Não mexe no tráfego |
| `mcpgw-definir-senha.sh` | senha do administrador do `/monitor` (usuário `admin`), digitada sem eco; grava só o hash |
| `stage2.sh` | **etapa 2, ativação:** gateway, Ollama só no loopback, nginx → gateway (`nginx -t`); confere pelo caminho real; volta atrás sozinho se falhar |
| `rollback.sh` | volta o nginx direto ao Ollama e o Ollama para a LAN, e para o gateway. A etapa 2 guarda uma cópia na pasta de backup |

**Binário (não versionado):** `D:\MCP\mcpollama-gateway` (Linux amd64), gerado pelo instalador a partir de `src/`.

**Segurança das etapas:**
- `/root/deploy96` tem de ser root 0700, com todos os arquivos do root;
- o hash do `SHA256SUMS` vem do PC como argumento, e as etapas conferem todos os arquivos contra ele;
- o binário é conferido de novo depois de copiado;
- o rollback roda da cópia guardada no backup;
- o token nunca vai para a linha de comando.

Backups em `/root/srv-backup-20261028/fase96-*`. Detalhes de operação em `CONEXAO-IA.md`. O plano e as decisões estão em
`.workspace/plans/MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md`.
