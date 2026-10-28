package ocr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"strings"
	"testing"
)

func requestHeader(w, h, stride, dpi uint32) []byte {
	b := make([]byte, requestHeaderLen)
	copy(b, requestMagic[:])
	binary.LittleEndian.PutUint16(b[4:], protocolVersion)
	binary.LittleEndian.PutUint32(b[8:], w)
	binary.LittleEndian.PutUint32(b[12:], h)
	binary.LittleEndian.PutUint32(b[16:], stride)
	binary.LittleEndian.PutUint32(b[20:], dpi)
	return b
}

func TestRequestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		img  *image.Gray
	}{
		{"plain", testImage(37, 11)},
		{"one pixel", testImage(1, 1)},
		{"padded stride", &image.Gray{Pix: make([]byte, 40*5), Stride: 40, Rect: image.Rect(0, 0, 33, 5)}},
		// A sub-image keeps the parent's stride (200 > 50+64): compacted.
		{"sub-image", testImage(200, 30).SubImage(image.Rect(10, 5, 60, 25)).(*image.Gray)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeRequest(&buf, c.img, 300); err != nil {
				t.Fatal(err)
			}
			req, err := readRequest(&buf, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if buf.Len() != 0 {
				t.Fatalf("%d bytes left after the frame", buf.Len())
			}
			got := req.image()
			if got.Rect.Dx() != c.img.Rect.Dx() || got.Rect.Dy() != c.img.Rect.Dy() || req.dpi != 300 {
				t.Fatalf("header mismatch: %v dpi=%d", got.Rect, req.dpi)
			}
			if got.Stride < got.Rect.Dx() || got.Stride > got.Rect.Dx()+MaxStridePad {
				t.Fatalf("stride %d for width %d", got.Stride, got.Rect.Dx())
			}
			for y := 0; y < got.Rect.Dy(); y++ {
				for x := 0; x < got.Rect.Dx(); x++ {
					want := c.img.GrayAt(c.img.Rect.Min.X+x, c.img.Rect.Min.Y+y).Y
					if g := got.GrayAt(x, y).Y; g != want {
						t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, g, want)
					}
				}
			}
		})
	}
}

func TestReadRequestLimits(t *testing.T) {
	const maxPix = 1000 * 1000
	cases := []struct {
		name string
		data []byte
	}{
		{"short header", requestHeader(10, 10, 10, 300)[:10]},
		{"bad magic", append([]byte("XXXX"), requestHeader(10, 10, 10, 300)[4:]...)},
		{"bad version", func() []byte { b := requestHeader(1, 1, 1, 300); b[4] = 9; return append(b, 0) }()},
		{"reserved", func() []byte { b := requestHeader(1, 1, 1, 300); b[6] = 1; return append(b, 0) }()},
		{"zero width", requestHeader(0, 10, 10, 300)},
		{"zero height", requestHeader(10, 0, 10, 300)},
		{"huge width", requestHeader(1<<31, 1, 1<<31, 300)},
		{"over dimension", requestHeader(MaxDimension+1, 1, MaxDimension+1, 300)},
		{"stride below width", requestHeader(100, 10, 99, 300)},
		{"stride far above width", requestHeader(100, 10, 100+MaxStridePad+1, 300)},
		{"stride overflow", requestHeader(1, 2, 0xFFFFFFFF, 300)},
		{"megapixels", requestHeader(2000, 1000, 2000, 300)},
		{"dpi", requestHeader(10, 10, 10, MaxDPI+1)},
		{"truncated pixels", append(requestHeader(10, 10, 10, 300), make([]byte, 99)...)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := readRequest(bytes.NewReader(c.data), maxPix)
			if !errors.Is(err, errFrame) {
				t.Fatalf("err = %v, want errFrame", err)
			}
		})
	}
	if _, err := readRequest(bytes.NewReader(nil), maxPix); err != io.EOF {
		t.Fatalf("empty stream: err = %v, want io.EOF", err)
	}
	// Exactly at the limits: accepted.
	full := append(requestHeader(1000, 1000, 1000+MaxStridePad, 0), make([]byte, (1000+MaxStridePad)*999+1000)...)
	if _, err := readRequest(bytes.NewReader(full), maxPix); err != nil {
		t.Fatalf("at the limits: %v", err)
	}
}

