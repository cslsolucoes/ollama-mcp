#!/usr/bin/env python3
# 10.7 - driver dos cenarios do aceite do pool (gateway de teste 127.0.0.1:11599).
# Uso: drv.py A|B   (A = secundario tipo cpu; B = secundario tipo remoto)
# Limites do owner: prompts pequenos (< 2k tokens) e no maximo 3 pedidos simultaneos.
import json, os, sqlite3, statistics, subprocess, sys, threading, time, urllib.error, urllib.request

GW = "http://127.0.0.1:11599"
W = "/var/tmp/aceite107"
BOOT0 = "8886b162-c7be-4322-8782-6235db648193"
ROUND = sys.argv[1]
ONLY = sys.argv[2] if len(sys.argv) > 2 else "abedc"
DB = f"{W}/gw{ROUND}/gw.db"
PHASE = ["init"]
PS = []  # (phase, t, ps434, tags434, ver434, ps435)
STOP = threading.Event()
VERD = []


def log(*a):
    print(time.strftime("%H:%M:%S"), *a, flush=True)


def verdict(cen, ok, msg):
    VERD.append((cen, "PASS" if ok else "FALHA", msg))
    log(f"VEREDITO|{cen}|{'PASS' if ok else 'FALHA'}|{msg}")


def boot_ok():
    b = open("/proc/sys/kernel/random/boot_id").read().strip()
    if b != BOOT0:
        log("BOOT MUDOU:", b, "- PARANDO DE VEZ")
        STOP.set()
        sys.exit(99)


def http(method, url, body=None, headers=None, timeout=600):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    if data is not None and "Content-Type" not in (headers or {}):
        req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.headers, r.read(), time.time() - t0
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read(), time.time() - t0
    except Exception as e:  # conexao
        return 0, {}, str(e).encode(), time.time() - t0


def gen(tag, prio=None, n=160, out=None):
    h = {"X-Mcpollama-Client": "aceite107"}
    if prio:
        h["X-Mcpollama-Priority"] = prio
    body = {"model": "qwen3-ia", "prompt": f"[{tag}] Escreva um texto corrido e detalhado sobre o mar, com pelo menos 300 palavras.", "stream": False,
            "options": {"num_predict": n, "temperature": 0}}
    st, hd, b, dt = http("POST", GW + "/api/generate", body, h)
    try:
        j = json.loads(b)
    except Exception:
        j = {"error": b[:160].decode(errors="replace")}
    r = dict(tag=tag, prio=prio or "normal", status=st, rid=hd.get("X-Request-Id") if hd else None, model=j.get("model"),
             eval=j.get("eval_count"), load_ms=int((j.get("load_duration") or 0) / 1e6), dur_s=round(dt, 1),
             err=j.get("error"), resp=(j.get("response") or "")[:60])
    log("GEN", json.dumps(r, ensure_ascii=False))
    if out is not None:
        out.append(r)
    return r


def burst(tags, prio=None, n=160, gap=0.4):
    out, th = [], []
    assert len(tags) <= 3, "no maximo 3 simultaneos"
    for t in tags:
        x = threading.Thread(target=gen, args=(t, prio, n, out))
        x.start()
        th.append(x)
        time.sleep(gap)
    for x in th:
        x.join()
    return out


def state():
    st, _, b, _ = http("GET", GW + "/monitor/api/state", timeout=10)
    return json.loads(b) if st == 200 else {}


def board(s=None):
    s = s or state()
    rows = []
    for x in s.get("backends", []):
        rows.append(f"{x['id']}:{x['type']}:{x['state']}:br={x['breaker']}:used={x['used']}/{x['slots']}:fila={x['queue']}"
                    f":dig={x['digest']}:proc={x['processor'] or '-'}:rules={x.get('rules', '')}:err={x['last_error'][:70]}")
    return " | ".join(rows)


def bk(s, i):
    for x in s.get("backends", []):
        if x["id"] == i:
            return x
    return {}


def q(sql, args=()):
    con = sqlite3.connect(f"file:{DB}?mode=ro", uri=True, timeout=10)
    try:
        return con.execute(sql, args).fetchall()
    finally:
        con.close()


