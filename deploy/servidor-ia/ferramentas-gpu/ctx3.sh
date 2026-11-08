#!/bin/bash
# simula o gateway (que tira num_ctx/num_gpu do cliente): o modelo carrega com 128k + num_gpu 66 vindos do PROPRIO modelo (Modelfile), pedidos sem essas opcoes
D=/root/pl-test/oc; L=$D/ctx3.log; : > $L; T=qwen3.8-t128
cleanup(){ ollama stop $T >/dev/null 2>&1; ollama rm $T >/dev/null 2>&1; ollama stop qwen3.8:27b >/dev/null 2>&1; sleep 4; systemctl restart ollama-warm
 for i in $(seq 1 40); do ollama ps 2>/dev/null | grep -q qwen3.8 && break; sleep 3; done
 echo "== restaurado: $(ollama ps | sed "s/\x1b\[[0-9;?]*[a-zA-Z]//g" | grep qwen)" | tee -a $L; echo "fim $(date +%T)" | tee -a $L; }
trap cleanup EXIT
ollama stop qwen3.8:27b >/dev/null 2>&1; sleep 6
printf "FROM qwen3.8:27b\nPARAMETER num_ctx 131072\nPARAMETER num_gpu 66\n" > $D/Modelfile.t128
ollama create $T -f $D/Modelfile.t128 2>&1 | tail -1 | sed "s/\x1b\[[0-9;?]*[a-zA-Z]//g" | tee -a $L
cat > $D/q.json <<J
{"model":"$T","prompt":"Liste em ordem os numeros de 1 a 60, um por linha, e depois explique em 5 linhas o que e uma fonte de alimentacao chaveada.","stream":false,"think":false,"options":{"num_predict":400,"temperature":0,"seed":42}}
J
for k in 1 2 3; do
  curl -s -m 600 127.0.0.1:11434/api/generate -d @$D/q.json | python3 -c "import sys,json; d=json.load(sys.stdin); print('  pedido $k (sem num_ctx/num_gpu): total', round(d['total_duration']/1e9,1), 's | carga', round(d['load_duration']/1e9,1), 's | geracao', round(d['eval_count']/(d['eval_duration']/1e9),1), 'tok/s') if 'error' not in d else print('  ERRO', d['error'][:200])" 2>&1 | tail -1 | tee -a $L
done
ollama ps | sed "s/\x1b\[[0-9;?]*[a-zA-Z]//g" | grep qwen | tee -a $L
echo "  cargas do modelo no log (deve ser 1): $(journalctl -u ollama --since "-3min" --no-pager | grep -c "load_model: initializing")" | tee -a $L
echo "  VRAM (MiB): $(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | tr "\n" " ")" | tee -a $L
echo "  -- contexto quase cheio (~125 mil tokens) --" | tee -a $L
python3 - <<PY > $D/big2.json
import json,random,time
random.seed(time.time_ns())
w="alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega".split()
t=" ".join(random.choice(w)+str(random.randint(0,999)) for _ in range(145000))
print(json.dumps({"model":"$T","prompt":t+"\n\nResuma em 2 linhas.","stream":False,"think":False,"options":{"num_predict":40,"temperature":0,"seed":42}}))
PY
curl -s -m 1200 127.0.0.1:11434/api/generate -d @$D/big2.json | python3 -c "import sys,json; d=json.load(sys.stdin); print('  ERRO', d['error'][:250]) if 'error' in d else print('  prompt', d['prompt_eval_count'], 'tokens | prefill', round(d['prompt_eval_count']/(d['prompt_eval_duration']/1e9)), 'tok/s | geracao', round(d['eval_count']/(d['eval_duration']/1e9),1), 'tok/s')" 2>&1 | tail -1 | tee -a $L
echo "  VRAM cheio (MiB): $(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | tr "\n" " ")" | tee -a $L
echo "  xid=$(journalctl -k --since "-15min" --no-pager | grep -ciE "Xid|GSP-CrashCat")" | tee -a $L
