package doctext

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"runtime"
	"strings"
	"testing"
)

// inkImage is a w x h white image with a dark block in the middle (so it is
// not blank) and pixel (0,0) dark too (the fake OCR reads it as "page 0").
func inkImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{255, 255, 255, 255}
			if (x > w/4 && x < 3*w/4 && y > h/3 && y < h/2) || (x == 0 && y == 0) {
				c = color.NRGBA{10, 20, 30, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func encodePNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPEG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pngHeaderOnly is a PNG with a valid IHDR declaring w x h (color type ct,
// bit depth bd) and no pixel data: what a decompression bomb looks like to
// DecodeConfig.
func pngHeaderOnly(w, h uint32, bd, ct byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(typ string, data []byte) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		crc := crc32.ChecksumIEEE(append([]byte(typ), data...))
		_ = binary.Write(&b, binary.BigEndian, crc)
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = bd, ct
	chunk("IHDR", ihdr)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(make([]byte, 64))
	_ = zw.Close()
	chunk("IDAT", z.Bytes())
	chunk("IEND", nil)
	return b.Bytes()
}

func TestImagePNGAndJPEG(t *testing.T) {
	for _, c := range []struct {
		name   string
		format Format
		data   []byte
	}{
		{"png", FormatPNG, encodePNG(t, inkImage(400, 300))},
		{"jpg", FormatJPEG, encodeJPEG(t, inkImage(400, 300))},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
				return "Recibo nº 42 — São João\r\nvalor R$ 1.234,56", 91, 0, nil
			}}
			r, err := Extract(context.Background(), c.format, c.data, ocrLimits(f, nil))
			if err != nil {
				t.Fatal(err)
			}
			want := "[imagem — OCR conf=91]\nRecibo nº 42 — São João\nvalor R$ 1.234,56"
			if r.Text != want || r.Format != c.format {
				t.Fatalf("texto %q formato %v", r.Text, r.Format)
			}
			if r.OCR != (OCRStats{Used: true, Pages: 1, ConfMean: 91}) {
				t.Fatalf("stats %+v", r.OCR)
			}
			if f.calls() != 1 || f.sizes[0] != (image.Point{400, 300}) || f.dpis[0] != DefaultOCRDPI {
				t.Fatalf("chamadas %d, tamanhos %v, dpi %v", f.calls(), f.sizes, f.dpis)
			}
		})
	}
}

func TestImageLowConfidenceAndRotation(t *testing.T) {
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "texto ruim", 33, 180, nil
	}}
	r, err := Extract(context.Background(), FormatPNG, encodePNG(t, inkImage(50, 50)), ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Text, "[imagem — OCR baixa confiança conf=33 girada=180]\n") {
		t.Fatalf("marcador: %q", r.Text)
	}
}

func TestImageWithoutOCRIsRefused(t *testing.T) {
	for f, data := range map[Format][]byte{FormatPNG: encodePNG(t, inkImage(20, 20)), FormatJPEG: encodeJPEG(t, inkImage(20, 20))} {
		_, err := Extract(context.Background(), f, data, Limits{})
		if r, _ := ReasonOf(err); r != ReasonOCRRequired {
			t.Fatalf("%v sem OCR: %v", f, err)
		}
		if err.Error() != "imagem requer OCR, que não está habilitado no servidor" {
			t.Fatalf("mensagem: %q", err.Error())
		}
	}
}

