date '+%F %T'; uptime -s; last -x -n 3 reboot shutdown | head -n 3
echo "== prev boot last lines"; journalctl -b -1 --no-pager -o short-iso -n 5 2>/dev/null | cut -c1-180
echo "== prev boot kernel since 13:25"; journalctl -b -1 -k --no-pager -o short-iso --since 13:25 2>/dev/null | grep -iE 'xid|nvrm|uvm|aer|pcie bus error|thermal|mce|machine check|power' | cut -c1-180 | tail -n 12
echo "== counts"; journalctl -b -1 -k --no-pager --since 13:25 2>/dev/null | grep -oE 'pcieport [0-9a-f:.]+|NVRM: Xid \(PCI:[0-9a-f:]+\): [0-9]+' | sort | uniq -c
echo "== sensors/ipmi"; command -v sensors >/dev/null && sensors 2>/dev/null | grep -iE 'package|core 0|temp1' | head -n 4
nvidia-smi --query-gpu=index,power.limit --format=csv,noheader; ls -l /etc/systemd/system/nvidia-powercap.service