#!/bin/bash
# Fase 9.6 - etapa 1 (preparo, sem mudar o trafego): usuario, diretorios e binario.
# Uso: bash /root/deploy96/stage1.sh <sha256 do arquivo SHA256SUMS, calculado no PC de origem>
# Seguranca (revisao 2026-10-30): pasta so do root (0700); manifesto SHA-256 de TODOS os
# arquivos, ancorado no hash passado como argumento; o binario e conferido de novo ja
# copiado (.new), antes do mv, para nao haver troca entre a conferencia e a instalacao.
set -euo pipefail
EXP_SUMS="$1"; D=/root/deploy96; B=/root/srv-backup-20261028/fase96-$(date +%Y%m%d-%H%M%S)
[ "$(id -u)" -eq 0 ] || { echo "rode como root"; exit 1; }
[ "$(stat -c '%U %a' "$D")" = "root 700" ] || { echo "$D precisa ser root 0700"; exit 1; }
for f in "$D"/*; do [ "$(stat -c '%U' "$f")" = "root" ] || { echo "dono errado: $f"; exit 1; }; done
[ "$(sha256sum "$D/SHA256SUMS" | cut -d' ' -f1)" = "$EXP_SUMS" ] || { echo "SHA256SUMS nao confere"; exit 1; }
(cd "$D" && sha256sum --quiet -c SHA256SUMS) || { echo "algum arquivo nao confere com o SHA256SUMS"; exit 1; }
EXP_BIN=$(awk '$2=="mcpollama-gateway"{print $1}' "$D/SHA256SUMS")
echo "boot: $(uptime -s)"
mkdir -p "$B"
id mcpgw >/dev/null 2>&1 || useradd --system --no-create-home --home-dir /var/lib/mcpollama-gateway --shell /sbin/nologin mcpgw
install -d -m 0750 -o mcpgw -g mcpgw /var/lib/mcpollama-gateway
install -d -m 0750 -o root -g mcpgw /etc/mcpollama-gateway
[ -e /usr/local/bin/mcpollama-gateway ] && cp -p /usr/local/bin/mcpollama-gateway "$B/" || true
install -m 0755 -o root -g root "$D/mcpollama-gateway" /usr/local/bin/mcpollama-gateway.new
[ "$(sha256sum /usr/local/bin/mcpollama-gateway.new | cut -d' ' -f1)" = "$EXP_BIN" ] || { rm -f /usr/local/bin/mcpollama-gateway.new; echo "binario copiado nao confere"; exit 1; }
mv -f /usr/local/bin/mcpollama-gateway.new /usr/local/bin/mcpollama-gateway
sync
/usr/local/bin/mcpollama-gateway --version
echo "backup em $B"
echo "ETAPA1_OK"
