package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Secret-bearing names the fixed denylist (owner decision P-10) does not cover.
// Policy gap, reported as ACHADO, not as a code escape.
func TestAttackDenylistGaps(t *testing.T) {
	var gaps []string
	for _, n := range []string{
		".npmrc", ".pypirc", ".netrc", "_netrc", ".pgpass", ".htpasswd", ".git-credentials",
		"putty.ppk", "release.jks", "app.keystore", "vault.kdbx", "client.ovpn", "terraform.tfstate",
		"kubeconfig", filepath.Join(".kube", "config"), filepath.Join(".docker", "config.json"),
		filepath.Join(".ssh", "config"), filepath.Join(".ssh", "known_hosts"), filepath.Join(".ssh", "github"),
		filepath.Join(".aws", "config"), "token.txt", "apikey.txt", "passwords.txt", "web.config", "appsettings.json",
		"id_rsa.pub", "csl-ollama.env", ".claude.json",
	} {
		if !Denied(components(n)) {
			gaps = append(gaps, n)
		}
	}
	t.Logf("VEC|deny/gaps|ACHADO|nao cobertos pela denylist fixa: %s", strings.Join(gaps, " "))
}

// A per-directory case-sensitive folder: root "case\root" and a sibling
// "case\Root". filepath.Rel compares case-insensitively; os.Root must still
// open only the real root.
func TestAttackCaseSensitiveDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	w := newWorld(t)
	cs := filepath.Join(w.base, "case")
	os.MkdirAll(cs, 0o755)
	if out, err := exec.Command("fsutil", "file", "setCaseSensitiveInfo", cs, "enable").CombinedOutput(); err != nil {
		t.Logf("VEC|case-sensitive-dir|NAO-TESTAVEL|fsutil setCaseSensitiveInfo: %v %s", err, strings.TrimSpace(string(out)))
		return
	}
	lower := filepath.Join(cs, "root")
	upper := filepath.Join(cs, "Root")
	os.MkdirAll(lower, 0o755)
	if err := os.MkdirAll(upper, 0o755); err != nil {
		t.Logf("VEC|case-sensitive-dir|NAO-TESTAVEL|%v", err)
		return
	}
	write(t, filepath.Join(lower, "f.txt"), "INSIDE")
	write(t, filepath.Join(upper, "f.txt"), outsideContent)
	g := w.guard(t, 1<<20, lower)
	f, err := g.Read(filepath.Join(upper, "f.txt"))
	switch {
	case err != nil:
		t.Logf("VEC|case-sensitive-dir/sibling-Root|BLOQUEADO|%v", err)
	case strings.Contains(f.Text, outsideContent):
		t.Errorf("VEC|case-sensitive-dir/sibling-Root|ESCAPOU|leu o irmao Root fora da raiz")
	default:
		// filepath.Rel (case-insensitive) took case\Root for the root; os.Root
		// opened the real case\root, so the INSIDE file was returned instead.
		t.Logf("VEC|case-sensitive-dir/sibling-Root|BLOQUEADO|sem vazamento; pedido de case\\Root\\f.txt devolveu %q (arquivo da raiz real: confusao de nome, nao fuga)", f.Text)
	}
}
