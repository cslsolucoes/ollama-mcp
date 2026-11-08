#!/bin/bash
D=/root/pl-test/oc; L=$D/ctx2.log; : > $L
ptxt(){ python3 - "$1" <<'PY'
import json,random,sys,time
random.seed(time.time_ns()); n=int(sys.argv[1])
w="alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega".split()
print(" ".join(random.choice(w)+str(random.randint(0,999)) for _ in range(n)))
PY
}
for C in 114688 131072; do
  ollama stop qwen3.8:27b >/dev/null 2>&1; sleep 6
  echo "== num_ctx=$C num_gpu=66 $(date +%T)" | tee -a $L
  cat > $D/c2.json <<J
{"model":"qwen3.8:27b","prompt":"Liste em ordem os numeros de 1 a 60, um por linha, e depois explique em 5 linhas o que e uma fonte de alimentacao chaveada.","stream":false,"think":false,"options":{"num_predict":400,"temperature":0,"seed":42,"num_ctx":$C,"num_gpu":66}}
J
  R=$(curl -s -m 600 127.0.0.1:11434/api/generate -d @$D/c2.json)
  echo "$R" | python3 -c "import sys,json; d=json.load(sys.stdin); print('  ERRO:', d['error'][:200]) if 'error' in d else print('  1a chamada (carga): total', round(d['total_duration']/1e9,1), 's')" 2>&1 | tail -1 | tee -a $L
  if echo "$R" | grep -q "\"error\""; then continue; fi
  for k in 1 2; do curl -s -m 600 127.0.0.1:11434/api/generate -d @$D/c2.json | python3 -c "import sys,json; d=json.load(sys.stdin); print('  geracao:', round(d['eval_count']/(d['eval_duration']/1e9),1), 'tok/s')" 2>&1 | tail -1 | tee -a $L; done
  ollama ps | sed "s/\x1b\[[0-9;?]*[a-zA-Z]//g" | grep qwen | tee -a $L
  echo "  VRAM (MiB): $(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | tr "\n" " ")" | tee -a $L
  journalctl -u ollama --since "-2min" --no-pager | grep -oE "offloaded [0-9]+/[0-9]+ layers[^,]*" | tail -1 | tee -a $L
  if [ $C = 131072 ]; then
    echo "  -- contexto realmente cheio (~98 mil tokens de prompt) --" | tee -a $L
    python3 - <<PY > $D/big.json
import json,subprocess
t=subprocess.run(["python3","-c","import random,time;random.seed(time.time_ns());w='alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega'.split();print(' '.join(random.choice(w)+str(random.randint(0,999)) for _ in range(76000)))"],capture_output=True,text=True).stdout
print(json.dumps({"model":"qwen3.8:27b","prompt":t+"\n\nResuma em 2 linhas.","stream":False,"think":False,"options":{"num_predict":60,"temperature":0,"seed":42,"num_ctx":131072,"num_gpu":66}}))
PY
    curl -s -m 900 127.0.0.1:11434/api/generate -d @$D/big.json | python3 -c "import sys,json; d=json.load(sys.stdin); print('  ERRO:', d['error'][:200]) if 'error' in d else print('  prompt', d['prompt_eval_count'], 'tokens: prefill', round(d['prompt_eval_count']/(d['prompt_eval_duration']/1e9)), 'tok/s; geracao com contexto cheio', round(d['eval_count']/(d['eval_duration']/1e9),1), 'tok/s')" 2>&1 | tail -1 | tee -a $L
    echo "  VRAM cheio (MiB): $(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | tr "\n" " ")" | tee -a $L
  fi
  X=$(journalctl -k --since "-6min" --no-pager | grep -ciE "Xid|GSP-CrashCat"); echo "  xid=$X" | tee -a $L
done
ollama stop qwen3.8:27b >/dev/null 2>&1; sleep 5; systemctl restart ollama-warm
for i in $(seq 1 40); do ollama ps 2>/dev/null | grep -q qwen3.8 && break; sleep 3; done
echo "== restaurado: $(ollama ps | sed "s/\x1b\[[0-9;?]*[a-zA-Z]//g" | grep qwen)" | tee -a $L
echo "fim $(date +%T)" | tee -a $L
