date '+%F %T'; uptime -s; systemctl show ollama -p NRestarts
echo "Xid=$(journalctl -b 0 -k --no-pager | grep -ciE 'NVRM: Xid|GPU lost|uvm.*fatal') nonfatal_fatal_msgs=$(journalctl -b 0 -k --no-pager | grep -ciE 'severity=(Uncorrected|Fatal)')"
for p in 00:01.0 00:1b.4 00:1c.2 00:1c.3; do f=/sys/bus/pci/devices/0000:$p; echo "$p corr=$(awk '/TOTAL/{print $2}' $f/aer_dev_correctable 2>/dev/null) nonfatal=$(awk '/TOTAL/{print $2}' $f/aer_dev_nonfatal 2>/dev/null) fatal=$(awk '/TOTAL/{print $2}' $f/aer_dev_fatal 2>/dev/null)"; done
journalctl -b 0 -k --no-pager -o short-iso --since 13:03 | grep 'PCIe Bus Error' | awk '{print $1, $3}' | uniq -c | tail -n 8
nvidia-smi --query-gpu=index,temperature.gpu,memory.used --format=csv,noheader; ollama ps | tail -n +2