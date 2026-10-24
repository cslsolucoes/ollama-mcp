// Package files is the read-only file gate of the economy tools (phase 4):
// the only code in the server that opens a file on behalf of a tool call.
//
// It exists because the server reads files on the user's machine and ships
// the text to a remote model; that makes it the largest attack surface of the
// project (MCPO-REQ-SEC-011, FILE-001..017). The reference implementation it
// replaces, MCPVMWare's datastore tool, reads any path it is given: no
// allowlist, no symlink check, no size limit (LIC-VMW-014).
//
// The package is pure: it knows nothing about Ollama, prompts or MCP.
// policy.go holds the path rules that need no file system (Windows special
// paths, the fixed denylist); guard.go holds the allowlist, the resolution and
// the read.
package files

import (
	"path/filepath"
	"strings"
)

// Reason classifies a refusal, so callers and tests can tell the cases apart
// without parsing the (Portuguese) message.
type Reason int

const (
	// ReasonNotAllowed covers a missing file, a path outside every root and a
	// symlink that leaves the roots. The three share one message on purpose
	// (FILE-014): the answer never says which one it was.
	ReasonNotAllowed Reason = iota + 1
	ReasonDenylisted
	ReasonSpecialPath
	ReasonBadRequest
	ReasonNotRegular
	ReasonTooLarge
	ReasonBinary
	ReasonEncoding
)

// Refusal is the error of every rejected request. Its message is addressed to
// the user (pt-BR) and never carries a path or any file content.
type Refusal struct {
	Reason Reason
	msg    string
}

func (r *Refusal) Error() string { return r.msg }

func refuse(r Reason, msg string) *Refusal { return &Refusal{Reason: r, msg: msg} }

// errNotAllowed is the single opaque answer for "does not exist", "outside the
// roots" and "symlink to outside" (FILE-014).
func errNotAllowed() *Refusal {
	return refuse(ReasonNotAllowed, "arquivo não encontrado ou fora das raízes permitidas (--allow-root)")
}

func errDenylisted() *Refusal {
	return refuse(ReasonDenylisted, "acesso negado: o arquivo está na lista fixa de nomes sensíveis (.env, chaves, credenciais, .git...) e não pode ser lido")
}

func errSpecial(what string) *Refusal {
	return refuse(ReasonSpecialPath, "caminho não permitido: "+what)
}

// components splits a path into its non-empty segments, volume excluded.
func components(p string) []string {
	p = strings.TrimPrefix(p, filepath.VolumeName(p))
	return strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
}

// normComponent is the form compared against the denylist: lower case, and
// without the trailing dots and spaces that Windows silently drops (".env."
// opens ".env").
func normComponent(c string) string {
	return strings.ToLower(strings.TrimRight(c, ". "))
}

// Denied applies the fixed denylist (owner decision P-10; there is no flag
// that turns it off) to path segments. Every segment is tested, directories
// included: a file under a directory called "secrets" or ".git" is as
// sensitive as the directory.
//
//	.env*  *.env  *.pem  *.key  *.pfx  *.p12  id_rsa*  id_ed25519*
//	id_ecdsa*  id_dsa*  .mcp.json  settings.local.json
//	any name containing "credential" or "secret"  .git (and what is inside)
//
// *.env, id_ecdsa* and id_dsa* are supersets of the owner's list, kept because
// they are the same kind of secret.
func Denied(segments []string) bool {
	for _, seg := range segments {
		if deniedName(normComponent(seg)) {
			return true
		}
	}
	return false
}

func deniedName(n string) bool {
	switch {
	case n == "":
		return false
	case n == ".git", n == ".mcp.json", n == "settings.local.json":
		return true
	case strings.HasPrefix(n, ".env"), strings.HasSuffix(n, ".env"):
		return true
	case strings.HasSuffix(n, ".pem"), strings.HasSuffix(n, ".key"),
		strings.HasSuffix(n, ".pfx"), strings.HasSuffix(n, ".p12"):
		return true
	case strings.HasPrefix(n, "id_rsa"), strings.HasPrefix(n, "id_ed25519"),
		strings.HasPrefix(n, "id_ecdsa"), strings.HasPrefix(n, "id_dsa"):
		return true
	case strings.Contains(n, "credential"), strings.Contains(n, "secret"):
		return true
	}
	return false
}

// deviceNames are the DOS device names Windows resolves in any directory, with
// or without an extension ("COM1.txt" is still the serial port).
var deviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"conin$": true, "conout$": true, "clock$": true,
}

func isDeviceName(seg string) bool {
	n := strings.ToLower(strings.TrimRight(seg, ". "))
	if i := strings.IndexByte(n, '.'); i >= 0 {
		n = n[:i]
	}
	n = strings.TrimRight(n, " ")
	if deviceNames[n] {
		return true
	}
	// com1..com9 and lpt1..lpt9, plus the superscript digits Windows also maps.
	for _, p := range []string{"com", "lpt"} {
		if strings.HasPrefix(n, p) {
			switch strings.TrimPrefix(n, p) {
			case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
				return true
			}
		}
	}
	return false
}

// checkSyntax rejects, from the string alone, the paths whose meaning on
// Windows is not "a file under a directory": UNC shares, the \\?\ and \\.\
// prefixes, NTFS alternate data streams, device names, drive-relative paths
// and wildcard characters. It is a pure function of the text so it can be
// tested on any OS; production passes windows = (GOOS == "windows").
//
// The NUL-in-path and the double-backslash prefix are refused on every OS:
// neither has a legitimate use here.
func checkSyntax(raw string, windows bool) *Refusal {
	if strings.ContainsRune(raw, 0) {
		return errSpecial("o caminho contém um caractere nulo")
	}
	if strings.HasPrefix(raw, `\\`) {
		return errSpecial(`caminhos UNC (\\servidor\compartilhamento) e os prefixos \\?\ e \\.\ são recusados`)
	}
	if !windows {
		return nil
	}
	if strings.HasPrefix(raw, "//") {
		return errSpecial("caminhos UNC (//servidor/compartilhamento) e os prefixos //?/ e //./ são recusados")
	}
	vol := filepath.VolumeName(raw)
	rest := raw
	if len(raw) >= 2 && raw[1] == ':' {
		if !isDriveLetter(raw[0]) {
			return errSpecial("fluxos NTFS alternativos (arquivo:fluxo) são recusados")
		}
		rest = raw[2:]
		if rest == "" || (rest[0] != '\\' && rest[0] != '/') {
			return errSpecial("caminhos relativos à unidade (C:arquivo) são recusados; use um caminho completo")
		}
	} else if vol != "" {
		return errSpecial("caminhos UNC e com prefixo de dispositivo são recusados")
	}
	if strings.ContainsRune(rest, ':') {
		return errSpecial("fluxos NTFS alternativos (arquivo:fluxo) são recusados")
	}
	if strings.ContainsAny(rest, `<>"|?*`) {
		return errSpecial("caracteres curinga ou inválidos no Windows (< > \" | ? *) são recusados")
	}
	for _, seg := range components(rest) {
		if isDeviceName(seg) {
			return errSpecial("nomes de dispositivo do Windows (CON, PRN, AUX, NUL, COM1-9, LPT1-9) são recusados")
		}
	}
	return nil
}

func isDriveLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// within reports whether p is inside root (or is root itself) and returns the
// path relative to root. Containment is decided by filepath.Rel on whole
// segments, never by a string prefix: C:\dados does not contain
// C:\dados-outro (FILE-003). On Windows filepath.Rel compares case-
// insensitively and refuses different volumes.
func within(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
