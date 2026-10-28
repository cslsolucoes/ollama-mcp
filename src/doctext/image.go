package doctext

// Images (step 7.9, owner decision D-12): .png and .jpg/.jpeg, decoded with the
// standard library only, turned into 8-bit gray and sent to the OCR. Their
// only text is the OCR's; without an OCR they are refused.
//
// The header is read first (DecodeConfig) and the pixel count and the memory
// the decoder would need are checked BEFORE decoding: a few bytes can declare
// a 100,000 x 100,000 image (a decompression bomb). An image above the OCR
// pixel ceiling but still safe to decode is reduced after decoding; above
// that, it is refused.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
)

// imageDecodeBudget caps the memory the decoder may allocate for one image
// (estimated from the header). imageDecodeFactor: an image may declare up to
// this many times the OCR pixel ceiling and still be decoded and reduced.
const (
	imageDecodeBudget = 256 << 20
	imageDecodeFactor = 2
)

// extractImage decodes the image, reduces it to the pixel ceiling and
// recognizes it. The text is "[imagem — OCR conf=93]" plus what was read.
func extractImage(ctx context.Context, f Format, data []byte, lim Limits) (res *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, refuse(ReasonMalformed)
		}
	}()
	if lim.OCR == nil {
		return nil, refuse(ReasonOCRRequired)
	}
	o := *lim.OCR
	img, scale, err := decodeGray(f, data, o.maxPixels())
	if err != nil {
		return nil, err
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if isBlankImage(img) {
		return nil, refuse(ReasonOCRNoText)
	}
	// Images carry no reliable resolution: assume the configured one (a scan)
	// and lower it by the reduction applied.
	dpi := int(math.Round(float64(o.DPI) * scale))
	if dpi < ocrMinImageDPI {
		dpi = ocrMinImageDPI
	}
	octx, cancel := ocrBudget(ctx)
	defer cancel()
	p := recognize(octx, o.Engine, img, dpi)
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	switch p.status {
	case ocrUnavailable:
		return nil, refuse(ReasonOCRUnavailable)
	case ocrTimedOut:
		return nil, refuse(ReasonTimeout)
	case ocrOK:
		if countNonBlank(p.text) > 0 {
			break
		}
		fallthrough
	default:
		return nil, refuse(ReasonOCRNoText)
	}
	if n := len([]rune(p.text)); n > lim.MaxTextRunes {
		return nil, refuse(ReasonTooLong)
	}
	return &Result{
		Text: ocrMarker("imagem", p) + "\n" + p.text,
		OCR:  OCRStats{Used: true, Pages: 1, ConfMean: p.conf},
	}, nil
}

// decodeGray checks the header, decodes the image and returns it in 8-bit
// gray, reduced (area average) to at most maxPixels and to the dimension
// ceiling, with the scale applied (1 when not reduced).
func decodeGray(f Format, data []byte, maxPixels float64) (*image.Gray, float64, error) {
	var (
		cfg image.Config
		err error
	)
	switch f {
	case FormatPNG:
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	case FormatJPEG:
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	default:
		return nil, 0, refuse(ReasonMalformed)
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, 0, refuse(ReasonMalformed)
	}
	pixels := float64(cfg.Width) * float64(cfg.Height)
	if pixels > maxPixels*imageDecodeFactor || pixels*decodeBytesPerPixel(f, cfg, data) > imageDecodeBudget {
		return nil, 0, refuse(ReasonImageTooLarge)
	}
	var img image.Image
	switch f {
	case FormatPNG:
		img, err = png.Decode(bytes.NewReader(data))
	default:
		img, err = jpeg.Decode(bytes.NewReader(data))
	}
	if err != nil || img == nil {
		return nil, 0, refuse(ReasonMalformed)
	}
	b := img.Bounds()
	if b.Dx() != cfg.Width || b.Dy() != cfg.Height {
		return nil, 0, refuse(ReasonMalformed)
	}
	scale := math.Min(1, math.Sqrt(maxPixels/pixels))
	scale = math.Min(scale, ocrMaxDimension/float64(max(cfg.Width, cfg.Height)))
	return grayFit(img, scale), scale, nil
}

// decodeBytesPerPixel estimates what the standard decoder allocates per
// pixel, from the header: the destination image, plus the coefficient
// buffers of a progressive JPEG and the pass images of an interlaced PNG.
func decodeBytesPerPixel(f Format, cfg image.Config, data []byte) float64 {
	var bpp, comps float64
	switch cfg.ColorModel {
	case color.GrayModel:
		bpp, comps = 1, 1
	case color.Gray16Model:
		bpp, comps = 2, 1
	case color.YCbCrModel:
		bpp, comps = 3, 3 // 4:4:4 is the worst case
	case color.CMYKModel, color.RGBAModel, color.NRGBAModel:
		bpp, comps = 4, 4
	default:
		if _, ok := cfg.ColorModel.(color.Palette); ok {
			bpp, comps = 1, 1
		} else {
			bpp, comps = 8, 4
		}
	}
	switch f {
	case FormatPNG:
		if len(data) > 28 && data[28] == 1 { // IHDR interlace method: Adam7
			bpp *= 2
		}
	case FormatJPEG:
		if jpegProgressive(data) {
			bpp += 4 * comps // one int32 per coefficient, per component
		}
	}
	return bpp
}