func TestWriteRequestRefusesBadImages(t *testing.T) {
	cases := map[string]*image.Gray{
		"nil":        nil,
		"empty":      image.NewGray(image.Rect(0, 0, 0, 0)),
		"short pix":  {Pix: make([]byte, 10), Stride: 10, Rect: image.Rect(0, 0, 10, 10)},
		"bad stride": {Pix: make([]byte, 100), Stride: 5, Rect: image.Rect(0, 0, 10, 10)},
		"too wide":   {Pix: make([]byte, MaxDimension+1), Stride: MaxDimension + 1, Rect: image.Rect(0, 0, MaxDimension+1, 1)},
	}
	for name, img := range cases {
		if err := writeRequest(io.Discard, img, 300); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := writeRequest(io.Discard, testImage(2, 2), MaxDPI+1); err == nil {
		t.Error("dpi out of range accepted")
	}
}

func TestResponseRoundTripAndLimits(t *testing.T) {
	in := []frameResponse{
		{kind: kindHello, status: statusOK, text: "5.3.3"},
		{kind: kindResult, status: statusOK, conf: 93, rotated: 270, text: "Cláusula 1ª — ação"},
		{kind: kindResult, status: statusEngine, text: "falha"},
		{kind: kindResult, status: statusOK, text: strings.Repeat("a", MaxTextBytes)},
	}
	for _, r := range in {
		var buf bytes.Buffer
		if err := writeResponse(&buf, r); err != nil {
			t.Fatal(err)
		}
		got, err := readResponse(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if got != r {
			t.Fatalf("round trip changed the response (kind %d status %d)", r.kind, r.status)
		}
	}
	bad := []frameResponse{
		{kind: 3},
		{kind: kindResult, status: maxStatusNumber + 1},
		{kind: kindResult, conf: 101},
		{kind: kindResult, rotated: 45},
		{kind: kindResult, text: strings.Repeat("a", MaxTextBytes+1)},
		{kind: kindResult, status: statusEngine, text: strings.Repeat("a", maxMessageBytes+1)},
	}
	for i, r := range bad {
		if err := writeResponse(io.Discard, r); !errors.Is(err, errFrame) {
			t.Errorf("bad response %d accepted: %v", i, err)
		}
	}
	// Reader side: a header announcing more than the limit is refused
	// before the text is allocated; truncated text is an error.
	hdr := make([]byte, responseHeaderLen)
	copy(hdr, responseMagic[:])
	binary.LittleEndian.PutUint16(hdr[4:], protocolVersion)
	hdr[6] = kindResult
	binary.LittleEndian.PutUint32(hdr[12:], MaxTextBytes+1)
	if _, err := readResponse(bytes.NewReader(hdr)); !errors.Is(err, errFrame) {
		t.Fatalf("oversized text: %v", err)
	}
	binary.LittleEndian.PutUint32(hdr[12:], 10)
	if _, err := readResponse(bytes.NewReader(append(hdr, "abc"...))); !errors.Is(err, errFrame) {
		t.Fatalf("truncated text: %v", err)
	}
	if _, err := readResponse(bytes.NewReader(hdr[:7])); !errors.Is(err, errFrame) {
		t.Fatalf("truncated header: %v", err)
	}
	if _, err := readResponse(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("empty: %v", err)
	}
}

func TestShortMessage(t *testing.T) {
	s := strings.Repeat("ã", maxMessageBytes) // 2 bytes per rune
	got := shortMessage(s)
	if len(got) > maxMessageBytes || !strings.HasPrefix(s, got) || strings.ContainsRune(got, '�') {
		t.Fatalf("len %d", len(got))
	}
}

// FuzzOCRFrame feeds arbitrary bytes to both decoders: never a panic, never
// an allocation beyond the header limits, and every accepted frame survives
// a re-encode unchanged.
func FuzzOCRFrame(f *testing.F) {
	var req bytes.Buffer
	_ = writeRequest(&req, testImage(7, 3), 300)
	f.Add(req.Bytes())
	f.Add(req.Bytes()[:requestHeaderLen+5])
	f.Add(requestHeader(100, 10, 99, 300))
	f.Add(requestHeader(1<<31, 1<<31, 1<<31, 300))
	var resp bytes.Buffer
	_ = writeResponse(&resp, frameResponse{kind: kindResult, conf: 80, rotated: 90, text: "ação"})
	f.Add(resp.Bytes())
	f.Add(resp.Bytes()[:responseHeaderLen+1])
	f.Fuzz(func(t *testing.T, data []byte) {
		if r, err := readRequest(bytes.NewReader(data), 1<<20); err == nil {
			var out bytes.Buffer
			if err := writeRequest(&out, r.image(), r.dpi); err != nil {
				t.Fatalf("re-encode request: %v", err)
			}
			r2, err := readRequest(&out, 1<<20)
			if err != nil {
				t.Fatalf("decode re-encoded request: %v", err)
			}
			if r2.width != r.width || r2.height != r.height || r2.dpi != r.dpi || imageSum(r2.image()) != imageSum(r.image()) {
				t.Fatal("request changed on re-encode")
			}
		}
		if r, err := readResponse(bytes.NewReader(data)); err == nil {
			var out bytes.Buffer
			if err := writeResponse(&out, r); err != nil {
				t.Fatalf("re-encode response: %v", err)
			}
			r2, err := readResponse(&out)
			if err != nil || r2 != r {
				t.Fatalf("response changed on re-encode: %v", err)
			}
		}
	})
}
