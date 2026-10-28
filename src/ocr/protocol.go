package ocr

// Worker protocol: binary frames over the worker's stdin (requests) and
// stdout (responses), little endian, one request answered by one response.
//
//	request  (24-byte header + pixels)
//	  0  [4]byte magic "MOCQ"
//	  4  uint16  version (1)
//	  6  uint16  reserved, 0
//	  8  uint32  width   1..MaxDimension
//	 12  uint32  height  1..MaxDimension
//	 16  uint32  stride  width..width+MaxStridePad
//	 20  uint32  dpi     0 (worker default) or MinDPI..MaxDPI
//	 24  pixels: stride*(height-1)+width bytes, 8-bit gray, row-major
//
//	response (16-byte header + text)
//	  0  [4]byte magic "MOCR"
//	  4  uint16  version (1)
//	  6  uint8   kind: 1 hello (sent once at start), 2 page result
//	  7  uint8   status (statusOK...)
//	  8  uint8   confidence 0..100
//	  9  uint8   reserved, 0
//	 10  uint16  rotation 0, 90, 180 or 270
//	 12  uint32  text length 0..MaxTextBytes
//	 16  text: UTF-8 (status OK: the page text or, in the hello, the engine
//	     version; other status: a short pt-BR message without document data)
//
// Every limit is checked on the header, before anything is allocated; a
// truncated frame, absurd dimensions or an inconsistent stride is an error,
// never a panic.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
)

const (
	protocolVersion   = 1
	requestHeaderLen  = 24
	responseHeaderLen = 16
	maxMessageBytes   = 512 // status messages (not page text)
)

var (
	requestMagic  = [4]byte{'M', 'O', 'C', 'Q'}
	responseMagic = [4]byte{'M', 'O', 'C', 'R'}
)

// Response kinds.
const (
	kindHello  = 1
	kindResult = 2
)

// Response status.
const (
	statusOK        = 0
	statusBadFrame  = 1 // the request was malformed; the worker exits
	statusEngine    = 2 // the engine failed on this page; the worker goes on
	statusInit      = 3 // hello: the engine could not start; the worker exits
	statusTooLarge  = 4 // the page text exceeds MaxTextBytes; the worker goes on
	statusBadImage  = 5 // the image is outside the limits; the worker goes on
	maxStatusNumber = statusBadImage
)

// errFrame is a protocol violation: the stream cannot be trusted any more.
var errFrame = errors.New("ocr: quadro do protocolo inválido")

func frameErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errFrame, fmt.Sprintf(format, a...))
}

// frameRequest is a decoded request.
type frameRequest struct {
	width, height, stride, dpi int
	pix                        []byte // len == stride*height (the last row padded)
}

func (r frameRequest) image() *image.Gray {
	return &image.Gray{Pix: r.pix, Stride: r.stride, Rect: image.Rect(0, 0, r.width, r.height)}
}

// frameResponse is a decoded response.
type frameResponse struct {
	kind, status int
	conf         int
	rotated      int
	text         string
}

