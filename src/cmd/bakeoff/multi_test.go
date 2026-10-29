package main

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		r    callRes
		want string
	}{
		{callRes{}, clsOK},
		{callRes{IsError: true, Err: "requisição recusada pelo servidor [HTTP 429 POST /api/generate]"}, clsHTTP429},
		{callRes{IsError: true, Err: "erro do servidor Ollama; o servidor pode estar reiniciando [HTTP 503 POST /api/generate]"}, clsHTTP5xx},
		{callRes{IsError: true, Err: "tempo esgotado após 10m0s (--timeout 10m0s)"}, clsTimeout},
		{callRes{IsError: true, Err: "o servidor encerrou a conexão após 3s sem resposta"}, clsConn},
		{callRes{IsError: true, Err: "não foi possível falar com https://x:8443: dial tcp"}, clsConn},
		{callRes{IsError: true, Proto: true, Err: "servidor_morreu: stdout do servidor fechou"}, clsProto},
		{callRes{IsError: true, Proto: true, Err: "timeout_driver: context deadline exceeded"}, clsTimeout},
		{callRes{IsError: true, Err: "arquivo fora das raízes"}, clsOther},
	}
	for _, c := range cases {
		if got := classify(c.r); got != c.want {
			t.Errorf("classify(%q)=%s, want %s", c.r.Err, got, c.want)
		}
	}
	if !abortsStep(clsConn) || !abortsStep(clsProto) || abortsStep(clsHTTP5xx) || abortsStep(clsTimeout) || abortsStep(clsHTTP429) {
		t.Fatal("only connection and protocol errors abort the step")
	}
}

func TestMultiTasks(t *testing.T) {
	s3 := []string{"b01.docx", "b02.pptx", "b03.pdf"}
	a0 := multiTasks(0, 3, `C:\c`, "b01.docx", s3, nil)
	a1 := multiTasks(1, 3, `C:\c`, "b01.docx", s3, nil)
	if len(a0) != 3 || a0[0].suite != "s2" || a0[1].suite != "s3" || a0[2].suite != "s2" {
		t.Fatalf("agent 0: %+v", a0)
	}
	if a1[0].suite != "s3" || a1[1].suite != "s2" || a1[2].suite != "s3" {
		t.Fatalf("agent 1: %+v", a1)
	}
	if a0[1].args["format"] != s3Formats[0].format || a0[1].label != "b02.pptx-json" {
		t.Fatalf("S3 task: %+v", a0[1])
	}
	big := multiTasks(4, 3, `C:\c`, "b01.docx", s3, []string{"x.txt", "y.txt"})
	if len(big) != 1 || big[0].tool != "summarize_file" || big[0].label != "x.txt" {
		t.Fatalf("big: %+v", big)
	}
}

func TestPercentileAndSummarize(t *testing.T) {
	if p := percentile([]float64{5, 1, 3, 2, 4}, 0.5); p != 3 {
		t.Fatalf("p50=%v", p)
	}
	if p := percentile([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.95); p != 10 {
		t.Fatalf("p95=%v", p)
	}
	t0 := time.Date(2026, 10, 28, 23, 0, 0, 0, time.Local)
	res := []multiRes{
		{callRes: callRes{Suite: "s2", WallMS: 10000, PromptE: 5000, EvalE: 1000}, Class: clsOK, StartOff: 0, EndOff: 10000, S2NT: 22, S2Total: 22, JSONOK: true},
		{callRes: callRes{Suite: "s3", WallMS: 20000, PromptE: 3000, EvalE: 1000}, Class: clsOK, StartOff: 0, EndOff: 20000, JSONOK: true, Strict: true},
		{callRes: callRes{Suite: "s2", WallMS: 1000}, Class: clsHTTP5xx, StartOff: 1000, EndOff: 2000},
		{callRes: callRes{Suite: "s2", WallMS: 1000}, Class: clsAborted, StartOff: 1000, EndOff: 90000},
	}
	s := summarize("m", "2x32k", 2, res, t0)
	if s.OK != 2 || s.Classes[clsHTTP5xx] != 1 || s.Classes[clsAborted] != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if s.WallS != 20 || s.TokS != 500 || s.PrefillTokS != 400 || s.GenTokS != 100 {
		t.Fatalf("throughput: wall=%v tok=%v pre=%v gen=%v", s.WallS, s.TokS, s.PrefillTokS, s.GenTokS)
	}
	if s.P50S != 10 || s.P95S != 20 || s.S2NT != 22 || s.S2Total != 22 || s.S3OK != 1 || s.S3Strict != 1 {
		t.Fatalf("latency/quality: %+v", s)
	}
	if s.FirstStart != "23:00:00" || s.LastEnd != "23:00:20" {
		t.Fatalf("window %s-%s", s.FirstStart, s.LastEnd)
	}
}

func TestScoreMulti(t *testing.T) {
	r := multiRes{callRes: callRes{Suite: "s2", text: "[{\"numero\":\"1.\",\"titulo\":\"Introdução\"}]\n\n[uso] modelo=m"}, Class: clsOK}
	scoreMulti(&r, testGab)
	if r.S2NT != 1 || r.S2Total != 3 || !r.JSONOK || !r.Strict {
		t.Fatalf("s2: %+v", r)
	}
	g := multiRes{callRes: callRes{Suite: "grande", text: "um resumo curto\n\n[uso] fim=stop", Fim: "stop"}, Class: clsOK}
	scoreMulti(&g, testGab)
	if g.Words != 3 || !g.JSONOK {
		t.Fatalf("grande: %+v", g)
	}
	cut := multiRes{callRes: callRes{Suite: "grande", text: "x", Fim: "length"}, Class: clsOK}
	scoreMulti(&cut, testGab)
	if cut.JSONOK {
		t.Fatal("a summary cut by length is not ok")
	}
}

func TestParseMon(t *testing.T) {
	lines := []string{
		"MON-INICIO 2026-10-28 22:50:00 boot=2026-10-28 21:10:46 xid=0 aer_1b4 corr=5 nonfatal=0 fatal=0",
		"P 23:59:58 0,50.0,40,9000,90 1,60.0,40,8000,90",
		"S 23:59:58 boot=2026-10-28 21:10:46 xid=0 aer_1b4 corr=5 nonfatal=0 fatal=0",
		"PS 23:59:58 gemma4:12b    abc    8.1 GB    100% GPU     32768      llamacpp    Forever",
		"P 00:00:01 0,99.5,41,9100,99 1,98.0,41,8100,99",
		"S 00:00:08 boot=2026-10-28 21:10:46 xid=0 aer_1b4 corr=9 nonfatal=0 fatal=0",
		"P 00:10:00 0,300.0,41,9100,99 1,300.0,41,8100,99",
	}
	s := parseMon(lines, "23:59:50", "00:00:10")
	if s.Samples != 2 || s.PeakSumW != 197.5 || s.PeakSumAt != "00:00:01" {
		t.Fatalf("power: %+v", s)
	}
	if s.AERFirst != "5" || s.AERLast != "9" || s.NonFatalOrFatal || s.MaxMemSumMiB != 17200 {
		t.Fatalf("aer/mem: %+v", s)
	}
	if !s.PS["gemma4:12b 100% GPU"] || !s.Boots["2026-10-28 21:10:46"] {
		t.Fatalf("ps/boots: %v %v", s.PS, s.Boots)
	}
}
