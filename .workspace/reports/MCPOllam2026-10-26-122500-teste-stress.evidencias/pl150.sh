set -e
B=/root/gpu4-backup-20261026; U=/etc/systemd/system/nvidia-powercap.service
test -s $U
cp $U $B/nvidia-powercap.service.170w.bak2; sync
sed -e 's|nvidia-smi -i "$i" -pl 170;|nvidia-smi -i "$i" -pl 150;|' -e 's|^# Power limit 170W nas 4 GPUs;|# Power limit 150W nas 4 GPUs (owner 2026-10-26 13:34; 200W cortou energia, 170W caiu em ~5 min de carga maxima);|' $U > $U.new
grep -q -- '-pl 150;' $U.new
mv -f $U.new $U; chmod 644 $U; sync
ls -l $U; grep -nE 'Power limit|-pl' $U | cut -c1-160
systemctl daemon-reload
systemctl restart nvidia-powercap
systemctl show nvidia-powercap -p LoadState -p UnitFileState -p ActiveState -p Result -p NeedDaemonReload
nvidia-smi --query-gpu=index,power.limit,enforced.power.limit --format=csv,noheader
sync