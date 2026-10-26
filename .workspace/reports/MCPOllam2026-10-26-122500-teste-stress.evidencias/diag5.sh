for b in -4 -3 -2 -1; do
  echo "===== boot $b: last kernel lines matching GPU/PCIe"
  journalctl -b $b -k --no-pager -o short-iso 2>/dev/null | grep -iE 'xid|nvrm|uvm|aer|pcie bus error|fallen|lost|mce|machine check|watchdog|panic' | grep -vE 'AER: enabled with IRQ|_OSC' | cut -c1-220 | tail -n 14
  echo "-- count by source"; journalctl -b $b -k --no-pager 2>/dev/null | grep -oE 'pcieport [0-9a-f:.]+|NVRM: Xid \(PCI:[0-9a-f:]+\): [0-9]+|GPU at PCI:[0-9a-f:]+' | sort | uniq -c
  echo "-- very last 4 lines of boot"; journalctl -b $b --no-pager -o short-iso -n 4 2>/dev/null | cut -c1-200
done
echo "===== now"; ollama ps | tail -n +2; systemctl is-active ollama-warm