// jpegProgressive reports whether the first frame header of a JPEG is
// progressive (SOF2, SOF6, SOF10, SOF14). It walks the marker segments up to
// the first SOF or SOS, with bounds checks only.
func jpegProgressive(data []byte) bool {
	i := 2 // after SOI
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return false
		}
		m := data[i+1]
		if m == 0xFF { // fill byte
			i++
			continue
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD8) { // markers without a length
			i += 2
			continue
		}
		if m == 0xDA || m == 0xD9 { // SOS or EOI before any SOF
			return false
		}
		if m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC {
			return m == 0xC2 || m == 0xC6 || m == 0xCA || m == 0xCE
		}
		n := int(data[i+2])<<8 | int(data[i+3])
		if n < 2 {
			return false
		}
		i += 2 + n
	}
	return false
}

// grayFit converts img to 8-bit gray over a white background (transparent
// pixels become white, not black) and, when scale < 1, reduces it by
// averaging the source pixels of each destination pixel. It works one source
// row at a time: no full-size gray copy is made.
func grayFit(img image.Image, scale float64) *image.Gray {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	tw, th := sw, sh
	if scale < 1 {
		tw = max(1, int(float64(sw)*scale))
		th = max(1, int(float64(sh)*scale))
	}
	out := image.NewGray(image.Rect(0, 0, tw, th))
	row := make([]byte, sw)
	if tw == sw && th == sh {
		for y := 0; y < sh; y++ {
			lumaRow(img, b.Min.Y+y, row)
			copy(out.Pix[y*out.Stride:], row)
		}
		return out
	}
	// Column map: source x goes into destination column x*tw/sw.
	col := make([]int, sw)
	cnt := make([]uint32, tw)
	for x := 0; x < sw; x++ {
		col[x] = x * tw / sw
		cnt[col[x]]++
	}
	acc := make([]uint32, tw)
	sy := 0
	for ty := 0; ty < th; ty++ {
		end := (ty + 1) * sh / th
		if end <= sy {
			end = sy + 1
		}
		clear(acc)
		rows := uint32(0)
		for ; sy < end && sy < sh; sy++ {
			lumaRow(img, b.Min.Y+sy, row)
			for x, v := range row {
				acc[col[x]] += uint32(v)
			}
			rows++
		}
		if rows == 0 {
			rows = 1
		}
		dst := out.Pix[ty*out.Stride : ty*out.Stride+tw]
		for tx := range dst {
			n := cnt[tx] * rows
			if n == 0 {
				dst[tx] = 255
				continue
			}
			dst[tx] = uint8((acc[tx] + n/2) / n)
		}
	}
	return out
}

// lumaRow writes the gray value of every pixel of source row y into row,
// with fast paths for the types the standard decoders return.
func lumaRow(img image.Image, y int, row []byte) {
	b := img.Bounds()
	switch m := img.(type) {
	case *image.Gray:
		off := m.PixOffset(b.Min.X, y)
		copy(row, m.Pix[off:off+len(row)])
	case *image.YCbCr:
		off := m.YOffset(b.Min.X, y)
		copy(row, m.Y[off:off+len(row)])
	case *image.NRGBA:
		off := m.PixOffset(b.Min.X, y)
		for x := range row {
			p := m.Pix[off+4*x : off+4*x+4]
			g := luma8(uint32(p[0]), uint32(p[1]), uint32(p[2]))
			a := uint32(p[3])
			row[x] = uint8((g*a + 255*(255-a) + 127) / 255)
		}
	case *image.RGBA: // premultiplied: add the white the alpha lets through
		off := m.PixOffset(b.Min.X, y)
		for x := range row {
			p := m.Pix[off+4*x : off+4*x+4]
			v := luma8(uint32(p[0]), uint32(p[1]), uint32(p[2])) + 255 - uint32(p[3])
			if v > 255 {
				v = 255
			}
			row[x] = uint8(v)
		}
	case *image.Paletted:
		pal := make([]uint8, len(m.Palette))
		for i, c := range m.Palette {
			pal[i] = grayOverWhite(c)
		}
		off := m.PixOffset(b.Min.X, y)
		for x := range row {
			if ci := int(m.Pix[off+x]); ci < len(pal) {
				row[x] = pal[ci]
			} else {
				row[x] = 255
			}
		}
	default:
		for x := range row {
			row[x] = grayOverWhite(img.At(b.Min.X+x, y))
		}
	}
}

// luma8 is the gray of 8-bit r, g, b with the weights of color.GrayModel.
func luma8(r, g, b uint32) uint32 {
	return (19595*r + 38470*g + 7471*b + 1<<15) >> 16
}

// grayOverWhite is the 8-bit gray of c composited over white.
func grayOverWhite(c color.Color) uint8 {
	r, g, b, a := c.RGBA() // 16-bit, alpha-premultiplied
	y := (19595*r + 38470*g + 7471*b + 1<<15) >> 16
	y += 0xffff - a
	if y > 0xffff {
		y = 0xffff
	}
	return uint8(y >> 8)
}
