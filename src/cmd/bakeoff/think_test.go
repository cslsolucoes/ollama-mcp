package main

import "testing"

// -think adds think:true only to the model tools, on a copy of the arguments.
func TestWithThink(t *testing.T) {
	args := map[string]any{"path": "x.docx", "instruction": "i"}
	got := withThink("extract", args, true)
	if got["think"] != true || got["path"] != "x.docx" {
		t.Fatalf("extract with -think: %v", got)
	}
	if _, ok := args["think"]; ok {
		t.Fatal("the caller's map was changed")
	}
	if g := withThink("extract", args, false); g["think"] != nil {
		t.Fatalf("without -think: %v", g)
	}
	for _, tool := range []string{"list_models", "embed", "reportar_erro"} {
		if g := withThink(tool, args, true); g["think"] != nil {
			t.Fatalf("%s must not get think: %v", tool, g)
		}
	}
	for _, tool := range []string{"generate", "chat", "summarize_file", "translate"} {
		if g := withThink(tool, map[string]any{}, true); g["think"] != true {
			t.Fatalf("%s without think", tool)
		}
	}
}
