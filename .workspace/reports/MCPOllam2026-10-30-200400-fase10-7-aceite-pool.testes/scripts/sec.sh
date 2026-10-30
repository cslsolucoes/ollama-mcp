#!/bin/bash
# 10.7 - secundario temporario (ollama na CPU, 127.0.0.1:11435). start | stop | proof
U=ollama-sec-test
case "$1" in
start)
  systemctl is-active --quiet $U && { echo "sec: ja ativo"; exit 0; }
  systemctl reset-failed $U 2>/dev/null
  systemd-run --unit=$U --description="10.7 aceite: ollama CPU temporaria 11435" \
    --uid=ollama --gid=ollama -p DevicePolicy=closed -p Nice=10 -p "CPUAffinity=2-5 8-11" -p CPUQuota=300% \
    --setenv=HOME=/usr/share/ollama --setenv=OLLAMA_HOST=127.0.0.1:11435 \
    --setenv=CUDA_VISIBLE_DEVICES=-1 --setenv=HIP_VISIBLE_DEVICES=-1 --setenv=ROCR_VISIBLE_DEVICES=-1 --setenv=GGML_VK_VISIBLE_DEVICES=-1 \
    --setenv=OLLAMA_MODELS=/usr/share/ollama/.ollama/models --setenv=OLLAMA_NOPRUNE=1 \
    --setenv=OLLAMA_NUM_PARALLEL=1 --setenv=OLLAMA_MAX_LOADED_MODELS=1 --setenv=OLLAMA_KEEP_ALIVE=10m \
    --setenv=OLLAMA_CONTEXT_LENGTH=8192 --setenv=OLLAMA_NO_CLOUD=true \
    /usr/local/bin/ollama serve
  for i in $(seq 1 30); do curl -s -m 2 127.0.0.1:11435/api/version >/dev/null && break; sleep 1; done
  echo "sec: $(curl -s -m 2 127.0.0.1:11435/api/version) ativo=$(systemctl is-active $U)"
  ;;
stop)
  systemctl stop $U 2>/dev/null; systemctl reset-failed $U 2>/dev/null
  echo "sec: parado ($(systemctl is-active $U 2>&1))"
  ;;
proof)
  echo "-- journal inference compute:"
  journalctl -u $U --no-pager -o cat | grep -E "inference compute|total_vram|library=" | tail -4
  echo "-- ollama ps 11435:"; OLLAMA_HOST=127.0.0.1:11435 ollama ps
  echo "-- /dev/nvidia* abertos pelos processos da unidade:"
  n=0; for p in $(systemctl show -p MainPID --value $U) $(pgrep -P "$(systemctl show -p MainPID --value $U)"); do
    [ "$p" = 0 ] && continue; c=$(ls -l /proc/$p/fd 2>/dev/null | grep -c nvidia); echo "   pid $p: $c"; n=$((n+c)); done
  echo "   total=$n"
  echo "-- cgroup: $(systemctl show -p DevicePolicy -p CPUAffinity -p CPUQuotaPerSecUSec $U | tr '\n' ' ')"
  echo "-- nvidia-smi compute apps:"; nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader
  ;;
esac
