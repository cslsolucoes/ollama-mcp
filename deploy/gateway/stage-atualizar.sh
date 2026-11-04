#!/bin/bash
# Fase 12 - atualizacao do gateway com troca da UNIT (usada pelo Instalar-Gateway.ps1 -Modo Atualizar, depois da etapa 1).
# Uso: bash /root/deploy96/stage-atualizar.sh <sha256 do arquivo SHA256SUMS, calculado no PC de origem>
# Faz: confere o pacote, guarda a unit atual, instala a unit nova SE for diferente (daemon-reload), reinicia o gateway e confere.
# NAO mexe no nginx nem no Ollama (nao recarrega o modelo). Se o gateway nao subir, devolve a unit e o binario anteriores
# (o binario antigo foi guardado pela etapa 1 na pasta fase96-AAAAMMDD-HHMMSS mais recente) e reinicia de novo.
# O restart corta quem esta em execucao e quem espera na fila: rode com a fila vazia.
set -euo pipefail
EXP_SUMS="$1"; D=/root/deploy96; G=http://127.0.0.1:11500; U=/etc/systemd/system/mcpollama-gateway.service
BIN=/usr/local/bin/mcpollama-gateway
[ "$(id -u)" -eq 0 ] || { echo "rode como root"; exit 1; }
[ "$(stat -c '%U %a' "$D")" = "root 700" ] || { echo "$D precisa ser root 0700"; exit 1; }
[ "$(sha256sum "$D/SHA256SUMS" | cut -d' ' -f1)" = "$EXP_SUMS" ] || { echo "SHA256SUMS nao confere"; exit 1; }
(cd "$D" && sha256sum --quiet -c SHA256SUMS) || { echo "algum arquivo nao confere com o SHA256SUMS"; exit 1; }
test -s /etc/mcpollama-gateway/doc.key || { echo "FALTA /etc/mcpollama-gateway/doc.key (a etapa 1 a cria)"; exit 1; }
B=/root/srv-backup-20261028/fase96-unit-$(date +%Y%m%d-%H%M%S); mkdir -m 0700 -p "$B"
PREV=$(ls -dt /root/srv-backup-20261028/fase96-[0-9]* 2>/dev/null | while read -r d; do [ -f "$d/mcpollama-gateway" ] && { echo "$d"; break; }; done || true)
[ -e "$U" ] && cp -p "$U" "$B/mcpollama-gateway.service"
CHANGED=0
if ! cmp -s "$D/mcpollama-gateway.service" "$U"; then
  install -m 0644 -o root -g root "$D/mcpollama-gateway.service" "$U.new"; mv -f "$U.new" "$U"; sync
  CHANGED=1; echo "unit: ATUALIZADA (a antiga esta em $B)"
else
  echo "unit: ja estava igual"
fi
revert() {
  echo "!! FALHA - devolvendo a unit e o binario anteriores"
  [ -s "$B/mcpollama-gateway.service" ] && install -m 0644 -o root -g root "$B/mcpollama-gateway.service" "$U"
  [ -n "$PREV" ] && [ -f "$PREV/mcpollama-gateway" ] && install -m 0755 -o root -g root "$PREV/mcpollama-gateway" "$BIN"
  systemctl daemon-reload; systemctl restart mcpollama-gateway || true
  exit 1
}
trap revert ERR
systemctl daemon-reload
systemctl restart mcpollama-gateway
sleep 3
systemctl is-active mcpollama-gateway
"$BIN" --version
echo "gateway /api/version: $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/api/version)"
echo "gateway /monitor:     $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/monitor)"
echo "gateway /monitor/api/treino: $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/monitor/api/treino)  (Fase 12; 200 = treino.db aberto, 503 = desligado)"
curl -s -m 5 -o /dev/null -w '%{http_code}' $G/api/version | grep -q '^200$'
trap - ERR
echo "unit alterada: $CHANGED; backup em $B"
echo "ATUALIZAR_OK"
