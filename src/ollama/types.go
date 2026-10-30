package ollama

// ModelOptions is the allow-list of sampling parameters (OLL-009). There is
// deliberately no num_ctx, keep_alive or format: the server fixes context and
// residency. Nil fields are omitted.
type ModelOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Seed        *int     `json:"seed,omitempty"`
}

// Usage holds the counters Ollama reports on the final chunk (OLL-006).
// Durations are nanoseconds, as sent by the server.
type Usage struct {
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
	// QueueMS is the wait in the server-side queue (header X-Queue-Ms set by the
	// gateway); 0 when there is no gateway. Not part of the Ollama JSON.
	QueueMS int64 `json:"-"`
	// Backend is "fallback" when the call was served by the fallback server
	// (--ollama-url-fallback) and "" for the primary. Not part of the Ollama JSON.
	Backend string `json:"-"`
}

// GenerateRequest is the input of Generate. Model empty means the client default.
type GenerateRequest struct {
	Model   string
	Prompt  string
	System  string
	Options *ModelOptions
	// Think asks the model to reason before answering (plan 8.8b); false by default.
	Think bool
}

// GenerateResponse is the aggregated result. Response is the concatenation of
// every streamed chunk. The reasoning ("thinking") field is not decoded at all
// (OLL-010) and the token "context" array is not decoded either (it is large
// and useless to the MCP client).
type GenerateResponse struct {
	Model      string `json:"model"`
	CreatedAt  string `json:"created_at"`
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`
	Error      string `json:"error"`
	Usage
}

// ChatMessage has no thinking field on purpose (OLL-010).
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the input of Chat. The history is sent by the caller; the
// server keeps no state. Model empty means the client default.
type ChatRequest struct {
	Model    string
	Messages []ChatMessage
	Options  *ModelOptions
	// Think asks the model to reason before answering (plan 8.8b); false by default.
	Think bool
}

// ChatResponse is the aggregated result; Message.Content is the concatenation
// of every streamed chunk.
type ChatResponse struct {
	Model      string      `json:"model"`
	CreatedAt  string      `json:"created_at"`
	Message    ChatMessage `json:"message"`
	Done       bool        `json:"done"`
	DoneReason string      `json:"done_reason"`
	Error      string      `json:"error"`
	Usage
}

// EmbedRequest is the input of Embed. Model empty means the client EmbedModel.
type EmbedRequest struct {
	Model string
	Input []string
}

// EmbedResponse carries the counters /api/embed reports.
type EmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	TotalDuration   int64       `json:"total_duration"`
	LoadDuration    int64       `json:"load_duration"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	Error           string      `json:"error"`
	QueueMS         int64       `json:"-"` // see Usage.QueueMS
	Backend         string      `json:"-"` // see Usage.Backend
}

// ModelDetails is the "details" object shared by tags, ps and show.
type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// TagModel is one entry of /api/tags.
type TagModel struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt string       `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

// TagsResponse is the body of /api/tags.
type TagsResponse struct {
	Models []TagModel `json:"models"`
}

// RunningModel is one entry of /api/ps. ContextLength is whatever the server
// reports; nothing in this package assumes a context size (PERF-006).
type RunningModel struct {
	Name          string       `json:"name"`
	Model         string       `json:"model"`
	Size          int64        `json:"size"`
	SizeVRAM      int64        `json:"size_vram"`
	Digest        string       `json:"digest"`
	ExpiresAt     string       `json:"expires_at"`
	ContextLength int          `json:"context_length"`
	Details       ModelDetails `json:"details"`
}

// PSResponse is the body of /api/ps.
type PSResponse struct {
	Models []RunningModel `json:"models"`
}

// ShowResponse leaves out modelfile, template and license (large, not needed).
type ShowResponse struct {
	Parameters   string         `json:"parameters"`
	Details      ModelDetails   `json:"details"`
	ModelInfo    map[string]any `json:"model_info"`
	Capabilities []string       `json:"capabilities"`
	ModifiedAt   string         `json:"modified_at"`
}
