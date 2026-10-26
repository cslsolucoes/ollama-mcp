date '+%F %T'; uptime -s; last -x -n 3 reboot shutdown | head -n 3
echo "== prev boot (-1) source counts"; journalctl -b -1 -k --no-pager 2>/dev/null | grep -oE 'pcieport [0-9a-f:.]+|NVRM: Xid \(PCI:[0-9a-f:]+\): [0-9]+' | sort | uniq -c
echo "== prev boot first/last runtime AER"; journalctl -b -1 -k --no-pager -o short-iso 2>/dev/null | grep 'PCIe Bus Error' | sed -n '4p;$p' | cut -c1-160
echo "== prev boot last 6 lines"; journalctl -b -1 --no-pager -o short-iso -n 6 2>/dev/null | cut -c1-180
echo "== ollama now"; systemctl is-active ollama; ollama ps | tail -n +2