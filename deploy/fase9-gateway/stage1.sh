#!/bin/bash
# Fase 9.6 - etapa 1 (preparo, sem mudar o trafego): usuario, diretorios e binario.
# Uso: bash /tmp/deploy96/stage1.sh <sha256-esperado-do-binario>
set -euo pipefail
EXP="$1"; D=/tmp/deploy96; B=/root/srv-backup-20261028/fase96-$(date +%Y%m%d-%H%M%S)
echo "boot: $(uptime -s)"
mkdir -p "$B"
GOT=$(sha256sum "$D/mcpollama-gateway" | cut -d' ' -f1)
[ "$GOT" = "$EXP" ] || { echo "SHA256 do binario nao confere: $GOT"; exit 1; }
id mcpgw >/dev/null 2>&1 || useradd --system --no-create-home --home-dir /var/lib/mcpollama-gateway --shell /sbin/nologin mcpgw
install -d -m 0750 -o mcpgw -g mcpgw /var/lib/mcpollama-gateway
install -d -m 0750 -o root -g mcpgw /etc/mcpollama-gateway
[ -e /usr/local/bin/mcpollama-gateway ] && cp -p /usr/local/bin/mcpollama-gateway "$B/" || true
install -m 0755 -o root -g root "$D/mcpollama-gateway" /usr/local/bin/mcpollama-gateway.new
mv -f /usr/local/bin/mcpollama-gateway.new /usr/local/bin/mcpollama-gateway
sync
/usr/local/bin/mcpollama-gateway --version
echo "backup em $B"
echo "ETAPA1_OK"