func TestImageBombRefusedWithoutAllocating(t *testing.T) {
	f := &fakeOCR{}
	bomb := pngHeaderOnly(100_000, 100_000, 8, 0)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := Extract(context.Background(), FormatPNG, bomb, ocrLimits(f, nil))
	runtime.ReadMemStats(&after)
	if r, _ := ReasonOf(err); r != ReasonImageTooLarge {
		t.Fatalf("bomba: %v", err)
	}
	if f.calls() != 0 {
		t.Fatal("a bomba chegou ao OCR")
	}
	if d := after.TotalAlloc - before.TotalAlloc; d > 4<<20 {
		t.Fatalf("a recusa alocou %d bytes", d)
	}
	t.Logf("bomba 100000x100000 recusada; alocado %d bytes", after.TotalAlloc-before.TotalAlloc)

	// Within the pixel factor but over the memory budget (16-bit RGBA).
	_, err = Extract(context.Background(), FormatPNG, pngHeaderOnly(6000, 6000, 16, 6), ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonImageTooLarge {
		t.Fatalf("36 MP RGBA de 16 bits: %v", err)
	}
	// Just over twice the ceiling.
	_, err = Extract(context.Background(), FormatPNG, pngHeaderOnly(9000, 9000, 8, 0), ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonImageTooLarge {
		t.Fatalf("81 MP: %v", err)
	}
}

func TestImageReducedToTheCeiling(t *testing.T) {
	f := &fakeOCR{}
	data := encodePNG(t, inkImage(2000, 1000)) // 2 MP, ceiling 1 MP
	r, err := Extract(context.Background(), FormatPNG, data, ocrLimits(f, func(o *OCROptions) { o.MaxMegapixels = 1 }))
	if err != nil {
		t.Fatalf("imagem acima do teto deveria ser reduzida: %v", err)
	}
	sz := f.sizes[0]
	if sz.X*sz.Y > 1_000_000 || sz.X < 1300 || sz.Y < 650 {
		t.Fatalf("tamanho reduzido %v", sz)
	}
	if f.dpis[0] >= DefaultOCRDPI || f.dpis[0] < 200 {
		t.Fatalf("dpi declarado %d", f.dpis[0])
	}
	if r.OCR.Pages != 1 {
		t.Fatalf("stats %+v", r.OCR)
	}
}

func TestImageMalformedAndBlank(t *testing.T) {
	f := &fakeOCR{}
	png := encodePNG(t, inkImage(30, 30))
	for name, data := range map[string][]byte{
		"truncated png":  png[:len(png)/2],
		"signature only": []byte("\x89PNG\r\n\x1a\n"),
		"truncated jpg":  encodeJPEG(t, inkImage(30, 30))[:40],
	} {
		format := FormatPNG
		if strings.HasSuffix(name, "jpg") {
			format = FormatJPEG
		}
		_, err := Extract(context.Background(), format, data, ocrLimits(f, nil))
		if r, _ := ReasonOf(err); r != ReasonMalformed {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// Transparent black pixels are white over the page, not black ink.
	clear := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	_, err := Extract(context.Background(), FormatPNG, encodePNG(t, clear), ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonOCRNoText {
		t.Fatalf("imagem transparente: %v", err)
	}
	if f.calls() != 0 {
		t.Fatal("imagem malformada ou em branco chegou ao OCR")
	}
}

func TestImageOCRFailureAndUnavailable(t *testing.T) {
	data := encodePNG(t, inkImage(30, 30))
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "", 0, 0, fmt.Errorf("falha")
	}}
	_, err := Extract(context.Background(), FormatPNG, data, ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonOCRNoText {
		t.Fatalf("falha do OCR na imagem: %v", err)
	}
	f = &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "", 0, 0, fmt.Errorf("x: %w", ErrOCRUnavailable)
	}}
	_, err = Extract(context.Background(), FormatPNG, data, ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonOCRUnavailable {
		t.Fatalf("OCR indisponível na imagem: %v", err)
	}
}

func TestGrayConversionPaths(t *testing.T) {
	// Every type the standard decoders return gives the same gray for the
	// same color (within rounding), composited over white.
	red := color.NRGBA{200, 30, 40, 255}
	want := uint8(luma8(200, 30, 40))
	pal := image.NewPaletted(image.Rect(0, 0, 3, 1), color.Palette{red})
	nrgba := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	rgba := image.NewRGBA(image.Rect(0, 0, 3, 1))
	rgba64 := image.NewRGBA64(image.Rect(0, 0, 3, 1))
	ycc := image.NewYCbCr(image.Rect(0, 0, 3, 1), image.YCbCrSubsampleRatio444)
	yy, cb, cr := color.RGBToYCbCr(200, 30, 40)
	for x := 0; x < 3; x++ {
		nrgba.Set(x, 0, red)
		rgba.Set(x, 0, red)
		rgba64.Set(x, 0, red)
		ycc.Y[x], ycc.Cb[x], ycc.Cr[x] = yy, cb, cr
	}
	for name, img := range map[string]image.Image{"paletted": pal, "nrgba": nrgba, "rgba": rgba, "rgba64": rgba64, "ycbcr": ycc} {
		g := grayFit(img, 1)
		for _, v := range g.Pix {
			if d := int(v) - int(want); d < -2 || d > 2 {
				t.Fatalf("%s: %d, esperado %d", name, v, want)
			}
		}
	}
	// Half transparent black over white is mid gray.
	half := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	half.Set(0, 0, color.NRGBA{0, 0, 0, 128})
	if v := grayFit(half, 1).Pix[0]; v < 120 || v > 135 {
		t.Fatalf("meio transparente: %d", v)
	}
	// Reduction averages: a 4x4 checkerboard becomes 2x2 mid gray.
	chk := image.NewGray(image.Rect(0, 0, 4, 4))
	for i := range chk.Pix {
		if (i/4+i%4)%2 == 0 {
			chk.Pix[i] = 255
		}
	}
	g := grayFit(chk, 0.5)
	if g.Rect.Dx() != 2 || g.Rect.Dy() != 2 {
		t.Fatalf("redução: %v", g.Rect)
	}
	for _, v := range g.Pix {
		if v < 126 || v > 129 {
			t.Fatalf("média do xadrez: %v", g.Pix)
		}
	}
}

func TestJPEGProgressiveDetection(t *testing.T) {
	base := encodeJPEG(t, inkImage(16, 16))
	if jpegProgressive(base) {
		t.Fatal("o encoder da stdlib grava baseline")
	}
	prog := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x04, 0x00, 0x00, 0xFF, 0xC2, 0x00, 0x0B}
	if !jpegProgressive(prog) {
		t.Fatal("SOF2 é progressivo")
	}
	for _, bad := range [][]byte{nil, {0xFF, 0xD8}, {0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x00}, {0xFF, 0xD8, 0x00, 0x00, 0x00}, {0xFF, 0xD8, 0xFF, 0xE0, 0xFF, 0xFF}} {
		if jpegProgressive(bad) {
			t.Fatalf("%x não deveria ser progressivo", bad)
		}
	}
}

// patchedJPEG is a real stdlib color JPEG whose SOF0 declares w x h (SOF2,
// progressive, when asked): DecodeConfig accepts it, decoding would not.
func patchedJPEG(t *testing.T, progressive bool, w, h int) []byte {
	t.Helper()
	d := encodeJPEG(t, inkImage(16, 16))
	i := bytes.Index(d, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("SOF0 not found")
	}
	if progressive {
		d[i+1] = 0xC2
	}
	d[i+5], d[i+6], d[i+7], d[i+8] = byte(h>>8), byte(h), byte(w>>8), byte(w)
	return d
}

func TestJPEGBombsRefusedWithoutAllocating(t *testing.T) {
	for name, data := range map[string][]byte{
		"65500x65500 baseline":    patchedJPEG(t, false, 65500, 65500),
		"8000x6000 progressive":   patchedJPEG(t, true, 8000, 6000),
		"9000x9000 baseline 81MP": patchedJPEG(t, false, 9000, 9000),
	} {
		f := &fakeOCR{}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := Extract(context.Background(), FormatJPEG, data, ocrLimits(f, nil))
		runtime.ReadMemStats(&after)
		if r, _ := ReasonOf(err); r != ReasonImageTooLarge || f.calls() != 0 {
			t.Fatalf("%s: %v, %d chamadas", name, err, f.calls())
		}
		if d := after.TotalAlloc - before.TotalAlloc; d > 4<<20 {
			t.Fatalf("%s: a recusa alocou %d bytes", name, d)
		}
	}
	if !jpegProgressive(patchedJPEG(t, true, 10, 10)) || jpegProgressive(patchedJPEG(t, false, 10, 10)) {
		t.Fatal("detecção de progressivo em JPEG real")
	}
}
