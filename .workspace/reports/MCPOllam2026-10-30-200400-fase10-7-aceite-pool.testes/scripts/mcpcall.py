# 10.7 (f): chama o mcpollama-mcp por stdio (initialize + tools/call generate) e imprime o resultado.
# Uso: python mcpcall.py <tag> <args do mcpollama...> -- <json dos argumentos da tool>
import json, subprocess, sys, threading, time

EXE = r"D:\Users\claiton.linhares\AppData\Local\Temp\claude\e--MCPOllam\19be7a02-91dd-4049-bbce-a0473f6228d1\scratchpad\a107\bin\mcpollama-mcp.exe"
i = sys.argv.index("--")
tag, srv_args, tool_args = sys.argv[1], sys.argv[2:i], json.loads(sys.argv[i + 1])
p = subprocess.Popen([EXE] + srv_args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")
err = []
threading.Thread(target=lambda: [err.append(l.rstrip()) for l in p.stderr], daemon=True).start()


def send(o):
    p.stdin.write(json.dumps(o) + "\n")
    p.stdin.flush()


def recv(want):
    while True:
        l = p.stdout.readline()
        if not l:
            return None
        m = json.loads(l)
        if m.get("id") == want:
            return m


t0 = time.time()
send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "aceite107", "version": "0"}}})
init = recv(1)
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
send({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "generate", "arguments": tool_args}})
r = recv(2)
dt = time.time() - t0
p.stdin.close()
try:
    p.wait(10)
except Exception:
    p.kill()
time.sleep(0.3)
print(f"=== {tag} ({dt:.1f}s) init={'ok' if init and 'result' in init else init}")
if r is None:
    print("sem resposta")
elif "error" in r:
    print("JSON-RPC error:", json.dumps(r["error"], ensure_ascii=False))
else:
    res = r["result"]
    print("isError:", res.get("isError"), "_meta:", json.dumps(res.get("_meta"), ensure_ascii=False))
    for c in res.get("content", []):
        print("content:", c.get("text", "")[-400:])
print("--- stderr do mcpollama:")
for l in err:
    print("  ", l[:300])
