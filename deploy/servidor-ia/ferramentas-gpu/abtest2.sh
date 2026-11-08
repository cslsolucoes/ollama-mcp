#!/bin/bash
D=/root/pl-test/oc; R=$D/ab2.jsonl; L=$D/ab2.log; : > $R; : > $L
setcfg(){ case $1 in
 A) nvidia-smi -pl 100 >/dev/null; for g in 0 1 2 3; do nvidia-smi -i $g -lgc 210,1400 >/dev/null; $D/oc set $g mem 0 >/dev/null; $D/oc set $g graphics 0 >/dev/null; done;;
 B) nvidia-smi -pl 240 >/dev/null; for g in 0 1 2 3; do nvidia-smi -i $g -rgc >/dev/null; $D/oc set $g mem 0 >/dev/null; $D/oc set $g graphics 0 >/dev/null; done;;
 C) nvidia-smi -pl 240 >/dev/null; for g in 0 1 2 3; do nvidia-smi -i $g -rgc >/dev/null; $D/oc set $g mem 1500 >/dev/null; $D/oc set $g graphics 150 >/dev/null; done;;
 esac; sleep 5; }
restore(){ setcfg C; echo "restaurado config final (C) $(date +%T)" | tee -a $L; echo fim | tee -a $L; }
trap restore EXIT
run(){ # $1=cfg $2=rodada
  cfg=$1; rd=$2
  ( while :; do nvidia-smi --query-gpu=clocks.sm,temperature.gpu,power.draw --format=csv,noheader,nounits | tr "\n" "|"; echo; sleep 2; done > $D/ab_mon.tmp ) & M=$!
  for s in 1 2 3; do
    SEED=$((1000 + RANDOM*10 + s)); python3 $D/genpf2.py $SEED 8000 120 > $D/ab_req.json
    curl -s -m 900 127.0.0.1:11434/api/generate -d @$D/ab_req.json | python3 -c "
import sys,json,hashlib
d=json.load(sys.stdin)
print(json.dumps({'cfg':'$cfg','rodada':$rd,'teste':'prompt_longo_$s','tokens':d['prompt_eval_count'],'prefill_tps':round(d['prompt_eval_count']/(d['prompt_eval_duration']/1e9),1),'decode_tps':round(d['eval_count']/(d['eval_duration']/1e9),1),'md5':hashlib.md5(d['response'].encode()).hexdigest()[:8]}))" >> $R 2>>$L
  done
  kill $M; wait $M 2>/dev/null
  python3 - <<PY >> $R
import json
mc=0;mt=0;mp=0
for l in open("$D/ab_mon.tmp"):
    for g in l.strip().split("|"):
        f=g.split(",")
        if len(f)>=3:
            try: mc=max(mc,int(f[0])); mt=max(mt,int(f[1])); mp=max(mp,float(f[2]))
            except: pass
print(json.dumps({'cfg':'$cfg','rodada':$rd,'teste':'pico','clock_mhz':mc,'temp_c':mt,'watts':mp}))
PY
  echo "feito $cfg rodada $rd $(date +%T)" | tee -a $L
}
for step in "A 1" "B 1" "C 1" "C 2" "B 2" "A 2"; do set -- $step; setcfg $1; run $1 $2; done
