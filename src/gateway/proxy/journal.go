package proxy

// The job log hooks of the proxy (Fase 10.L): the full prompt goes to the
// journal when the line accepts the request, the answer text is teed out of the
// response body while it streams, and the journal is told when the slot is
// granted. The proxy itself keeps no text: it only hands it over.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/hmackey"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

// Journal receives the life of a job. Every method is called in the goroutine
// of the request and may block for a synchronous disk write.
type Journal interface {
	// Enqueued is called once the line accepted the request (status na_fila),
	// before it waits for a slot. prompt is the full text sent to the model.
	Enqueued(rec queue.Record, prompt string)
	// Started is called right after the slot is granted, before the request is
	// forwarded to the Ollama.
	Started(id string, at time.Time, backend string)
	// Response returns the sink of the answer of job id; nil is allowed.
	Response(id string) ResponseSink
}

// ResponseSink collects the answer of one job.
type ResponseSink interface {
	// Write adds generated text, in order.
	Write(text string)
	// Finish flushes durably. note is a short text appended last (the embeddings
	// summary). responded is true when some text or a note was produced.
	Finish(note string) (responded bool, bytes int64)
}

// maxEmbedBody bounds what is buffered to count the vectors of an embedding
// answer; the vectors themselves are never kept.
const maxEmbedBody = 16 << 20

// maxLine bounds one buffered line of a streamed answer.
const maxLine = 4 << 20

// extractPrompt builds the text shown in the job log from the body that goes to
// the model: system, prompt, messages and input, each under a label. Values
// that are not plain strings are kept as the JSON they came as.
func extractPrompt(kind routeKind, body []byte, key *hmackey.Key) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return string(body)
	}
	get := func(name string) (json.RawMessage, bool) {
		for k, v := range m {
			if strings.EqualFold(k, name) {
				return v, true
			}
		}
		return nil, false
	}
	var b strings.Builder
	b.Grow(len(body)) // the text is about the body's size: no doubling copies (revisão 10.L, M1)
	section := func(label string, raw json.RawMessage) {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[" + label + "]\n")
		b.WriteString(textOf(raw))
	}
	if raw, ok := get("system"); ok {
		section("system", raw)
	}
	if raw, ok := get("prompt"); ok {
		section("prompt", raw)
	}
	if raw, ok := get("suffix"); ok {
		section("suffix", raw)
	}
	if raw, ok := get("messages"); ok {
		var msgs []map[string]json.RawMessage
		if json.Unmarshal(raw, &msgs) == nil {
			for _, mm := range msgs {
				role := "?"
				if r, ok := mm["role"]; ok {
					role = textOf(r)
				}
				c, ok := mm["content"]
				if !ok {
					c = json.RawMessage(`""`)
				}
				section(role, redactParts(c, key))
				if imgs, ok := mm["images"]; ok {
					var l []json.RawMessage
					if json.Unmarshal(imgs, &l) == nil && len(l) > 0 {
						fmt.Fprintf(&b, "\n(%d imagens omitidas)", len(l))
					}
				}
			}
		} else {
			section("messages", raw)
		}
	}
	if raw, ok := get("input"); ok {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			for i, it := range list {
				section(fmt.Sprintf("input %d", i+1), it)
			}
		} else {
			section("input", raw)
		}
	}
	return b.String()
}

// redactParts keeps image data out of the flat text: the content of a message
// that is an array of parts (/v1) goes through the same scan as the prompt JSON,
// so an image_url part shows as {"imagem_hmac":"<hex>"} and never as the data
// URI. It reuses the scan by wrapping the value as a message body. If the scan
// fails, the parts are not shown at all (fail closed).
func redactParts(raw json.RawMessage, key *hmackey.Key) json.RawMessage {
	t := bytes.TrimLeft(raw, " \t\r\n")
	if len(t) == 0 || t[0] != '[' {
		return raw
	}
	const pre, post = `{"messages":[{"content":`, `}]}`
	wrapped := make([]byte, 0, len(pre)+len(raw)+len(post))
	wrapped = append(append(append(wrapped, pre...), raw...), post...)
	out, has, ok := spliceImages(wrapped, key)
	if !ok {
		return json.RawMessage(`"[partes da mensagem omitidas: conteúdo ilegível]"`)
	}
	if !has {
		return raw
	}
	return json.RawMessage(out[len(pre) : len(out)-len(post)])
}

