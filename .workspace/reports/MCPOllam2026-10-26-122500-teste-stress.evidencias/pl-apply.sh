set -e
U=/etc/systemd/system/nvidia-powercap.service
cp -a $U /root/gpu4-backup-20261026/nvidia-powercap.service.170w
sed -i 's|nvidia-smi -i "$i" -pl 170;|nvidia-smi -i "$i" -pl 200;|' $U
sed -i 's|^# Power limit 170W nas 4 GPUs;|# Power limit 200W nas 4 GPUs (owner 2026-10-26 13:19; era 170W);|' $U
grep -nE 'pl [0-9]+|Power limit' $U
systemctl daemon-reload
systemctl restart nvidia-powercap
systemctl show nvidia-powercap -p ActiveState -p Result
nvidia-smi --query-gpu=index,pci.bus_id,power.limit,enforced.power.limit,power.draw --format=csv,noheader
systemctl show ollama -p NeedDaemonReload; systemctl show nvidia-powercap -p NeedDaemonReload
ollama ps | tail -n +2