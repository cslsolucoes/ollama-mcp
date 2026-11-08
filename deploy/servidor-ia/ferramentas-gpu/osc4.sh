#!/bin/bash
# degraus de offset de MEMORIA nas 4 GPUs ao mesmo tempo (memtest simultaneo); para no 1o erro, Xid ou reinicio
D=/root/pl-test/oc; L=$D/osc.log; STEPS=${STEPS:-"2000"}; PREV=0
reset(){ for g in 0 1 2 3; do $D/oc set $g mem 0 >/dev/null 2>&1; done; }
trap reset EXIT
for O in $STEPS; do
  for g in 0 1 2 3; do $D/oc set $g mem $O >/dev/null; done
  echo "== offset memoria +$O MHz nas 4 GPUs $(date +%T)" | tee -a $L
  ( for i in $(seq 1 12); do sleep 2; nvidia-smi --query-gpu=clocks.mem,power.draw,temperature.gpu --format=csv,noheader | tr "\n" "|" >> $D/clk.tmp; echo >> $D/clk.tmp; done ) & C=$!
  for g in 0 1 2 3; do $D/memtest $g ${SECS:-40} ${MB:-3500} > $D/mt_$g.out 2>&1 & eval P$g=$!; done
  BAD=0; for g in 0 1 2 3; do eval wait \$P$g; R=$?; cat $D/mt_$g.out | tee -a $L; [ $R != 0 ] && BAD=1; done
  wait $C; cp $D/clk.tmp $D/clk_last.tmp; echo "  clk mem/W/C (pico): $(sort -r $D/clk.tmp | head -1)" | tee -a $L; rm -f $D/clk.tmp
  X=$(journalctl -k --since "-2min" --no-pager | grep -ciE "Xid|fell off")
  echo "  offset=$O falha=$BAD xid=$X" | tee -a $L
  BW=$(grep -ho "~[0-9]* GB/s" $D/mt_*.out | tr -d "~ GB/s" | sort -n | head -1)
  TM=$(tr "|" "
" < $D/clk_last.tmp 2>/dev/null | awk -F, "NF>=3{if(\$3+0>m)m=\$3+0}END{print m+0}")
  echo "  banda minima entre as GPUs: ${BW} GB/s (anterior ${PREV}); temp pico: ${TM}" | tee -a $L
  [ "$PREV" != 0 ] && [ "$BW" -lt $(( PREV - 2 )) ] && { BAD=1; echo "  banda CAIU (correcao de erro da memoria)" | tee -a $L; }
  [ "$TM" -ge 94 ] && { BAD=1; echo "  temperatura >= 94" | tee -a $L; }
  PREV=$BW
  if [ $BAD != 0 -o "$X" != 0 ]; then echo "PARADO no offset $O" | tee -a $L; break; fi
  sleep 5
done
reset; echo "fim $(date +%T); offsets zerados" | tee -a $L
