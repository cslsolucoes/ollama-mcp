package doctext

import "context"

// TEMP stub: replaced by pdf.go (7.5)
func extractPDF(ctx context.Context, data []byte, lim Limits) (*Result, error) {
	return nil, refuse(ReasonMalformed)
}
