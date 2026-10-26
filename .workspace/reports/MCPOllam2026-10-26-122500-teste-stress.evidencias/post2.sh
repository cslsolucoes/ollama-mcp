date '+%F %T'; uptime -s; systemctl show ollama -p NRestarts
echo "Xid=$(journalctl -b 0 -k --no-pager | grep -ciE 'NVRM: Xid|GPU lost|uvm.*fatal')"
for p in 00:1b.4 00:1c.2 00:1c.3; do echo "$p corr=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_correctable) nonfatal=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_nonfatal) fatal=$(awk '/TOTAL/{print $2}' /sys/bus/pci/devices/0000:$p/aer_dev_fatal)"; done
ollama ps | tail -n +2