def rows_for(res):
    time.sleep(1.5)
    ids = [r["rid"] for r in res if r["rid"]]
    if not ids:
        return []
    out = q(f"SELECT id,priority_final,backend,COALESCE(failover_rule,''),status,http_status,eval_count,run_ms,wait_ms "
            f"FROM requests WHERE id IN ({','.join('?' * len(ids))}) ORDER BY started_at", ids)
    for o in out:
        tag = [r["tag"] for r in res if r["rid"] == o[0]][0]
        log(f"  SQLITE {tag} id={o[0][:8]} prio={o[1]} backend={o[2]} failover_rule='{o[3]}' status={o[4]} http={o[5]} eval={o[6]} run_ms={o[7]} wait_ms={o[8]}")
    return out


def events_since(t0ms, kinds=None):
    ev = q("SELECT ts,kind,detail FROM events WHERE ts>=? ORDER BY ts", (t0ms,))
    if kinds:
        ev = [e for e in ev if e[1] in kinds]
    return ev


def show_events(t0ms, kinds=None):
    ev = events_since(t0ms, kinds)
    for e in ev:
        log(f"  EVENTO {time.strftime('%H:%M:%S', time.localtime(e[0] / 1000))} {e[1]} {e[2][:140]}")
    return ev


def wait_for(pred, timeout, every=1.0, what=""):
    t0 = time.time()
    while time.time() - t0 < timeout:
        boot_ok()
        try:
            v = pred()
            if v:
                return v
        except Exception as e:
            log("  wait_for erro:", e)
        time.sleep(every)
    log(f"  TIMEOUT esperando {what} ({timeout}s)")
    return None


def now_ms():
    return int(time.time() * 1000)


def watcher():  # (g) custo da checagem: tempos de /api/ps etc. a 1 Hz, e o quadro quando muda
    last = ""
    while not STOP.is_set():
        t = time.time()
        r = []
        for u in ("http://127.0.0.1:11434/api/ps", "http://127.0.0.1:11434/api/tags", "http://127.0.0.1:11434/api/version",
                  "http://127.0.0.1:11435/api/ps"):
            st, _, _, dt = http("GET", u, timeout=10)
            r.append(round(dt * 1000, 1) if st == 200 else None)
        PS.append((PHASE[0], t, *r))
        try:
            b = board()
            if b != last:
                log("  QUADRO", b)
                last = b
        except Exception:
            pass
        STOP.wait(max(0.0, 1.0 - (time.time() - t)))


def ps_summary():
    log("== (g) tempos das leituras da checagem ativa (ms), por fase")
    phases = []
    for p in PS:
        if p[0] not in phases:
            phases.append(p[0])
    allmax = 0
    for ph in phases:
        xs = [p for p in PS if p[0] == ph]
        parts = []
        for i, nm in enumerate(("ps11434", "tags11434", "ver11434", "ps11435")):
            v = [x[2 + i] for x in xs if x[2 + i] is not None]
            nf = sum(1 for x in xs if x[2 + i] is None)
            if v:
                parts.append(f"{nm} n={len(v)} p50={statistics.median(v):.1f} max={max(v):.1f} falhas={nf}")
                if i < 3:
                    allmax = max(allmax, max(v))
            else:
                parts.append(f"{nm} n=0 falhas={nf}")
        log(f"  PS {ph}: " + " ; ".join(parts))
    return allmax


def sec(cmd):
    out = subprocess.run(["bash", f"{W}/sec.sh", cmd], capture_output=True, text=True).stdout
    for line in out.splitlines():
        log("  SEC", line)


def digests():
    d = {}
    for port in (11434, 11435):
        st, _, b, _ = http("GET", f"http://127.0.0.1:{port}/api/tags", timeout=10)
        if st == 200:
            for m in json.loads(b)["models"]:
                if m["name"].startswith("qwen3-ia"):
                    d[port] = m["digest"]
    return d


