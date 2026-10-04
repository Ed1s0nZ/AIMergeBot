package platform

import (
	"context"
	"fmt"
)

type metadataArgs struct {
	Path string `json:"path"`
}

func (t *auditTools) changeMetadata(ctx context.Context, a metadataArgs) (toolOutput, error) {
	return t.invoke("get_change_metadata", a, func() (toolOutput, error) {
		metadata, ok := t.scope.Metadata[a.Path]
		if !ok || !metadata.valid() {
			return toolOutput{}, fmt.Errorf("verified change metadata is unavailable or outside the included scope")
		}
		text := metadata.canonical()
		if len(text) > 16000 {
			return toolOutput{}, fmt.Errorf("metadata record exceeds tool output budget")
		}
		return toolOutput{Text: text, Metadata: &metadata}, nil
	})
}
