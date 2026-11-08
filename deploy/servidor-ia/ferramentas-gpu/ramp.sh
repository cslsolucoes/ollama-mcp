#!/bin/bash
# degraus de potencia com carga SGEMM nas 4 GPUs ao mesmo tempo; para no 1o sinal de problema
D=/root/pl-test; SECS=${SECS:-60}
for W in ${STEPS:-130 160 190 220 240}; do
  nvidia-smi -pl $W >/dev/null
  echo "== degrau ${W}W $(date +%T)" | tee -a $D/ramp.log
  ( while :; do echo "$(date +%T) $(nvidia-smi --query-gpu=power.draw,clocks.sm,temperature.gpu,clocks_throttle_reasons.active --format=csv,noheader,nounits | tr "\n" "|")"; sleep 1; done ) > $D/ramp_$W.mon.log & M=$!
  BURN_SECS=$SECS /root/pl-test/burn2 >> $D/ramp.log 2>&1; R=$?
  kill $M
  python3 - <<PY | tee -a $D/ramp.log
mx=[0]*4; mt=[0]*4; mc=[9999]*4; tot=0
for l in open("$D/ramp_$W.mon.log"):
    try: p=l.strip().split(" ",1)[1].split("|")[:4]
    except: continue
    s=0
    for i,x in enumerate(p):
        f=x.split(","); w=float(f[0]); mx[i]=max(mx[i],w); mt[i]=max(mt[i],int(f[2])); s+=w
        if len(f)>1 and w>50: mc[i]=min(mc[i],int(f[1]))
    tot=max(tot,s)
print("  pico W por GPU",mx,"| pico SOMA",round(tot),"W | temp max",mt,"| clock min sob carga",mc)
PY
  X=$(journalctl -k --since "-3min" --no-pager | grep -ciE "Xid|fell off")
  echo "  exit=$R xid=$X" | tee -a $D/ramp.log
  T=$(tr "|" "
" < $D/ramp_$W.mon.log | awk -F, "NF>=3{if(\$3>m)m=\$3}END{print m+0}"); [ "$R" != 0 -o "$X" != 0 -o "$T" -ge 90 ] && { echo "PARADO no degrau $W" | tee -a $D/ramp.log; break; }
  sleep 20
done
nvidia-smi -pl 240 >/dev/null; echo "fim $(date +%T); limite em 240W" | tee -a $D/ramp.log