# ---------------------------------------------------------------------------------------------
def round_A():
    PHASE[0] = "A0-partida"
    s = wait_for(lambda: (lambda s: s if bk(s, "sec-cpu").get("digest") and bk(s, "gpu-local").get("digest") else None)(state()), 60, what="digests")
    log("QUADRO inicial:", board(s))
    d = digests()
    log(f"DIGEST qwen3-ia 11434={d.get(11434)} 11435={d.get(11435)} iguais={d.get(11434) == d.get(11435)}")
    show_events(0)

    PHASE[0] = "A1-aquecimento"
    w = [gen("w1"), gen("w2")]
    rows_for(w)

    if "a" not in ONLY:
        return
    # (a) repasse por sobrecarga
    PHASE[0] = "a-repasse"
    t0 = now_ms()
    res = burst(["a1", "a2", "a3"])
    rws = rows_for(res)
    ev = show_events(t0, {"overload_on", "overload_off"})
    on_sec = [r for r in rws if r[2] == "sec-cpu"]
    ok = (all(r["status"] == 200 and r["model"] == "qwen3-ia" for r in res) and len(on_sec) >= 1
          and all(r[3].startswith("F2:") and "S1" in r[3] for r in on_sec)
          and all(r[3] == "" for r in rws if r[2] == "gpu-local") and any(e[1] == "overload_on" for e in ev)
          and d.get(11434) == d.get(11435))
    verdict("a", ok, f"{len(on_sec)} de 3 no sec-cpu com regra {[r[3] for r in on_sec]}; todos 200 e model=qwen3-ia; digest igual")

    # (b) volta com histerese
    PHASE[0] = "b-histerese"
    t0b = now_ms()
    off = wait_for(lambda: events_since(t0, {"overload_off"}), 60, what="overload_off")
    show_events(t0, {"overload_on", "overload_off"})
    s = state()
    log("QUADRO apos a volta:", board(s))
    r1 = [gen("b1")]
    rb = rows_for(r1)
    # a calma conta do fim do ultimo pedido no PRIMARIO (o que roda no secundario nao entra na carga do primario)
    lastreq_end = q("SELECT MAX(finished_at) FROM requests WHERE backend='gpu-local' AND id IN (%s)" % ",".join("?" * len(res)), [r["rid"] for r in res])[0][0] or 0
    hold_ok = bool(off) and off[0][0] - lastreq_end >= 10000
    ok = bool(off) and bk(s, "gpu-local").get("state") == "ok" and rb and rb[0][2] == "gpu-local" and rb[0][3] == "" and hold_ok
    verdict("b", ok, f"overload_off {((off[0][0] - lastreq_end) / 1000) if off else '-'} s depois do fim do ultimo pedido (hold=10s); b1 em {rb[0][2] if rb else '-'} regra='{rb[0][3] if rb else '-'}'")

    # (e) alta com secundario cpu: nunca vai para ele
    PHASE[0] = "e-alta-cpu"
    t0 = now_ms()
    res = burst(["e1", "e2", "e3"], prio="alta")
    rws = rows_for(res)
    ev = show_events(t0, {"overload_on", "overload_off"})
    ok = (len(rws) == 3 and all(r[1] == "alta" and r[2] == "gpu-local" and r[3] == "" for r in rws)
          and all(r["status"] == 200 for r in res) and any(e[1] == "overload_on" for e in ev))
    verdict("e", ok, f"3 alta: backends={[r[2] for r in rws]} prio={[r[1] for r in rws]} com sobrecarga ligada={any(e[1] == 'overload_on' for e in ev)}")
    wait_for(lambda: events_since(t0, {"overload_off"}), 60, what="overload_off (e)")

    if "d" not in ONLY:
        return
    # (d) drenar e retomar pela API de admin
    PHASE[0] = "d-drenar"
    t0 = now_ms()
    pw = open(f"{W}/pw").read().strip()
    org = {"Origin": "http://127.0.0.1:11599"}
    st, hd, b, _ = http("POST", GW + "/monitor/api/login", {"user": "aceite", "password": pw}, org)
    ck = (hd.get("Set-Cookie") or "").split(";")[0]
    csrf = json.loads(b).get("csrf", "") if st == 200 else ""
    log(f"  LOGIN http={st} cookie={'sim' if ck.startswith('mcpgw_session=') else 'nao'} csrf={'sim' if csrf else 'nao'}")
    neg = {}
    neg["sem_sessao"] = http("POST", GW + "/monitor/api/backends/sec-cpu/drain", {}, {**org, "X-CSRF-Token": csrf})[0]
    neg["sem_csrf"] = http("POST", GW + "/monitor/api/backends/sec-cpu/drain", {}, {**org, "Cookie": ck})[0]
    neg["origem_alheia"] = http("POST", GW + "/monitor/api/backends/sec-cpu/drain", {}, {"Origin": "http://evil.example", "Cookie": ck, "X-CSRF-Token": csrf})[0]
    neg["csrf_errado"] = http("POST", GW + "/monitor/api/backends/sec-cpu/drain", {}, {**org, "Cookie": ck, "X-CSRF-Token": "x" * len(csrf)})[0]
    neg["id_desconhecido"] = http("POST", GW + "/monitor/api/backends/nao-existe/drain", {}, {**org, "Cookie": ck, "X-CSRF-Token": csrf})[0]
    log("  NEGATIVOS", neg)
    adm = {**org, "Cookie": ck, "X-CSRF-Token": csrf}
    st_d, _, bd, _ = http("POST", GW + "/monitor/api/backends/sec-cpu/drain", {"reason": "aceite 10.7"}, adm)
    log("  DRAIN sec-cpu", st_d, bd.decode()[:120])
    s = state()
    log("QUADRO drenado:", board(s))
    drained_state = bk(s, "sec-cpu").get("state")
    res = burst(["d1", "d2", "d3"])
    rws = rows_for(res)
    st_u, _, bu, _ = http("POST", GW + "/monitor/api/backends/sec-cpu/undrain", {"reason": "aceite 10.7"}, adm)
    log("  UNDRAIN sec-cpu", st_u, bu.decode()[:120])
    s2 = state()
    log("QUADRO retomado:", board(s2))
    wait_for(lambda: events_since(t0, {"overload_off"}), 60, what="overload_off (d)")
    # drenar o primario: o pedido vai ao secundario (F2:S3)
    st_p, _, bp, _ = http("POST", GW + "/monitor/api/backends/gpu-local/drain", {"reason": "aceite 10.7 primario"}, adm)
    log("  DRAIN gpu-local", st_p, bp.decode()[:120])
    r4 = [gen("d4")]
    rw4 = rows_for(r4)
    st_pu, _, _, _ = http("POST", GW + "/monitor/api/backends/gpu-local/undrain", {"reason": "aceite 10.7 primario"}, adm)
    log("  UNDRAIN gpu-local", st_pu)
    time.sleep(1)
    aud = q("SELECT ts,usuario,ip,acao,request_id,antes,depois,motivo FROM audit WHERE ts>=? ORDER BY ts", (t0,))
    for a in aud:
        log(f"  AUDIT {time.strftime('%H:%M:%S', time.localtime(a[0] / 1000))} user={a[1]} ip={a[2]} acao={a[3]} alvo={a[4]} {a[5]}->{a[6]} motivo={a[7]}")
    ev = show_events(t0, {"backend_drenado", "backend_retomado", "overload_on", "overload_off"})
    ok = (st == 200 and neg == {"sem_sessao": 401, "sem_csrf": 403, "origem_alheia": 403, "csrf_errado": 403, "id_desconhecido": 404}
          and st_d == 200 and drained_state == "drenado" and len(rws) == 3 and all(r[2] == "gpu-local" for r in rws)
          and st_u == 200 and bk(s2, "sec-cpu").get("state") in ("ok",) and st_p == 200 and rw4 and rw4[0][2] == "sec-cpu"
          and rw4[0][3].startswith("F2:") and "S3" in rw4[0][3] and st_pu == 200 and r4[0]["status"] == 200
          and len([a for a in aud if a[3] in ("drain", "undrain")]) == 4
          and len([e for e in ev if e[1] in ("backend_drenado", "backend_retomado")]) == 4)
    verdict("d", ok, f"login 200; negativos {neg}; sec-cpu drenado={drained_state} e 0/3 pedidos nele; retomado; primario drenado -> d4 em {rw4[0][2] if rw4 else '-'} regra '{rw4[0][3] if rw4 else '-'}'; auditoria {len(aud)} linhas")

    # (c) disjuntor
    PHASE[0] = "c-disjuntor"
    log("  morto (127.0.0.1:11436) desde a partida:")
    evm = [e for e in show_events(0, {"backend_down", "backend_up", "breaker_open", "breaker_close"}) if e[2].startswith("morto")]
    s = state()
    log("QUADRO:", board(s))
    morto_ok = (any(e[1] == "backend_down" for e in evm) and any(e[1] == "breaker_open" and "falhas seguidas" in e[2] for e in evm)
                and any(e[1] == "breaker_open" and "meio-aberto" in e[2] for e in evm) and not any(e[1] == "breaker_close" for e in evm))
    t0 = now_ms()
    sec("stop")
    op = wait_for(lambda: [e for e in events_since(t0, {"breaker_open"}) if e[2].startswith("sec-cpu")], 60, what="breaker_open sec-cpu")
    s_open = state()
    log("QUADRO sec parado:", board(s_open))
    time.sleep(3)
    sec("start")
    cl = wait_for(lambda: [e for e in events_since(t0, {"breaker_close"}) if e[2].startswith("sec-cpu")], 90, what="breaker_close sec-cpu")
    s_cl = state()
    log("QUADRO sec de volta:", board(s_cl))
    evs = [e for e in show_events(t0, {"backend_down", "backend_up", "breaker_open", "breaker_close"}) if e[2].startswith("sec-cpu")]
    t_open = op[0][0] if op else 0
    t_close = cl[0][0] if cl else 0
    ok = (morto_ok and bool(op) and "3 falhas" in op[0][2] and bk(s_open, "sec-cpu").get("state") in ("fora", "disjuntor_aberto")
          and bool(cl) and "checagem ativa" in cl[0][2] and t_close - t_open >= 20000 and bk(s_cl, "sec-cpu").get("breaker") == "fechado")
    verdict("c", ok, f"morto: down + aberto por 3 falhas + reaberto no meio-aberto, nunca fechou={morto_ok}; sec-cpu parado -> breaker_open '{op[0][2] if op else '-'}' -> volta -> breaker_close apos {(t_close - t_open) / 1000 if op and cl else '-'} s (breaker-open=20s)")


