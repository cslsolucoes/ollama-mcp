package doctext

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Package is an OOXML (ZIP) package opened from memory. Nothing is written to
// disk and nothing is extracted wholesale: a part is read only by its exact
// normalized name, through a bounded reader, and the real decompressed bytes
// of every read are summed against MaxTotalBytes for the life of the Package.
// A Package is not safe for concurrent use.
type Package struct {
	lim   Limits
	files map[string]*zip.File // normalized name -> entry
	lower map[string]string    // lower-case normalized name -> normalized name
	total int64                // real decompressed bytes read so far
}

// testHookEntry is called once per entry while names are validated. Tests use
// it to prove that the entry count is checked before any iteration.
var testHookEntry func()

// OpenPackage opens a ZIP held in data. The entry count is checked before any
// entry is visited; unsafe, duplicate, encrypted or oddly compressed entries
// refuse the whole package (a duplicate would let one part hide behind
// another). Every failure is the opaque ReasonMalformed.
func OpenPackage(data []byte, lim Limits) (*Package, error) {
	lim = lim.withDefaults()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil && !(errors.Is(err, zip.ErrInsecurePath) && zr != nil) {
		return nil, refuse(ReasonMalformed)
	}
	if len(zr.File) > lim.MaxEntries {
		return nil, refuse(ReasonMalformed)
	}
	p := &Package{lim: lim, files: make(map[string]*zip.File, len(zr.File)), lower: make(map[string]string, len(zr.File))}
	seen := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		if testHookEntry != nil {
			testHookEntry()
		}
		name, ok := normalizeEntryName(f.Name)
		if !ok {
			return nil, refuse(ReasonMalformed)
		}
		// OOXML part names are case-insensitive, so two entries that differ
		// only by case are a duplicate too.
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return nil, refuse(ReasonMalformed)
		}
		seen[key] = struct{}{}
		if f.Flags&0x1 != 0 { // encrypted
			return nil, refuse(ReasonMalformed)
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, refuse(ReasonMalformed)
		}
		if strings.HasSuffix(f.Name, "/") { // directory marker: never a part
			continue
		}
		p.files[name] = f
		p.lower[key] = name
	}
	return p, nil
}

// normalizeEntryName validates a raw entry name and returns its path.Clean
// form. It refuses backslashes, NUL, absolute names, drive letters and any
// ".." segment (checked on the raw name, before Clean could hide it).
func normalizeEntryName(raw string) (string, bool) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00") || strings.HasPrefix(raw, "/") {
		return "", false
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return "", false
		}
	}
	if len(raw) >= 2 && raw[1] == ':' {
		return "", false
	}
	n := path.Clean(raw)
	if n == "." || n == ".." || strings.HasPrefix(n, "../") {
		return "", false
	}
	return n, true
}

