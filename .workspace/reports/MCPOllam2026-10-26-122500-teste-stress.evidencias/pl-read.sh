date '+%F %T'; uptime -s
nvidia-smi --query-gpu=index,pci.bus_id,power.limit,power.default_limit,power.min_limit,power.max_limit,enforced.power.limit,power.draw --format=csv
echo "== unit"; systemctl cat nvidia-powercap 2>/dev/null
echo "== scripts referenced"; for f in $(systemctl cat nvidia-powercap 2>/dev/null | grep -oE '/[a-zA-Z0-9_./-]+\.sh|/usr/local/bin/[a-zA-Z0-9_.-]+' | sort -u); do echo "--- $f"; ls -l $f; cat $f; done
echo "== journal powercap this boot"; journalctl -b 0 -u nvidia-powercap --no-pager | tail -n 8
grep -rn -- '-pl\|power-limit' /etc/systemd/system/*.service /usr/local/bin 2>/dev/null | head