def round_B():
    PHASE[0] = "B0-partida"
    s = wait_for(lambda: (lambda s: s if bk(s, "sec-cpu").get("digest") and bk(s, "gpu-local").get("digest") else None)(state()), 60, what="digests")
    log("QUADRO inicial (sec tipo remoto, modelo nao residente):", board(s))
    PHASE[0] = "B1-aquecimento"
    rows_for([gen("bw1", "alta"), gen("bw2", "alta")])
    PHASE[0] = "B2-alta-remoto"
    t0 = now_ms()
    res = burst(["B1", "B2", "B3"], prio="alta")
    rws = rows_for(res)
    show_events(t0, {"overload_on", "overload_off"})
    on_sec = [r for r in rws if r[2] == "sec-cpu"]
    ok = len(on_sec) >= 1 and all(r[1] == "alta" and r[3].startswith("F2:") for r in on_sec) and all(r["status"] == 200 and r["model"] == "qwen3-ia" for r in res)
    verdict("e2-alta-remoto", ok, f"alta com tipo remoto: {len(on_sec)} no sec-cpu regra {[r[3] for r in on_sec]}")
    time.sleep(7)
    s = state()
    log("QUADRO depois da carga no secundario remoto:", board(s))
    deg = bk(s, "sec-cpu").get("state")
    wait_for(lambda: events_since(t0, {"overload_off"}), 60, what="overload_off (B)")
    PHASE[0] = "B3-remoto-degradado"
    res = burst(["B4", "B5", "B6"], prio="alta")
    rws = rows_for(res)
    verdict("e2-remoto-degradado", deg == "degradado" and all(r[2] == "gpu-local" for r in rws),
            f"secundario 'remoto' com o modelo a 100% CPU fica {deg}; nova rajada alta: {[r[2] for r in rws]}")


th = threading.Thread(target=watcher, daemon=True)
th.start()
boot_ok()
log(f"== rodada {ROUND} boot_id ok; uptime -s {subprocess.run(['uptime', '-s'], capture_output=True, text=True).stdout.strip()}")
try:
    round_B() if ROUND == "B" else round_A()
finally:
    STOP.set()
    th.join(timeout=15)
    mx = ps_summary()
    log(f"== (g) maior tempo de leitura no primario (ps/tags/version): {mx:.1f} ms; passa de 3 s: {mx > 3000}")
    log("== VEREDITOS")
    for v in VERD:
        log("  ", "|".join(v))
    open(f"{W}/ps-{ROUND}.json", "w").write(json.dumps(PS))
