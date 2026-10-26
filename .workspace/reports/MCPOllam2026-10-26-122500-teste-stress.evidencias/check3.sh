date '+%F %T'; uptime -s; last -x -n 4 reboot shutdown | head -n 4
nvidia-smi --query-gpu=index,pci.bus_id,uuid,memory.used --format=csv,noheader
for b in $(nvidia-smi --query-gpu=pci.bus_id --format=csv,noheader); do readlink -f /sys/bus/pci/devices/$(echo $b | tr 'A-F' 'a-f' | sed 's/^00000000/0000/'); done
for p in /sys/bus/pci/devices/0000:00:*; do [ -f $p/aer_dev_correctable ] || continue; echo "$(basename $p) corr=$(awk '/TOTAL/{print $2}' $p/aer_dev_correctable)"; done
grep -E 'CONTEXT|CUDA' /etc/systemd/system/ollama.service.d/10-performance.conf; ollama ps | tail -n +2