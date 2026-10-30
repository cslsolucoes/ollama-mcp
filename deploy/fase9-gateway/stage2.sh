#!/bin/bash
# Fase 9.6 - etapa 2 (ativacao): unit do gateway, Ollama so no loopback, nginx -> gateway.
# Pre-requisito: /etc/mcpollama-gateway/admin criado pelo owner (senha digitada sem eco).
# Qualquer falha depois de mexer no nginx/Ollama chama o rollback.
set -euo pipefail
D=/tmp/deploy96; U=http://127.0.0.1:11434; G=http://127.0.0.1:11500
B=/root/srv-backup-20261028/fase96-ativacao-$(date +%Y%m%d-%H%M%S); mkdir -p "$B"
NG=/etc/nginx/conf.d/ollama.conf; PERF=/etc/systemd/system/ollama.service.d/10-performance.conf
echo "boot: $(uptime -s)"
test -s /etc/mcpollama-gateway/admin || { echo "FALTA /etc/mcpollama-gateway/admin (senha do owner)"; exit 1; }
chown root:mcpgw /etc/mcpollama-gateway/admin; chmod 0640 /etc/mcpollama-gateway/admin
cp -p "$NG" "$B/ollama.conf"; cp -p "$PERF" "$B/10-performance.conf"; sync
test -s "$B/ollama.conf"; test -s "$B/10-performance.conf"
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

rollback() { echo "!! FALHA - rollback"; bash "$D/rollback.sh" "$B" || true; exit 1; }
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

# 4) verificacao pelo caminho real (nginx + Bearer + gateway + Ollama)
TOK=$(sed -n 2p /root/API-Key)
R=$(curl -sk -m 60 https://127.0.0.1:8443/api/generate -H "Authorization: Bearer $TOK" -H 'X-Mcpollama-Client: deploy96/check' \
  -D /tmp/deploy96/h.txt -d '{"model":"qwen3-ia","prompt":"Responda apenas: pong","stream":false}')
unset TOK
echo "resposta: $(echo "$R" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(repr(d.get("response")), "eval", d.get("eval_count"), "thinking", len(d.get("thinking") or ""))')"
grep -iE '^x-(request-id|queue-ms|queue-position):' /tmp/deploy96/h.txt | tr -d '\r'
echo "sem token:  $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/version)"
echo "pull:       $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/pull)"
echo "/monitor:   $(curl -sk -m 5 -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/monitor)"
curl -s -m 5 $G/monitor/api/state | python3 -c 'import sys,json; d=json.load(sys.stdin); print("monitor/state chaves:", sorted(d.keys())[:12])'
ollama ps
rm -f /tmp/deploy96/h.txt
echo "ETAPA2_OK (backups em $B)"
