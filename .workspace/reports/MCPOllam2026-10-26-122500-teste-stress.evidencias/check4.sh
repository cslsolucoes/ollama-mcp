F=/etc/systemd/system/ollama.service.d/10-performance.conf
stat -c '%y %n' $F /usr/local/bin/ollama-warm /root/gpu4-backup-20261026/* 2>/dev/null
systemctl show ollama -p NeedDaemonReload -p ActiveEnterTimestamp -p NRestarts
systemctl show ollama -p Environment | tr ' ' '\n' | grep -E 'CUDA|CONTEXT'
ollama show qwen3-ia --parameters | grep num_ctx; grep -o '"num_ctx":[0-9]*' /usr/local/bin/ollama-warm
journalctl -u ollama --since 12:52 --no-pager | grep -E 'found [0-9]+ CUDA|Started|Stopp' | tail -n 5
grep -E '^Oct 26 1(2:5[3-9]|3:0)' /var/log/secure | grep -iE 'Accepted|sudo' | tail -n 6