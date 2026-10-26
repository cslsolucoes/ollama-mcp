date '+%F %T'; uptime -s; last -x -n 2 reboot | head -n 2; systemctl show ollama -p NRestarts
echo "Xid=$(journalctl -b 0 -k --no-pager | grep -ciE 'NVRM: Xid|GPU lost|uvm.*fatal') uncorr=$(journalctl -b 0 -k --no-pager | grep -ciE 'severity=(Uncorrected|Fatal)')"
for p in 00:1b.4 00:1c.2 00:1c.3; do f=/sys/bus/pci/devices/0000:$p; echo "$p corr=$(awk '/TOTAL/{print $2}' $f/aer_dev_correctable) nonfatal=$(awk '/TOTAL/{print $2}' $f/aer_dev_nonfatal) fatal=$(awk '/TOTAL/{print $2}' $f/aer_dev_fatal)"; done
journalctl -b 0 -k --no-pager | grep -oE 'type=(Physical|Data Link) Layer|Multiple Correctable' | sort | uniq -c
nvidia-smi --query-gpu=index,power.limit,temperature.gpu --format=csv,noheader; ollama ps | tail -n +2