// Names returns the part names in sorted order (deterministic).
func (p *Package) Names() []string {
	out := make([]string, 0, len(p.files))
	for n := range p.files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Has reports whether a part with exactly this normalized name exists. The
// extraction uses it for word/document.xml and ppt/presentation.xml.
func (p *Package) Has(name string) bool {
	n, ok := normalizeEntryName(name)
	if !ok {
		return false
	}
	_, found := p.lookup(n)
	return found
}

// lookup finds a part by its normalized name, case-insensitively as OPC
// requires (a .rels may say "Header1.xml" for the entry "word/header1.xml").
// Duplicates that differ only by case were already refused by OpenPackage.
func (p *Package) lookup(n string) (*zip.File, bool) {
	if f, ok := p.files[n]; ok {
		return f, true
	}
	if real, ok := p.lower[strings.ToLower(n)]; ok {
		return p.files[real], true
	}
	return nil, false
}

// Read returns the decompressed bytes of one part. A missing part, a
// decompression error or a limit exceeded is ReasonMalformed.
//
// The declared (central directory) size is used only in ways a lie cannot
// exploit. archive/zip fails a read that yields more bytes than declared
// (ErrFormat) or fewer (ErrUnexpectedEOF), so a part that declares more than
// the limit can never be read successfully: it is refused before anything is
// decompressed. Otherwise the buffer is pre-sized to the declared size, which
// keeps the peak at about one copy of the part (io.ReadAll held ~2.4x: growing
// chunks plus the final copy). A header that lies upward only buys an
// allocation no larger than the limit, and that allocation is charged to
// MaxTotalBytes like real bytes. The bytes really produced are still counted
// against the limit, independently of archive/zip.
func (p *Package) Read(name string) ([]byte, error) {
	n, ok := normalizeEntryName(name)
	if !ok {
		return nil, refuse(ReasonMalformed)
	}
	f, found := p.lookup(n)
	if !found {
		return nil, refuse(ReasonMalformed)
	}
	remaining := p.lim.MaxTotalBytes - p.total
	if remaining < 0 {
		return nil, refuse(ReasonMalformed)
	}
	max := p.lim.MaxEntryBytes
	if remaining < max {
		max = remaining
	}
	if f.UncompressedSize64 > uint64(max) {
		// Over the per-entry cap or over what is left of the total budget.
		// Charged as if max+1 bytes had been read, so the budget ends in the
		// same state as when the limit is hit while reading.
		p.total += max + 1
		return nil, refuse(ReasonMalformed)
	}
	hint := int64(f.UncompressedSize64)
	rc, err := f.Open()
	if err != nil {
		return nil, refuse(ReasonMalformed)
	}
	defer rc.Close()
	buf, err := readBounded(rc, hint, max)
	charged := int64(len(buf))
	if charged < hint {
		charged = hint // the pre-sized allocation counts even if unused
	}
	p.total += charged
	if err != nil {
		return nil, refuse(ReasonMalformed)
	}
	return buf, nil
}

// errTooBig is returned by readBounded when more than max bytes arrive.
var errTooBig = errors.New("doctext: part over the limit")

// readBounded reads r to EOF into a buffer pre-sized to hint (0 <= hint <=
// max) and fails with errTooBig as soon as more than max bytes have arrived.
// The +1 of capacity lets an exact hint reach EOF without growing. Past the
// hint the buffer grows like append, still checked against max after every
// Read, so a reader that ignores its declared size cannot go far past max.
func readBounded(r io.Reader, hint, max int64) ([]byte, error) {
	buf := make([]byte, 0, hint+1)
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if int64(len(buf)) > max {
			return buf, errTooBig
		}
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return buf, err
		}
	}
}

// Relationship is one internal entry of a .rels part. Target is already
// resolved to a part name relative to the package root.
type Relationship struct {
	ID     string
	Type   string
	Target string
}

// relsName is the .rels part that describes the given part ("" is the package).
func relsName(part string) string {
	dir, base := path.Split(part)
	return path.Join(dir, "_rels", base+".rels")
}

// Relationships reads and resolves the relationships of a part (for example
// "word/document.xml"). A part with no .rels yields nil, nil. A Target is
// resolved against the directory of the part and path.Clean-ed; one that is
// absolute, uses a backslash or a scheme, or leaves the package root, refuses
// the package. TargetMode="External" entries are skipped, never resolved and
// never followed. Entries come in document order.
func (p *Package) Relationships(part string) ([]Relationship, error) {
	rels := relsName(part)
	if !p.Has(rels) {
		return nil, nil
	}
	data, err := p.Read(rels)
	if err != nil {
		return nil, err
	}
	dec := NewDecoder(bytes.NewReader(data), p.lim)
	baseDir := path.Dir(part)
	if baseDir == "." {
		baseDir = ""
	}
	var out []Relationship
	for {
		tok, err := dec.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !IsPkgRel(se.Name, "Relationship") {
			continue
		}
		var r Relationship
		external := false
		target := ""
		for _, a := range se.Attr {
			if a.Name.Space != "" {
				continue
			}
			switch a.Name.Local {
			case "Id":
				r.ID = a.Value
			case "Type":
				r.Type = a.Value
			case "Target":
				target = a.Value
			case "TargetMode":
				external = strings.EqualFold(a.Value, "External")
			}
		}
		if external {
			continue
		}
		resolved, ok := resolveTarget(baseDir, target)
		if !ok {
			return nil, refuse(ReasonMalformed)
		}
		r.Target = resolved
		out = append(out, r)
	}
}

// resolveTarget resolves an internal relationship target against baseDir and
// refuses anything that is not a plain part name inside the package.
func resolveTarget(baseDir, target string) (string, bool) {
	if target == "" || strings.ContainsAny(target, "\\\x00") || strings.HasPrefix(target, "/") {
		return "", false
	}
	if u, err := url.PathUnescape(target); err == nil {
		target = u
	} else {
		return "", false
	}
	if strings.ContainsAny(target, "\\\x00") || strings.HasPrefix(target, "/") {
		return "", false
	}
	// A scheme ("http:", "file:") or drive letter before the first slash.
	if i := strings.IndexByte(target, ':'); i >= 0 && !strings.Contains(target[:i], "/") {
		return "", false
	}
	joined := path.Clean(path.Join(baseDir, target))
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") || strings.HasPrefix(joined, "/") {
		return "", false
	}
	return joined, true
}
