//go:build windows

package ocr

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestWorkerStdioDetachesStdout proves both ways that WorkerStdio keeps
// native prints out of the frame channel: without it, every kind of print
// (os.Stdout, the Win32 handle, msvcrt _write and puts) reaches the pipe;
// with it, none does.
func TestWorkerStdioDetachesStdout(t *testing.T) {
	run := func(mode string) string {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, WorkerArgs(fakeConfig(t).WorkerConfig())...)
		cmd.Env = append(workerEnv(os.Environ()), "OCRTEST_MODE="+mode)
		cmd.Stdin = strings.NewReader("") // a pipe, not a console
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v %s", mode, err, errb.String())
		}
		return out.String()
	}
	control := run("noise-unprotected")
	for _, want := range []string{"NOISE os.Stdout", "NOISE win32", "NOISE crt write", "NOISE crt puts"} {
		if !strings.Contains(control, want) {
			t.Errorf("control: %q did not reach stdout, the test would prove nothing (got %q)", want, control)
		}
	}
	if got := run("noise-protected"); got != "" {
		t.Fatalf("with WorkerStdio, stdout received %q", got)
	}
}
