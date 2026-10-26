date '+%F %T'
ls -l /etc/systemd/system/nvidia-powercap.service /run/systemd/system/nvidia-powercap.service /root/gpu4-backup-20261026/ 2>&1
systemctl is-enabled nvidia-powercap 2>&1; systemctl show nvidia-powercap -p LoadState -p ActiveState -p FragmentPath -p UnitFileState
nvidia-smi --query-gpu=index,power.limit,enforced.power.limit,power.draw --format=csv,noheader
ls -lt /etc/systemd/system/*.service 2>/dev/null | head -n 5
journalctl -b 0 --no-pager -o short-iso --since 13:21 | grep -iE 'mask|powercap|sudo|Accepted|daemon-reload|Reload' | cut -c1-180 | tail -n 15