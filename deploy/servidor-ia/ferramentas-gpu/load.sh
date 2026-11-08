#!/bin/bash
# uso: load.sh <rotulo> ; carga: prefill de ~60k tokens + 300 tokens gerados, direto no Ollama (127.0.0.1:11434)
R=$1; D=/root/pl-test
python3 - <<PY > $D/$R.req.json
import json, random
random.seed(int("$(date +%s)"))
w = "alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega".split()
txt = " ".join(random.choice(w)+str(random.randint(0,999)) for _ in range(38000))
print(json.dumps({"model":"qwen3.8:27b","prompt":txt+"\n\nResuma em 3 linhas o que voce leu.","stream":False,"think":False,"options":{"num_predict":300,"temperature":0}}))
PY
( while :; do echo "$(date +%T) $(nvidia-smi --query-gpu=power.draw,clocks.sm,temperature.gpu --format=csv,noheader,nounits | tr "\n" "|")"; sleep 1; done ) > $D/$R.mon.log &
M=$!
S=$(date +%s)
curl -s -m 900 127.0.0.1:11434/api/generate -d @$D/$R.req.json | python3 -c "import sys,json; d=json.load(sys.stdin); print({k:d.get(k) for k in [\"prompt_eval_count\",\"eval_count\",\"prompt_eval_duration\",\"eval_duration\",\"total_duration\"]})" > $D/$R.res.txt 2>&1
echo "fim $(( $(date +%s)-S ))s" >> $D/$R.res.txt
kill $M
