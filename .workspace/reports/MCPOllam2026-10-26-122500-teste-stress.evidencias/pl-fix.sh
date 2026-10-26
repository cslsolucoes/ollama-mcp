set -e
B=/root/gpu4-backup-20261026; U=/etc/systemd/system/nvidia-powercap.service
sed -e 's|nvidia-smi -i "$i" -pl 200;|nvidia-smi -i "$i" -pl 170;|' -e 's|^# Power limit 200W nas 4 GPUs (owner 2026-10-26 13:19; era 170W);|# Power limit 170W nas 4 GPUs;|' $B/nvidia-powercap.service.200w-falhou > $U.new
mv -f $U.new $U; chmod 644 $U; cp $U $B/nvidia-powercap.service.170w; sync
ls -l $U $B/nvidia-powercap.service.170w; grep -nE 'Power limit|-pl' $U | cut -c1-150
systemctl daemon-reload
systemctl show nvidia-powercap -p LoadState -p UnitFileState
systemctl restart nvidia-powercap
systemctl show nvidia-powercap -p ActiveState -p Result
nvidia-smi --query-gpu=index,power.limit,enforced.power.limit --format=csv,noheader
echo "== zero-length files changed in last 4h"; find /etc /usr/local/bin /root -xdev -type f -size 0 -mmin -240 2>/dev/null
echo "== sizes of today-edited files"; ls -l /etc/systemd/system/ollama.service.d/10-performance.conf /usr/local/bin/ollama-warm /etc/systemd/system/ollama-warm.service
grep -E 'CONTEXT|CUDA' /etc/systemd/system/ollama.service.d/10-performance.conf
sync