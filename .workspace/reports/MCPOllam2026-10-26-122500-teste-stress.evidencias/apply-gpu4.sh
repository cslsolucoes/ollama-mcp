set -e
B=/root/gpu4-backup-20261026; mkdir -p $B
cp -a /etc/systemd/system/ollama.service.d/10-performance.conf $B/
cp -a /usr/local/bin/ollama-warm $B/
ollama show qwen3-ia --modelfile > $B/qwen3-ia.Modelfile
echo "backup ok: $(ls $B | tr '\n' ' ')"
UUIDS="GPU-13ce3be3-9fa8-3729-491f-3add358bbf81,GPU-13d97d49-7bdf-79b2-7994-28c4dc0e2ac8,GPU-5f254212-4068-1e03-d590-d5759554409e,GPU-d7166024-7c32-3cb8-6e85-4e9188e933e7"
F=/etc/systemd/system/ollama.service.d/10-performance.conf
sed -i "s|^# Contexto 32k (pedido do owner 2026-10-23).*|# Contexto 128k (2026-10-26: 4 GPUs de novo, GPU3 em slot direto 00:1c.3 apos manutencao do owner).|" $F
sed -i 's|^Environment="OLLAMA_CONTEXT_LENGTH=65536"|Environment="OLLAMA_CONTEXT_LENGTH=131072"|' $F
sed -i "s|^# Usar as 4x RTX 3070.*|# 4x RTX 3070 (32 GB) por UUID (o indice muda se a topologia PCIe mudar); camadas espalhadas -> 100% GPU.|" $F
sed -i "s|^Environment=\"CUDA_VISIBLE_DEVICES=0,1,2\"|Environment=\"CUDA_VISIBLE_DEVICES=$UUIDS\"|" $F
grep -nE 'CONTEXT|CUDA|^# (Contexto|4x)' $F
sed -i 's|"num_ctx":65536|"num_ctx":131072|' /usr/local/bin/ollama-warm
grep -n 'num_ctx' /usr/local/bin/ollama-warm
sed 's/^PARAMETER num_ctx 65536$/PARAMETER num_ctx 131072/' $B/qwen3-ia.Modelfile > /root/qwen3-ia-131k.Modelfile
grep -n 'num_ctx' /root/qwen3-ia-131k.Modelfile
ollama create qwen3-ia -f /root/qwen3-ia-131k.Modelfile 2>&1 | tail -n 2
ollama show qwen3-ia --parameters | grep num_ctx
systemctl daemon-reload
systemctl restart ollama
for i in $(seq 1 30); do curl -sf http://127.0.0.1:11434/api/version >/dev/null && break; sleep 1; done
T0=$(date +%s)
curl -sf -m 300 http://127.0.0.1:11434/api/generate -d '{"model":"qwen3-ia","prompt":"hi","stream":false,"think":false,"options":{"num_predict":1}}' | grep -oE '"load_duration":[0-9]+|"total_duration":[0-9]+'
echo "load_wall_s=$(( $(date +%s) - T0 ))"
ollama ps
nvidia-smi --query-gpu=index,pci.bus_id,memory.used,memory.total,temperature.gpu --format=csv,noheader
journalctl -u ollama --since "-3min" --no-pager | grep -E 'found [0-9]+ CUDA devices|offloaded|cannot meet|failed to fit|projected to use|n_ctx  |llama_context: n_ctx|error' | tail -n 12
echo "Xid=$(journalctl -b 0 -k --no-pager | grep -ciE 'NVRM: Xid|GPU lost|uvm.*fatal')"
for p in 00:01.0 00:1b.4 00:1c.2 00:1c.3; do echo "$p corr=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_correctable 2>/dev/null) nonfatal=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_nonfatal 2>/dev/null) fatal=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_fatal 2>/dev/null)"; done