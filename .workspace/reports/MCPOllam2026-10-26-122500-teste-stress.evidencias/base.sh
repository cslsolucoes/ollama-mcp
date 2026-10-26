echo "=== data/uptime"; date '+%F %T'; uptime -s; uptime -p
echo "=== ultimos boots"; last -x reboot 2>/dev/null | head -5
echo "=== journal persistente"; ls -d /var/log/journal 2>/dev/null && journalctl --list-boots 2>/dev/null | tail -3
echo "=== GPUs"; nvidia-smi --query-gpu=index,pci.bus_id,pcie.link.gen.current,pcie.link.width.current,power.draw,power.limit,temperature.gpu,memory.used,memory.total --format=csv,noheader
echo "=== Xid/AER/NVRM desde o boot"; journalctl -k -b 2>/dev/null | grep -ciE 'xid|aer|gpu lost|nvrm.*error'
echo "=== ollama"; systemctl is-active ollama; systemctl show ollama -p NRestarts -p ActiveEnterTimestamp