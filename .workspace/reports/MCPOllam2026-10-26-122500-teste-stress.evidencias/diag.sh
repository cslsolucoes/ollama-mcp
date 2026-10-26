echo "== now"; date '+%F %T'
echo "== uptime"; uptime -s; uptime
echo "== boots"; journalctl --list-boots --no-pager | tail -n 4
echo "== last"; last -x -n 8 reboot shutdown 2>/dev/null | head -n 10
echo "== prev boot tail (-b -1)"; journalctl -b -1 --no-pager -n 40 -o short-iso 2>/dev/null
echo "== prev boot Xid/AER/NVRM/MCE count"; journalctl -b -1 -k --no-pager 2>/dev/null | grep -ciE 'xid|aer|nvrm|mce|machine check'
echo "== prev boot Xid/AER/NVRM last 25"; journalctl -b -1 -k --no-pager -o short-iso 2>/dev/null | grep -iE 'xid|aer|nvrm|mce|machine check|pcie bus error' | tail -n 25
echo "== this boot Xid/AER/NVRM count"; journalctl -b 0 -k --no-pager 2>/dev/null | grep -ciE 'xid|aer|nvrm|mce|machine check'
echo "== ollama"; systemctl is-active ollama nginx ollama-warm 2>/dev/null; systemctl show ollama -p NRestarts -p ActiveEnterTimestamp
echo "== ollama journal prev boot last 20"; journalctl -b -1 -u ollama --no-pager -n 20 -o short-iso 2>/dev/null
echo "== ollama journal this boot last 15"; journalctl -b 0 -u ollama --no-pager -n 15 -o short-iso 2>/dev/null
echo "== gpus"; nvidia-smi --query-gpu=index,name,pcie.link.gen.current,pcie.link.width.current,memory.used,temperature.gpu,power.draw --format=csv 2>&1
echo "== ollama ps"; ollama ps 2>&1
