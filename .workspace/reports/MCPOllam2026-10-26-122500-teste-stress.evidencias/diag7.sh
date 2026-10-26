date '+%F %T'; uptime -s; last -x -n 3 reboot shutdown | head -n 3
echo "== prev boot (-1) last lines"; journalctl -b -1 --no-pager -o short-iso -n 8 2>/dev/null | cut -c1-200
echo "== prev boot kernel GPU/PCIe after 13:19"; journalctl -b -1 -k --no-pager -o short-iso --since 13:19 2>/dev/null | grep -iE 'xid|nvrm|uvm|aer|pcie bus error|fallen|lost|mce|machine check|power|thermal' | cut -c1-200 | tail -n 15
echo "== counts"; journalctl -b -1 -k --no-pager --since 13:19 2>/dev/null | grep -oE 'pcieport [0-9a-f:.]+|NVRM: Xid \(PCI:[0-9a-f:]+\): [0-9]+' | sort | uniq -c
echo "== now"; nvidia-smi --query-gpu=index,power.limit --format=csv,noheader; systemctl is-active ollama; ollama ps | tail -n +2