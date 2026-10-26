set -e
systemctl daemon-reload
systemctl show ollama -p Environment | tr ' ' '\n' | grep -E 'CUDA|CONTEXT'
systemctl restart ollama
for i in $(seq 1 30); do curl -sf http://127.0.0.1:11434/api/version >/dev/null && break; sleep 1; done
curl -sf -m 300 http://127.0.0.1:11434/api/generate -d '{"model":"qwen3-ia","prompt":"hi","stream":false,"think":false,"options":{"num_ctx":65536,"num_predict":1}}' | grep -oE '"load_duration":[0-9]+'
ollama ps | tail -n +2
nvidia-smi --query-gpu=index,pci.bus_id,memory.used --format=csv,noheader
journalctl -u ollama --since "-2min" --no-pager | grep -E 'found [0-9]+ CUDA|offloaded|projected|n_ctx  ' | tail -n 4
echo "1b.4 corr=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable)"