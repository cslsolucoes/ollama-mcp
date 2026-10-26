echo "== now/uptime"; date '+%F %T'; uptime -s
echo "== last"; last -x -n 10 reboot shutdown | head -n 10
echo "== boots"; journalctl --list-boots --no-pager | tail -n 6
echo "== messages 12:28-12:40 kernel/nvidia/uvm/aer"; grep -hE '^Oct 26 12:(2[89]|3[0-9]|40)' /var/log/messages 2>/dev/null | grep -iE 'xid|nvrm|uvm|aer|pcie bus error|fallen|lost|reboot|panic|watchdog|mce|machine check|Linux version|Command line' | awk '{ $4=""; print }' | cut -c1-230 | head -n 70
echo "== messages per-port AER count 12:28-12:40"; grep -hE '^Oct 26 12:(2[89]|3[0-9]|40)' /var/log/messages | grep -oE 'pcieport [0-9a-f:.]+|NVRM: Xid \(PCI:[0-9a-f:]+\): [0-9]+' | sort | uniq -c
echo "== last lines of each boot segment"; grep -nE 'Linux version' /var/log/messages | tail -n 6
echo "== ollama now"; systemctl is-active ollama; ollama ps 2>&1 | tail -n +2
nvidia-smi --query-gpu=index,pci.bus_id,pcie.link.gen.current,pcie.link.width.current,memory.used --format=csv,noheader 2>&1