// writeRequest encodes img. A stride wider than width+MaxStridePad (a
// sub-image of a larger image) is compacted to width.
func writeRequest(w io.Writer, img *image.Gray, dpi int) error {
	if err := checkImage(img, int64(MaxDimension)*MaxDimension); err != nil {
		return err
	}
	if dpi != 0 && (dpi < MinDPI || dpi > MaxDPI) {
		return ErrBadImage
	}
	width, height := img.Rect.Dx(), img.Rect.Dy()
	stride := img.Stride
	compact := stride-width > MaxStridePad
	if compact {
		stride = width
	}
	var hdr [requestHeaderLen]byte
	copy(hdr[0:4], requestMagic[:])
	binary.LittleEndian.PutUint16(hdr[4:], protocolVersion)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(width))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(height))
	binary.LittleEndian.PutUint32(hdr[16:], uint32(stride))
	binary.LittleEndian.PutUint32(hdr[20:], uint32(dpi))
	bw := bufio.NewWriterSize(w, 64<<10)
	if _, err := bw.Write(hdr[:]); err != nil {
		return err
	}
	off := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
	if compact {
		for y := 0; y < height; y++ {
			row := off + y*img.Stride
			if _, err := bw.Write(img.Pix[row : row+width]); err != nil {
				return err
			}
		}
	} else {
		n := stride*(height-1) + width
		if _, err := bw.Write(img.Pix[off : off+n]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// readRequest decodes one request. io.EOF means a clean end of stream (no
// byte of a new frame); any other error is a broken frame.
func readRequest(r io.Reader, maxPixels int64) (frameRequest, error) {
	var hdr [requestHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if err == io.EOF {
			return frameRequest{}, io.EOF
		}
		return frameRequest{}, frameErr("cabeçalho truncado")
	}
	if [4]byte(hdr[0:4]) != requestMagic {
		return frameRequest{}, frameErr("assinatura desconhecida")
	}
	if v := binary.LittleEndian.Uint16(hdr[4:]); v != protocolVersion {
		return frameRequest{}, frameErr("versão %d não suportada", v)
	}
	if binary.LittleEndian.Uint16(hdr[6:]) != 0 {
		return frameRequest{}, frameErr("campo reservado não nulo")
	}
	w := int64(binary.LittleEndian.Uint32(hdr[8:]))
	h := int64(binary.LittleEndian.Uint32(hdr[12:]))
	stride := int64(binary.LittleEndian.Uint32(hdr[16:]))
	dpi := int64(binary.LittleEndian.Uint32(hdr[20:]))
	if w < 1 || h < 1 || w > MaxDimension || h > MaxDimension {
		return frameRequest{}, frameErr("dimensões fora dos limites")
	}
	if stride < w || stride > w+MaxStridePad {
		return frameRequest{}, frameErr("stride inconsistente com a largura")
	}
	if w*h > maxPixels {
		return frameRequest{}, frameErr("imagem acima do teto de megapixels")
	}
	if dpi != 0 && (dpi < MinDPI || dpi > MaxDPI) {
		return frameRequest{}, frameErr("dpi fora do intervalo")
	}
	// Bounded: stride*h <= (MaxDimension+MaxStridePad)*MaxDimension and
	// w*h <= maxPixels.
	pix := make([]byte, stride*h)
	n := stride*(h-1) + w
	if _, err := io.ReadFull(r, pix[:n]); err != nil {
		return frameRequest{}, frameErr("pixels truncados")
	}
	return frameRequest{width: int(w), height: int(h), stride: int(stride), dpi: int(dpi), pix: pix}, nil
}

func validRotation(r int) bool { return r == 0 || r == 90 || r == 180 || r == 270 }

// writeResponse encodes resp after checking it against the same limits the
// reader enforces.
func writeResponse(w io.Writer, resp frameResponse) error {
	if resp.kind != kindHello && resp.kind != kindResult {
		return frameErr("tipo de resposta inválido")
	}
	if resp.status < 0 || resp.status > maxStatusNumber || resp.conf < 0 || resp.conf > 100 || !validRotation(resp.rotated) {
		return frameErr("campos da resposta fora dos limites")
	}
	if len(resp.text) > MaxTextBytes {
		return frameErr("texto acima do limite")
	}
	if resp.status != statusOK && len(resp.text) > maxMessageBytes {
		return frameErr("mensagem acima do limite")
	}
	var hdr [responseHeaderLen]byte
	copy(hdr[0:4], responseMagic[:])
	binary.LittleEndian.PutUint16(hdr[4:], protocolVersion)
	hdr[6] = byte(resp.kind)
	hdr[7] = byte(resp.status)
	hdr[8] = byte(resp.conf)
	binary.LittleEndian.PutUint16(hdr[10:], uint16(resp.rotated))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(len(resp.text)))
	buf := make([]byte, 0, responseHeaderLen+len(resp.text))
	buf = append(buf, hdr[:]...)
	buf = append(buf, resp.text...)
	_, err := w.Write(buf)
	return err
}

// readResponse decodes one response.
func readResponse(r io.Reader) (frameResponse, error) {
	var hdr [responseHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if err == io.EOF {
			return frameResponse{}, io.EOF
		}
		return frameResponse{}, frameErr("cabeçalho truncado")
	}
	if [4]byte(hdr[0:4]) != responseMagic {
		return frameResponse{}, frameErr("assinatura desconhecida")
	}
	if v := binary.LittleEndian.Uint16(hdr[4:]); v != protocolVersion {
		return frameResponse{}, frameErr("versão %d não suportada", v)
	}
	resp := frameResponse{
		kind:    int(hdr[6]),
		status:  int(hdr[7]),
		conf:    int(hdr[8]),
		rotated: int(binary.LittleEndian.Uint16(hdr[10:])),
	}
	if resp.kind != kindHello && resp.kind != kindResult {
		return frameResponse{}, frameErr("tipo de resposta inválido")
	}
	if resp.status > maxStatusNumber || resp.conf > 100 || hdr[9] != 0 || !validRotation(resp.rotated) {
		return frameResponse{}, frameErr("campos da resposta fora dos limites")
	}
	n := binary.LittleEndian.Uint32(hdr[12:])
	if n > MaxTextBytes {
		return frameResponse{}, frameErr("texto acima do limite")
	}
	if resp.status != statusOK && n > maxMessageBytes {
		return frameResponse{}, frameErr("mensagem acima do limite")
	}
	text := make([]byte, n)
	if _, err := io.ReadFull(r, text); err != nil {
		return frameResponse{}, frameErr("texto truncado")
	}
	resp.text = string(text)
	return resp, nil
}

// shortMessage bounds a status message to maxMessageBytes, on a rune
// boundary.
func shortMessage(s string) string {
	if len(s) <= maxMessageBytes {
		return s
	}
	cut := maxMessageBytes
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}
