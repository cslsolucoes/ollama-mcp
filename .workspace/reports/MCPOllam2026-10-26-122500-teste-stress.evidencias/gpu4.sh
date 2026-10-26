echo "== now/uptime"; date '+%F %T'; uptime -s
echo "== boots"; last -x -n 4 reboot shutdown | head -n 4
echo "== nvidia-smi"; nvidia-smi --query-gpu=index,pci.bus_id,uuid,pcie.link.gen.current,pcie.link.gen.max,pcie.link.width.current,pcie.link.width.max,power.limit,temperature.gpu,memory.used,memory.total,utilization.gpu --format=csv,noheader
echo "== root ports"; for b in $(nvidia-smi --query-gpu=pci.bus_id --format=csv,noheader); do readlink -f /sys/bus/pci/devices/$(echo $b | tr 'A-F' 'a-f' | sed 's/^00000000/0000/'); done
echo "== ollama unit env"; systemctl show ollama -p Environment | tr ' ' '\n' | grep -E 'CUDA_|OLLAMA_NUM|OLLAMA_CONTEXT|OLLAMA_SCHED|OLLAMA_KEEP|OLLAMA_FLASH|OLLAMA_KV'
echo "== drop-ins"; systemctl cat ollama 2>/dev/null | grep -nE '^# /|CUDA_|CONTEXT|NUM_PARALLEL|SCHED_SPREAD'
echo "== ollama state"; systemctl is-active ollama; systemctl show ollama -p NRestarts -p ActiveEnterTimestamp
echo "== ollama ps"; ollama ps
echo "== ollama log: gpu discovery/offload (this start)"; journalctl -u ollama --since "$(systemctl show ollama -p ActiveEnterTimestamp --value)" --no-pager -o short-iso 2>/dev/null | grep -iE 'inference compute|offload|layers|library=cuda|total_vram|CUDA[0-9]? |device|error|fail|n_ctx|split' | head -n 40
echo "== kernel since boot: Xid/NVRM/AER"; journalctl -b 0 -k --no-pager | grep -ciE 'NVRM: Xid|GPU has fallen|GPU lost|uvm.*fatal'; journalctl -b 0 -k --no-pager | grep -c 'PCIe Bus Error'; journalctl -b 0 -k --no-pager -o short-iso | grep -E 'PCIe Bus Error|NVRM: Xid|AER: (Un|Multiple)' | awk '{print $1, $3, $4, $5, $6, $7, $8, $9, $10}' | tail -n 12
echo "== AER counters per gpu port"; for p in /sys/bus/pci/devices/0000:00:*; do [ -f $p/aer_dev_correctable ] || continue; c=$(awk '/TOTAL/{print $2}' $p/aer_dev_correctable 2>/dev/null); n=$(awk '/TOTAL/{print $2}' $p/aer_dev_nonfatal 2>/dev/null); f=$(awk '/TOTAL/{print $2}' $p/aer_dev_fatal 2>/dev/null); echo "$(basename $p) corr=$c nonfatal=$n fatal=$f"; done
