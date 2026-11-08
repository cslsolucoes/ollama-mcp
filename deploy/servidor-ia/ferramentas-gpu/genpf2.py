import json, random, sys
seed = int(sys.argv[1]); n = int(sys.argv[2]); npred = int(sys.argv[3])
random.seed(seed)
w = "alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega".split()
t = " ".join(random.choice(w) + str(random.randint(0, 999)) for _ in range(n))
print(json.dumps({"model": "qwen3.8:27b", "prompt": t + "\n\nResuma em 2 linhas.", "stream": False, "think": False, "options": {"num_predict": npred, "temperature": 0, "seed": 42}}))
