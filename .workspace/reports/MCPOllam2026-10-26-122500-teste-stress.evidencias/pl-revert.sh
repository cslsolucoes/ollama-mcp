set -e
U=/etc/systemd/system/nvidia-powercap.service
cp -a $U /root/gpu4-backup-20261026/nvidia-powercap.service.200w-falhou
cp -a /root/gpu4-backup-20261026/nvidia-powercap.service.170w $U
grep -n -- '-pl' $U | cut -c1-140
systemctl daemon-reload
systemctl restart nvidia-powercap
nvidia-smi --query-gpu=index,power.limit,enforced.power.limit --format=csv,noheader
systemctl show nvidia-powercap -p NeedDaemonReload -p Result
grep -E 'CONTEXT|CUDA' /etc/systemd/system/ollama.service.d/10-performance.conf
ollama ps | tail -n +2