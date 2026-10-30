"""Minimal stdio MCP client: validates mcpollama 0.5.2 with the GestorERP .mcp.json args.
Never prints the token. Usage: python -I drv.py <path-to-.mcp.json>"""
import json, re, subprocess, sys, time

cfg = json.load(open(sys.argv[1], encoding="utf-8"))["mcpServers"]["mcpOllama"]
tok = cfg["args"][cfg["args"].index("--api-key") + 1]
p = subprocess.Popen([cfg["command"]] + cfg["args"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                     stderr=subprocess.DEVNULL, text=True, encoding="utf-8", bufsize=1)
nid = 0

def rpc(method, params=None, notify=False):
    global nid
    msg = {"jsonrpc": "2.0", "method": method}
    if params is not None:
        msg["params"] = params
    if not notify:
        nid += 1
        msg["id"] = nid
    p.stdin.write(json.dumps(msg) + "\n"); p.stdin.flush()
    if notify:
        return None
    while True:
        line = p.stdout.readline()
        if not line:
            raise SystemExit("server closed")
        r = json.loads(line)
        if r.get("id") == nid:
            return r

def call(name, args):
    time.sleep(1.0)
    r = rpc("tools/call", {"name": name, "arguments": args})
    if "error" in r:
        return True, "RPCERR " + json.dumps(r["error"], ensure_ascii=False)
    res = r["result"]
    txt = "".join(c.get("text", "") for c in res.get("content", []))
    return bool(res.get("isError")), txt

def uso(t):
    m = re.search(r"\[uso\][^\n]*", t)
    return m.group(0) if m else "-"

def body(t):
    return re.split(r"\n\s*\[(?:uso|arquivo)\]", t)[0].strip()

def scrub(s):
    return s.replace(tok, "<token>")

out = []
def log(s):
    out.append(scrub(s)); print(scrub(s), flush=True)

init = rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                          "clientInfo": {"name": "gestorerp-validacao", "version": "1"}})
rpc("notifications/initialized", notify=True)
log("serverInfo=" + json.dumps(init["result"].get("serverInfo")))
tl = rpc("tools/list", {})
tools = tl["result"]["tools"]
raw = json.dumps(tl["result"], ensure_ascii=False, separators=(",", ":"))
log("tools=%d nomes=%s bytes_tools_list=%d" % (len(tools), ",".join(sorted(t["name"] for t in tools)), len(raw.encode("utf-8"))))
gen = next(t for t in tools if t["name"] == "generate")["description"]
emb = next(t for t in tools if t["name"] == "embed")["description"]
log("R2 frase no generate: %s" % ("Counts and arithmetic are unreliable" in gen))
log("P1 aviso no embed: %s" % ("evicts the generation model" in emb))

GE = r"E:\Dropbox\Claiton Linhares\GestorERP"
MAN = GE + r"\.cursor\rules\pack-rules-manifest_V1.11.0.md"

# P3/R2: arithmetic, direct vs step-by-step
contas = [("17*23 + 144/12", 403), ("48*12 - 96/8", 564), ("36*27 - 512/16", 940), ("19*19 + 210/14", 376)]
ok_d = ok_s = 0
for expr, exp in contas:
    e, t = call("generate", {"prompt": "Responda apenas com o número, sem texto: quanto é %s?" % expr, "temperature": 0, "num_predict": 50})
    got = body(t); ok = got.strip() == str(exp); ok_d += ok
    log("CONTA direta %s esperado=%d obtido=%s %s" % (expr, exp, got, "OK" if ok else "ERRO"))
    e, t = call("generate", {"prompt": "Calcule passo a passo e termine com uma linha 'RESULTADO: <n>': quanto é %s?" % expr, "temperature": 0, "num_predict": 400})
    m = re.findall(r"RESULTADO:\s*\**\s*(-?\d+)", t); got = m[-1] if m else "?"; ok = got == str(exp); ok_s += ok
    log("CONTA passo  %s esperado=%d obtido=%s %s | %s" % (expr, exp, got, "OK" if ok else "ERRO", uso(t)))
log("RESUMO contas: direta %d/%d, passo a passo %d/%d" % (ok_d, len(contas), ok_s, len(contas)))

# R1: count by listing vs direct, on the GestorERP manifest (19) and TOOLS.md (7)
casos = [(MAN, "as linhas da tabela 'Rules activas' (o nome do ficheiro de cada linha, primeira coluna)", 19),
         (r"E:\MCPOllam\TOOLS.md", "os nomes das ferramentas (cabeçalhos '## `nome`')", 7)]
for path, what, exp in casos:
    okl = okc = 0
    for i in range(3):
        e, t = call("extract", {"path": path, "instruction": "Liste %s." % what, "format": "JSON array de strings, sem texto fora do JSON"})
        b = body(t)
        try:
            n = len(json.loads(b[b.index("["):b.rindex("]") + 1]))
        except Exception:
            n = "parse_erro"
        okl += (n == exp)
        e, t = call("extract", {"path": path, "instruction": "Quantos são %s? Responda apenas com o número." % what, "format": "apenas o número"})
        c = body(t).strip(); okc += (c == str(exp))
        log("CONTAGEM %s esperado=%d listar=%s contar=%s" % (path.split("\\")[-1], exp, n, c))
    log("RESUMO contagem %s: listar %d/3, contar direto %d/3" % (path.split("\\")[-1], okl, okc))

# P1: embed -> generate without reload
e, t = call("embed", {"input": ["nota fiscal eletrônica", "NF-e emitida pela SEFAZ", "receita de bolo de chocolate"]})
log("EMBED %s" % uso(t))
e, t = call("generate", {"prompt": "Responda só com a capital do Brasil, uma palavra.", "temperature": 0, "num_predict": 10})
log("GENERATE apos embed: %s | %s" % (body(t), uso(t)))

# P2: README summary without 'concepção'/'fase inicial' (3x, with and without focus)
bad = 0
for i in range(3):
    for focus in (None, "para que serve, como se configura no cliente e quais ferramentas existem"):
        a = {"path": r"E:\MCPOllam\README.md", "max_words": 120}
        if focus:
            a["focus"] = focus
        e, t = call("summarize_file", a)
        hit = bool(re.search(r"concep|fase inicial|est[aá]gio inicial", t, re.I)); bad += hit
log("RESUMO README: frases 'concepção/fase inicial' em %d/6" % bad)

# P5: GestorERP root readable; sensitive and outside-root blocked
e, t = call("extract", {"path": MAN, "instruction": "Qual é o FolderVersion e a Data do cabeçalho?", "format": "JSON {folderVersion, data}"})
log("P5 leitura GestorERP: erro=%s %s" % (e, body(t)[:120]))
e, t = call("summarize_file", {"path": GE + r"\.mcp.json", "max_words": 20})
log("P5 .mcp.json: erro=%s %s" % (e, t[:110]))
e, t = call("summarize_file", {"path": r"D:\Users\claiton.linhares\.claude\CLAUDE.md", "max_words": 20})
log("P5 fora da raiz: erro=%s %s" % (e, t[:90]))

p.stdin.close(); p.wait(timeout=10)
open(sys.argv[2] if len(sys.argv) > 2 else "drv-out.txt", "w", encoding="utf-8").write("\n".join(out) + "\n")
