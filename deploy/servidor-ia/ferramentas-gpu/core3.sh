#!/bin/bash
D=/root/pl-test/oc; L=$D/core.log; STEPS=${STEPS:-"0 50 100 150 200 250 300"}; : > $L
reset(){ for g in 0 1 2 3; do $D/oc set $g graphics 0 >/dev/null 2>&1; done; }
trap reset EXIT
REF=""
for O in $STEPS; do
  for g in 0 1 2 3; do $D/oc set $g graphics $O >/dev/null; done
  echo "== offset NUCLEO +$O MHz nas 4 GPUs $(date +%T)" | tee -a $L
  ( for i in $(seq 1 10); do sleep 2; nvidia-smi --query-gpu=clocks.sm,power.draw,temperature.gpu --format=csv,noheader | tr "\n" "|"; echo; done > $D/ck.tmp ) & C=$!
  for g in 0 1 2 3; do $D/cb $g 22 > $D/cb_$g.out 2>&1 & eval P$g=$!; done
  BAD=0; for g in 0 1 2 3; do eval wait \$P$g; R=$?; cat $D/cb_$g.out | tee -a $L; [ $R != 0 ] && BAD=1; done
  wait $C; echo "  clock sm/W/C (pico clock): $(sort -r $D/ck.tmp | head -1)" | tee -a $L
  TM=$(tr "|" "\n" < $D/ck.tmp | awk -F, "NF>=3{if(\$3+0>m)m=\$3+0}END{print m+0}")
  python3 $D/genpf.py > $D/pf_step.json
  R=$(curl -s -m 300 127.0.0.1:11434/api/generate -d @$D/pf_step.json)
  H=$(echo "$R" | python3 -c "import sys,json,hashlib; d=json.load(sys.stdin); print(hashlib.md5(d[\"response\"].encode()).hexdigest()[:10], round(d[\"prompt_eval_count\"]/(d[\"prompt_eval_duration\"]/1e9)), \"tok/s prefill\")" 2>&1 | tail -1)
  X=$(journalctl -k --since "-2min" --no-pager | grep -ciE "Xid|GSP-CrashCat|heartbeat timed")
  REF=${H%% *}
  echo "  prefill(md5, tok/s): $H | temp pico $TM | xid=$X | igual_ref=$([ "${H%% *}" = "$REF" ] && echo sim || echo NAO)" | tee -a $L
  if [ $BAD != 0 -o "$X" != 0 -o "${H%% *}" = "KeyError:" -o "$TM" -ge 93 ]; then echo "PARADO no offset $O" | tee -a $L; break; fi
  sleep 5
done
reset; echo "fim $(date +%T); offset do nucleo zerado" | tee -a $L
