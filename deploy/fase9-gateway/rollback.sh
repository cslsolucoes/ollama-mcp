#!/bin/bash
# Fase 9.6 - rollback: volta o nginx direto ao Ollama e o Ollama a escutar na LAN; para o gateway.
# Uso: bash /tmp/deploy96/rollback.sh <pasta-de-backup-da-etapa2>
set -uo pipefail
B="$1"; NG=/etc/nginx/conf.d/ollama.conf; PERF=/etc/systemd/system/ollama.service.d/10-performance.conf
test -s "$B/ollama.conf" && cp -p "$B/ollama.conf" "$NG.rb" && mv -f "$NG.rb" "$NG"
test -s "$B/10-performance.conf" && cp -p "$B/10-performance.conf" "$PERF.rb" && mv -f "$PERF.rb" "$PERF"
sync
nginx -t && systemctl reload nginx
systemctl daemon-reload; systemctl restart ollama; sleep 3; systemctl restart ollama-warm
curl -sf http://127.0.0.1:11434/api/embed -d '{"model":"nomic-embed-text","input":"warm","keep_alive":-1}' >/dev/null && echo nomic_ok
systemctl disable --now mcpollama-gateway 2>/dev/null || true
ss -tlnH | grep -E '11434|11500' || true
ollama ps
echo "ROLLBACK_FEITO"
