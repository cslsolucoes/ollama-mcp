ls -la /etc/systemd/system/ollama.service.d/
for f in /etc/systemd/system/ollama.service.d/*; do echo "=== $f"; cat -A "$f" | sed 's/\$$//' | grep -viE 'key|token|secret|pass' ; done
echo "=== grep CUDA in etc"; grep -rlE 'CUDA_VISIBLE_DEVICES' /etc/systemd /etc/default /etc/environment /usr/local/bin 2>/dev/null
echo "=== ollama-warm"; grep -nE 'num_ctx|model|curl' /usr/local/bin/ollama-warm
echo "=== modelfile num_ctx"; ollama show qwen3-ia --parameters 2>&1 | head -n 20