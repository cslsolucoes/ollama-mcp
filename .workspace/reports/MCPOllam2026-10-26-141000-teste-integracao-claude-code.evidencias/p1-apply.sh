set -e
B=/root/gpu4-backup-20261026; F=/etc/systemd/system/ollama.service.d/10-performance.conf
test -s $F
cp $F $B/10-performance.conf.maxloaded1; sync
grep -c '^Environment="OLLAMA_MAX_LOADED_MODELS=1"$' $F
sed 's|^Environment="OLLAMA_MAX_LOADED_MODELS=1"$|# 2 residentes (2026-10-26): qwen3-ia + nomic-embed-text; evita recarga de ~18 GB a cada embed.\nEnvironment="OLLAMA_MAX_LOADED_MODELS=2"|' $F > $F.new
grep -q 'OLLAMA_MAX_LOADED_MODELS=2' $F.new
mv -f $F.new $F; chmod 644 $F; sync
ls -l $F; grep -nE 'MAX_LOADED|CUDA|CONTEXT' $F
systemctl daemon-reload
systemctl restart ollama
for i in $(seq 1 30); do curl -sf http://127.0.0.1:11434/api/version >/dev/null && break; sleep 1; done
systemctl show ollama -p Environment | tr ' ' '\n' | grep MAX_LOADED
curl -sf -m 300 http://127.0.0.1:11434/api/generate -d '{"model":"qwen3-ia","prompt":"hi","stream":false,"think":false,"options":{"num_predict":1}}' | grep -oE '"load_duration":[0-9]+'
ollama ps | tail -n +2
echo "1b.4=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable)"
sync