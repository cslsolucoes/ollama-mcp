#!/bin/bash
# Fase 9.6 - etapa 2 (ativacao): unit do gateway, Ollama so no loopback, nginx -> gateway.
# Uso: bash /root/deploy96/stage2.sh <sha256 do arquivo SHA256SUMS, calculado no PC de origem>
# Pre-requisito: /etc/mcpollama-gateway/admin criado pelo owner (bash /root/mcpgw-definir-senha.sh).
# Qualquer falha depois de mexer no nginx/Ollama chama o rollback (a copia guardada no backup).
# Seguranca (revisao 2026-10-30): pasta so do root (0700), manifesto SHA-256 conferido de novo
# antes de instalar, rollback executado da copia no backup e token passado ao curl pelo stdin
# (nunca na linha de comando, que aparece no ps).
set -euo pipefail
EXP_SUMS="$1"; D=/root/deploy96; U=http://127.0.0.1:11434; G=http://127.0.0.1:11500
NG=/etc/nginx/conf.d/ollama.conf; PERF=/etc/systemd/system/ollama.service.d/10-performance.conf
[ "$(id -u)" -eq 0 ] || { echo "rode como root"; exit 1; }
[ "$(stat -c '%U %a' "$D")" = "root 700" ] || { echo "$D precisa ser root 0700"; exit 1; }
[ "$(sha256sum "$D/SHA256SUMS" | cut -d' ' -f1)" = "$EXP_SUMS" ] || { echo "SHA256SUMS nao confere"; exit 1; }
(cd "$D" && sha256sum --quiet -c SHA256SUMS) || { echo "algum arquivo nao confere com o SHA256SUMS"; exit 1; }
B=/root/srv-backup-20261028/fase96-ativacao-$(date +%Y%m%d-%H%M%S); mkdir -m 0700 -p "$B"
echo "boot: $(uptime -s)"
test -s /etc/mcpollama-gateway/admin || { echo "FALTA /etc/mcpollama-gateway/admin (bash /root/mcpgw-definir-senha.sh)"; exit 1; }
# o gateway (usuario mcpgw) le o arquivo e recusa qualquer permissao de grupo/outros (admin.go: 0600)
chown mcpgw:mcpgw /etc/mcpollama-gateway/admin; chmod 0600 /etc/mcpollama-gateway/admin
# Fase 12: a unit aponta MCPGW_DOC_KEY_FILE para este arquivo (criado pela etapa 1; nunca sobrescrito)
test -s /etc/mcpollama-gateway/doc.key || { echo "FALTA /etc/mcpollama-gateway/doc.key (rode a etapa 1: stage1.sh a cria)"; exit 1; }
[ "$(stat -c '%U:%G %a' /etc/mcpollama-gateway/doc.key)" = "mcpgw:mcpgw 600" ] || { echo "doc.key precisa ser mcpgw:mcpgw 0600"; exit 1; }
cp -p "$NG" "$B/ollama.conf"; cp -p "$PERF" "$B/10-performance.conf"; cp -p "$D/rollback.sh" "$B/rollback.sh"; sync
test -s "$B/ollama.conf"; test -s "$B/10-performance.conf"; test -s "$B/rollback.sh"
echo "backups em $B"

# 1) gateway (ainda sem trafego: o nginx aponta para o Ollama)
install -m 0644 -o root -g root "$D/mcpollama-gateway.service" /etc/systemd/system/mcpollama-gateway.service.new
mv -f /etc/systemd/system/mcpollama-gateway.service.new /etc/systemd/system/mcpollama-gateway.service; sync
systemctl daemon-reload
systemctl enable --now mcpollama-gateway
sleep 2
systemctl is-active mcpollama-gateway
echo "gateway /api/version: $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/api/version)"
echo "gateway /monitor:     $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/monitor)"
echo "gateway /monitor/api/treino: $(curl -s -m 5 -o /dev/null -w '%{http_code}' $G/monitor/api/treino)  (Fase 12; 200 = treino.db aberto)"

rollback() { echo "!! FALHA - rollback"; bash "$B/rollback.sh" "$B" || true; exit 1; }
trap rollback ERR

# 2) Ollama so no loopback (D9-2)
sed 's|^Environment="OLLAMA_HOST=0.0.0.0:11434"|# Fase 9.6 (owner 2026-10-29, D9-2): so loopback; todo acesso passa pelo nginx + gateway\nEnvironment="OLLAMA_HOST=127.0.0.1:11434"|' "$PERF" > "$PERF.new"
test -s "$PERF.new"; grep -q 'OLLAMA_HOST=127.0.0.1:11434' "$PERF.new"; mv -f "$PERF.new" "$PERF"; sync
systemctl daemon-reload; systemctl restart ollama; sleep 3; systemctl restart ollama-warm
curl -sf $U/api/embed -d '{"model":"nomic-embed-text","input":"warm","keep_alive":-1}' >/dev/null
ss -tlnH | grep 11434

# 3) nginx -> gateway
install -m 0644 -o root -g root "$D/ollama.conf" "$NG.new"; mv -f "$NG.new" "$NG"; sync
nginx -t
systemctl reload nginx
trap - ERR
# o reload troca os workers de forma gradual: sem esta espera, a 1a conferencia pode cair num worker
# antigo (que ainda aponta direto ao Ollama) - aconteceu na ativacao de 2026-10-30 12:12
sleep 5

# 4) verificacao pelo caminho real (nginx + Bearer + gateway + Ollama); token so pelo stdin
R=$(printf 'Authorization: Bearer %s\n' "$(sed -n 2p /root/API-Key)" | curl -sk -m 60 https://127.0.0.1:8443/api/generate -H @- \
  -H 'X-Mcpollama-Client: deploy96/check' -D "$B/h.txt" -d '{"model":"qwen3.8:27b","prompt":"Responda apenas: pong","stream":false}')
echo "resposta: $(echo "$R" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(repr(d.get("response")), "eval", d.get("eval_count"), "thinking", len(d.get("thinking") or ""))')"
grep -iE '^x-(request-id|queue-ms|queue-position):' "$B/h.txt" | tr -d '\r' || echo "AVISO: resposta sem os headers da fila (nginx ainda nao recarregou?) - confira de novo"; rm -f "$B/h.txt"
echo "sem token:  $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/version)"
echo "pull:       $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/pull)"
echo "/monitor:   $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/monitor)"
curl -s -m 5 $G/monitor/api/state | python3 -c 'import sys,json; d=json.load(sys.stdin); print("monitor/state chaves:", sorted(d.keys())[:12])'
ollama ps
echo "ETAPA2_OK (backups e rollback em $B: bash $B/rollback.sh $B)"
