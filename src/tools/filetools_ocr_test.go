package tools

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
)

// OCR at the tool layer (step 7.9): the footer fields and the [aviso] that
// must follow the footer whenever text came from the OCR.

// cannedExtractor claims documents and images and returns a fixed result.
type cannedExtractor struct{ res files.Extracted }

func (cannedExtractor) IsDocument(name string) bool {
	n := strings.ToLower(name)
	for _, e := range []string{".pdf", ".png", ".jpg"} {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

func (c cannedExtractor) Extract(context.Context, string, []byte) (*files.Extracted, error) {
	r := c.res
	return &r, nil
}

func (e fileEnv) cannedDeps(t *testing.T, m *mock, res files.Extracted) Deps {
	t.Helper()
	d := e.deps(t, m, 128000, 40)
	if err := d.Files.SetExtractor(cannedExtractor{res: res}, 32<<20); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOCRFooterAndNotice(t *testing.T) {
	cases := []struct {
		name, file string
		res        files.Extracted
		footer     string
		notice     bool
	}{
		{"pdf with OCR", "scan.pdf",
			files.Extracted{Text: "[página 1 — OCR conf=93]\nabc", Format: "pdf", Pages: 3, PagesWithoutText: 2,
				OCR: files.OCRInfo{Used: true, Pages: 1, ConfMean: 93, Failed: 1}},
			" formato=pdf paginas=3 paginas_sem_texto=2 paginas_ocr=1 conf_ocr_media=93 paginas_ocr_falhas=1", true},
		{"pdf with pages left out", "scan.pdf",
			files.Extracted{Text: "[página 1 — OCR conf=70]\nabc", Format: "pdf", Pages: 4, PagesWithoutText: 4,
				OCR: files.OCRInfo{Used: true, Pages: 2, ConfMean: 70, NotProcessed: 2}},
			" formato=pdf paginas=4 paginas_sem_texto=4 paginas_ocr=2 conf_ocr_media=70 paginas_ocr_falhas=0 paginas_ocr_nao_processadas=2", true},
		{"pdf whose OCR pages all failed", "misto.pdf",
			files.Extracted{Text: "[página 1]\ntexto\n\n[página 2 — OCR falhou]", Format: "pdf", Pages: 2, PagesWithoutText: 1,
				OCR: files.OCRInfo{Used: true, Failed: 1}},
			" formato=pdf paginas=2 paginas_sem_texto=1 paginas_ocr=0 conf_ocr_media=0 paginas_ocr_falhas=1", false},
		{"png", "foto.png",
			files.Extracted{Text: "[imagem — OCR conf=88]\nabc", Format: "png", OCR: files.OCRInfo{Used: true, Pages: 1, ConfMean: 88}},
			" formato=png paginas_ocr=1 conf_ocr_media=88 paginas_ocr_falhas=0", true},
		{"jpg", "foto.jpg",
			files.Extracted{Text: "[imagem — OCR conf=61]\nabc", Format: "jpg", OCR: files.OCRInfo{Used: true, Pages: 1, ConfMean: 61}},
			" formato=jpg paginas_ocr=1 conf_ocr_media=61 paginas_ocr_falhas=0", true},
		{"pdf without OCR", "texto.pdf",
			files.Extracted{Text: "[página 1]\nabc", Format: "pdf", Pages: 1},
			" formato=pdf paginas=1 paginas_sem_texto=0", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newFileEnv(t)
			e.putBytes(t, c.file, []byte("conteúdo qualquer"))
			d := e.cannedDeps(t, newMock(t, generateOK), c.res)
			for _, tool := range []string{"summarize_file", "extract", "translate"} {
				args := `{"path":"` + c.file + `","target_language":"English"}`
				switch tool {
				case "summarize_file":
					args = `{"path":"` + c.file + `"}`
				case "extract":
					args = `{"path":"` + c.file + `","instruction":"CPF"}`
				}
				res, err := call(t, d, tool, args)
				if err != nil || res.IsError {
					t.Fatalf("%s: %v %+v", tool, err, res)
				}
				out := text(t, res)
				fl := fileLine(out)
				if !strings.HasSuffix(fl, c.footer) {
					t.Fatalf("%s footer: %q", tool, fl)
				}
				i := strings.Index(out, fl)
				hasNotice := strings.Contains(out, "\n"+ocrNotice)
				if hasNotice != c.notice {
					t.Fatalf("%s: aviso de OCR = %v, esperado %v\n%s", tool, hasNotice, c.notice, out)
				}
				if c.notice {
					if strings.Index(out, ocrNotice) < i {
						t.Fatalf("%s: aviso antes do rodapé", tool)
					}
					// It comes right after the footer, before the other notices.
					if !strings.Contains(out, fl+"\n"+ocrNotice) {
						t.Fatalf("%s: o aviso de OCR deve vir logo após o rodapé:\n%s", tool, out)
					}
				}
			}
		})
	}
}

// ocrDocExtractor is testDocExtractor with images and a fake OCR wired into
// doctext, as the binary does with the real one.
type ocrDocExtractor struct{ eng doctext.OCR }

func (x ocrDocExtractor) IsDocument(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".png") || strings.HasSuffix(n, ".jpg") || testDocExtractor{}.IsDocument(name)
}

func (x ocrDocExtractor) Extract(ctx context.Context, name string, data []byte) (*files.Extracted, error) {
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	f, err := doctext.Detect(name, head)
	if err != nil {
		return nil, err
	}
	lim := doctext.DefaultLimits()
	if x.eng != nil {
		lim.OCR = &doctext.OCROptions{Engine: x.eng}
	}
	r, err := doctext.Extract(ctx, f, data, lim)
	if err != nil {
		return nil, err
	}
	return &files.Extracted{Text: r.Text, Format: r.Format.String(), OCR: files.OCRInfo{
		Used: r.OCR.Used, Pages: r.OCR.Pages, ConfMean: r.OCR.ConfMean, Failed: r.OCR.Failed, NotProcessed: r.OCR.NotProcessed}}, nil
}

type stubOCR struct{}

func (stubOCR) Recognize(context.Context, *image.Gray, int) (string, int, int, error) {
	return "Nota fiscal nº 123 — São Paulo", 87, 0, nil
}

func inkPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 60, 40))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.SetGray(10, 10, color.Gray{0})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestImageThroughToolWithDoctext(t *testing.T) {
	e := newFileEnv(t)
	e.putBytes(t, "nota.png", inkPNG(t))
	e.putBytes(t, "falsa.png", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0})
	m := newMock(t, generateOK)
	d := e.deps(t, m, 128000, 40)
	if err := d.Files.SetExtractor(ocrDocExtractor{eng: stubOCR{}}, 32<<20); err != nil {
		t.Fatal(err)
	}
	res, err := call(t, d, "summarize_file", `{"path":"nota.png"}`)
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	out := text(t, res)
	if !strings.HasSuffix(fileLine(out), " formato=png paginas_ocr=1 conf_ocr_media=87 paginas_ocr_falhas=0") || !strings.Contains(out, ocrNotice) {
		t.Fatalf("saída:\n%s", out)
	}
	prompt, _ := m.body(0)["prompt"].(string)
	if !strings.Contains(prompt, "[imagem — OCR conf=87]\nNota fiscal nº 123 — São Paulo") {
		t.Fatalf("prompt sem o texto do OCR: %q", prompt)
	}
	// A .png holding a JPEG is refused by signature.
	res, err = call(t, d, "summarize_file", `{"path":"falsa.png"}`)
	if (err == nil && !res.IsError) || (err != nil && !strings.Contains(err.Error(), "não corresponde à extensão")) {
		t.Fatalf("png com conteúdo jpg: %v %+v", err, res)
	}
	// Without OCR the image is refused with the new message, no path.
	if err := d.Files.SetExtractor(ocrDocExtractor{}, 32<<20); err != nil {
		t.Fatal(err)
	}
	res, err = call(t, d, "summarize_file", `{"path":"nota.png"}`)
	if err == nil || !strings.Contains(err.Error(), "imagem requer OCR, que não está habilitado no servidor") {
		t.Fatalf("imagem sem OCR: %v %+v", err, res)
	}
	if strings.Contains(err.Error(), e.dir) {
		t.Fatalf("a recusa vaza o caminho: %v", err)
	}
}
