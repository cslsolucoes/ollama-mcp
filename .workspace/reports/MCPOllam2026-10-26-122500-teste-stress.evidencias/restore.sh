set -e
B=/root/gpu4-backup-20261026
cp -a /etc/systemd/system/ollama.service.d/10-performance.conf $B/10-performance.conf.4gpu-131k-falhou
cp -a $B/10-performance.conf /etc/systemd/system/ollama.service.d/10-performance.conf
cp -a $B/ollama-warm /usr/local/bin/ollama-warm
grep -nE 'CONTEXT|CUDA' /etc/systemd/system/ollama.service.d/10-performance.conf
grep -n num_ctx /usr/local/bin/ollama-warm
grep -n num_ctx $B/qwen3-ia.Modelfile
ollama create qwen3-ia -f $B/qwen3-ia.Modelfile 2>&1 | tail -n 1
ollama show qwen3-ia --parameters | grep num_ctx
systemctl daemon-reload
systemctl restart ollama
for i in $(seq 1 30); do curl -sf http://127.0.0.1:11434/api/version >/dev/null && break; sleep 1; done
T0=$(date +%s)
curl -sf -m 300 http://127.0.0.1:11434/api/generate -d '{"model":"qwen3-ia","prompt":"hi","stream":false,"think":false,"options":{"num_ctx":65536,"num_predict":1}}' | grep -oE '"load_duration":[0-9]+'
echo "load_wall_s=$(( $(date +%s) - T0 ))"
ollama ps | tail -n +2
nvidia-smi --query-gpu=index,pci.bus_id,memory.used --format=csv,noheader
journalctl -u ollama --since "-3min" --no-pager | grep -E 'found [0-9]+ CUDA devices|offloaded|n_ctx  ' | tail -n 4