// textOf decodes a JSON string; anything else is kept as its JSON text.
func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// respTee reads the answer bytes as the proxy copies them to the client and
// hands the generated text to the sink. It must never disturb the stream: a
// panic is swallowed.
type respTee struct {
	kind  routeKind
	sink  ResponseSink
	buf   []byte
	mode  string // "", "raciocinio" or "resposta": which kind of text came last
	embed []byte // embeddings answer (bounded), parsed at the end
	over  bool   // embed body larger than maxEmbedBody
}

func newRespTee(kind routeKind, sink ResponseSink) *respTee {
	return &respTee{kind: kind, sink: sink}
}

type respLine struct {
	Response string `json:"response"`
	Thinking string `json:"thinking"`
	Message  *struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Choices []struct {
		Text  string `json:"text"`
		Delta *struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"delta"`
		Message *struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
}

// Write feeds raw response bytes.
func (t *respTee) Write(p []byte) {
	defer func() { _ = recover() }()
	if t.kind == kindEmbed {
		if !t.over {
			if len(t.embed)+len(p) > maxEmbedBody {
				t.over, t.embed = true, nil
			} else {
				t.embed = append(t.embed, p...)
			}
		}
		return
	}
	t.buf = append(t.buf, p...)
	for {
		i := bytes.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		t.line(t.buf[:i])
		t.buf = append(t.buf[:0], t.buf[i+1:]...)
	}
	if len(t.buf) > maxLine { // a line this long is not text we can keep
		t.buf = t.buf[:0]
	}
}

func (t *respTee) line(l []byte) {
	l = bytes.TrimSpace(l)
	l = bytes.TrimSpace(bytes.TrimPrefix(l, []byte("data:")))
	if len(l) == 0 || string(l) == "[DONE]" || l[0] != '{' {
		return
	}
	var r respLine
	if json.Unmarshal(l, &r) != nil {
		return
	}
	think, text := r.Thinking, r.Response
	if r.Message != nil {
		think, text = think+r.Message.Thinking, text+r.Message.Content
	}
	for _, c := range r.Choices {
		text += c.Text
		if c.Delta != nil {
			text, think = text+c.Delta.Content, think+c.Delta.Reasoning
		}
		if c.Message != nil {
			text, think = text+c.Message.Content, think+c.Message.Reasoning
		}
	}
	t.emit("raciocinio", think)
	t.emit("resposta", text)
}

// emit writes a piece of text; when the model reasons, the two kinds are
// labelled so the log tells them apart.
func (t *respTee) emit(mode, text string) {
	if text == "" {
		return
	}
	if mode == "raciocinio" && t.mode != "raciocinio" {
		t.sink.Write("[raciocínio] ")
	} else if mode == "resposta" && t.mode == "raciocinio" {
		t.sink.Write("\n[resposta] ")
	}
	t.mode = mode
	t.sink.Write(text)
}

// End finishes the answer and returns the sink's verdict.
func (t *respTee) End() (responded bool, bytes int64) {
	var note string
	func() {
		defer func() { _ = recover() }()
		if len(t.buf) > 0 { // a last line without newline (non-stream JSON)
			t.line(t.buf)
			t.buf = nil
		}
		if t.kind == kindEmbed {
			note = embedNote(t.embed, t.over)
		}
	}()
	return t.sink.Finish(note)
}

// embedNote describes an embeddings answer without keeping a single number.
func embedNote(body []byte, over bool) string {
	if over {
		return "embeddings: resposta grande demais para contar (vetores não gravados)"
	}
	var r struct {
		Embedding  []json.RawMessage   `json:"embedding"`
		Embeddings [][]json.RawMessage `json:"embeddings"`
		Data       []struct {
			Embedding []json.RawMessage `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	n, dim := 0, 0
	switch {
	case len(r.Embeddings) > 0:
		n, dim = len(r.Embeddings), len(r.Embeddings[0])
	case len(r.Data) > 0:
		n, dim = len(r.Data), len(r.Data[0].Embedding)
	case len(r.Embedding) > 0:
		n, dim = 1, len(r.Embedding)
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("embeddings: %d vetor(es) de %d dimensões (vetores não gravados)", n, dim)
}
