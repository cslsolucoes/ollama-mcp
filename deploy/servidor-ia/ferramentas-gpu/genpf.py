import json, random, time
random.seed(time.time_ns())
w = "alfa beta gama delta epsilon zeta eta teta iota kapa lambda mu ni xi omicron pi ro sigma tau upsilon fi qui psi omega".split()
txt = " ".join(random.choice(w) + str(random.randint(0, 999)) for _ in range(11000))
print(json.dumps({"model": "qwen3.8:27b", "prompt": txt + "\n\nResuma em 2 linhas.", "stream": False, "think": False, "options": {"num_predict": 40, "temperature": 0, "seed": 42}}))
