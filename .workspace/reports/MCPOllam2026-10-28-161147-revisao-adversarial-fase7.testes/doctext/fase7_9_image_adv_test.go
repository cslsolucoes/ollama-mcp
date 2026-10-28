package doctext

// Vetores de imagem da etapa 7.9 (OCR de .png/.jpg), no mesmo roteiro da
// revisao adversarial da Fase 7: VEC|img/<nome>|<resultado>|<detalhe>.
// Usa o vec/reasonStr de fase7_common_adv_test.go. Nenhuma DLL: o OCR e falso
// e conta as chamadas (uma imagem hostil nao pode chegar a ele).

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"runtime"
	"sync/atomic"
	"testing"
)

type advOCR struct{ calls atomic.Int32 }

func (a *advOCR) Recognize(context.Context, *image.Gray, int) (string, int, int, error) {
	a.calls.Add(1)
	return "texto", 90, 0, nil
}

func advPNG(w, h uint32, bd, ct byte, idat []byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(typ string, data []byte) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = bd, ct
	chunk("IHDR", ihdr)
	chunk("IDAT", idat)
	chunk("IEND", nil)
	return b.Bytes()
}

func advZlib(n int) []byte {
	var z bytes.Buffer
	w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
	_, _ = w.Write(make([]byte, n))
	_ = w.Close()
	return z.Bytes()
}

// advJPEGPatched encodes a small color JPEG with the standard library and
// patches its SOF0 to declare w x h (and to SOF2, progressive, when asked).
func advJPEGPatched(progressive bool, w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = byte(i)
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	d := b.Bytes()
	i := bytes.Index(d, []byte{0xFF, 0xC0})
	if progressive {
		d[i+1] = 0xC2
	}
	d[i+5], d[i+6] = byte(h>>8), byte(h)
	d[i+7], d[i+8] = byte(w>>8), byte(w)
	return d
}

// advImage runs one image through Extract with the fake OCR and measures the
// bytes allocated.
func advImage(f Format, data []byte) (calls int32, alloc uint64, err error) {
	o := &advOCR{}
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	_, err = Extract(context.Background(), f, data, Limits{OCR: &OCROptions{Engine: o}})
	runtime.ReadMemStats(&b)
	return o.calls.Load(), b.TotalAlloc - a.TotalAlloc, err
}

func TestAdvImages(t *testing.T) {
	// 1. PNG bomb: IHDR declares 100000 x 100000 (10 Gpx) in a few bytes.
	calls, alloc, err := advImage(FormatPNG, advPNG(100000, 100000, 8, 0, advZlib(64)))
	if r, _ := ReasonOf(err); r == ReasonImageTooLarge && calls == 0 && alloc < 4<<20 {
		vec(t, "img", "png-bomba-ihdr-100k", "BLOQUEADO", fmt.Sprintf("%s alocado=%d B ocr=%d", reasonStr(err), alloc, calls))
	} else {
		vec(t, "img", "png-bomba-ihdr-100k", "ESCAPOU", fmt.Sprintf("%s alocado=%d B ocr=%d", reasonStr(err), alloc, calls))
	}

	// 2. JPEG bomb: a real stdlib JPEG whose SOF0 is patched to declare
	// 65500 x 65500 (a hand-written SOF alone is malformed for DecodeConfig,
	// which reads up to the SOS for 3 components).
	calls, alloc, err = advImage(FormatJPEG, advJPEGPatched(false, 65500, 65500))
	if r, _ := ReasonOf(err); r == ReasonImageTooLarge && calls == 0 && alloc < 4<<20 {
		vec(t, "img", "jpeg-bomba-sof-65500", "BLOQUEADO", fmt.Sprintf("%s alocado=%d B", reasonStr(err), alloc))
	} else {
		vec(t, "img", "jpeg-bomba-sof-65500", "ESCAPOU", fmt.Sprintf("%s alocado=%d B ocr=%d", reasonStr(err), alloc, calls))
	}

	// 3. Progressive color JPEG of 48 MP (8000 x 6000): under twice the
	// pixel ceiling, but its coefficient buffers would need ~720 MB: refused
	// before decoding.
	calls, alloc, err = advImage(FormatJPEG, advJPEGPatched(true, 8000, 6000))
	if r, _ := ReasonOf(err); r == ReasonImageTooLarge && calls == 0 && alloc < 4<<20 {
		vec(t, "img", "jpeg-progressivo-48mp", "BLOQUEADO", fmt.Sprintf("%s alocado=%d B", reasonStr(err), alloc))
	} else {
		vec(t, "img", "jpeg-progressivo-48mp", "ESCAPOU", fmt.Sprintf("%s alocado=%d B ocr=%d", reasonStr(err), alloc, calls))
	}

	// 4. zlib that inflates far beyond the declared 100 x 100 gray image
	// (64 MiB of zeros in ~64 KB): the decoder must stop at what IHDR says.
	calls, alloc, err = advImage(FormatPNG, advPNG(100, 100, 8, 0, advZlib(64<<20)))
	switch {
	case alloc > 32<<20:
		vec(t, "img", "png-zlib-alem-do-declarado", "ESCAPOU", fmt.Sprintf("%s alocado=%d B", reasonStr(err), alloc))
	case err == nil && calls == 1:
		vec(t, "img", "png-zlib-alem-do-declarado", "BLOQUEADO", fmt.Sprintf("lido so o declarado (100x100), alocado=%d B", alloc))
	default:
		vec(t, "img", "png-zlib-alem-do-declarado", "BLOQUEADO", fmt.Sprintf("%s alocado=%d B ocr=%d", reasonStr(err), alloc, calls))
	}

	// 5. Extension and signature disagree: a JPEG named .png, a PNG named
	// .jpg, a PDF named .png. Never decoded, never sent to the OCR.
	for name, head := range map[string][]byte{
		"a.png": {0xFF, 0xD8, 0xFF, 0xE0}, "a.jpg": []byte("\x89PNG\r\n\x1a\n"), "b.png": []byte("%PDF-1.7\n"),
	} {
		_, err := Detect(name, head)
		if r, _ := ReasonOf(err); r == ReasonMismatch {
			vec(t, "img", "assinatura-errada-"+name, "BLOQUEADO", reasonStr(err))
		} else {
			vec(t, "img", "assinatura-errada-"+name, "ESCAPOU", reasonStr(err))
		}
	}

	// 6. No OCR configured: the image is refused out loud, never "empty".
	_, err = Extract(context.Background(), FormatPNG, advPNG(10, 10, 8, 0, advZlib(110)), Limits{})
	if r, _ := ReasonOf(err); r == ReasonOCRRequired {
		vec(t, "img", "sem-ocr", "BLOQUEADO", reasonStr(err))
	} else {
		vec(t, "img", "sem-ocr", "SILENCIOSO", reasonStr(err))
	}
}
