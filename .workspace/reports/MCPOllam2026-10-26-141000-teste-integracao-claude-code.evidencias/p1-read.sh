date '+%F %T'; uptime -s
echo "== env"; systemctl show ollama -p Environment | tr ' ' '\n' | grep -E 'MAX_LOADED|KEEP_ALIVE|SCHED_SPREAD|CUDA|CONTEXT|NUM_PARALLEL'
systemctl show ollama -p NeedDaemonReload -p ActiveEnterTimestamp
echo "== ollama list"; ollama list
echo "== ollama ps"; ollama ps
echo "== vram"; nvidia-smi --query-gpu=index,memory.used,memory.total,power.limit --format=csv,noheader
echo "== base model history"; ls -l --time-style=+%F_%T /usr/share/ollama/.ollama/models/manifests/registry.ollama.ai/library/*/ 2>/dev/null
journalctl -u ollama --since today --no-pager | grep -E 'POST +"/api/(pull|create|delete)"' | tail -n 10
echo "== 1b.4 AER"; awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable