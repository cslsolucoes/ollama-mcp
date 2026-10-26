echo "== nvidia-smi map"; nvidia-smi --query-gpu=index,pci.bus_id,uuid,serial,pcie.link.gen.max,pcie.link.width.max,pcie.link.width.current --format=csv,noheader
echo "== ollama CUDA env"; systemctl show ollama -p Environment | tr ' ' '\n' | grep -E 'CUDA_|GPU|OLLAMA_NUM|OLLAMA_CONTEXT' 
grep -rhE 'CUDA_VISIBLE|CUDA_DEVICE_ORDER' /etc/systemd/system/ollama.service.d/ /etc/systemd/system/ollama.service 2>/dev/null
echo "== lspci nvidia+eth"; lspci -D | grep -iE 'vga|3d|ethernet'
echo "== root port of each gpu"; for b in $(nvidia-smi --query-gpu=pci.bus_id --format=csv,noheader); do d=$(echo $b | sed 's/^0000//; s/^\(....\):/\1:/' | tr 'A-F' 'a-f' | sed 's/^0*//'); p=$(readlink -f /sys/bus/pci/devices/$(echo $b | tr 'A-F' 'a-f' | sed 's/^00000000/0000/') 2>/dev/null); echo "$b -> $p"; done
echo "== journald storage"; ls -d /run/log/journal/* /var/log/journal/* 2>&1; journalctl --header 2>/dev/null | grep -E '^File path' | head -n 4
echo "== prev boot logs that sit in /var/log/messages: AER root ports since Oct 23"; grep -h 'PCIe Bus Error' /var/log/messages* 2>/dev/null | grep -oE 'pcieport [0-9a-f:.]+' | sort | uniq -c