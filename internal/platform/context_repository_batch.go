package platform

import (
	"context"
	"fmt"
	"strings"
)

type contextBatchArgs struct {
	RepositoryID int        `json:"repository_id"`
	Files        []readArgs `json:"files"`
}

func (t *auditTools) contextBatch(ctx context.Context, a contextBatchArgs) (toolOutput, error) {
	return t.invokeContext(ctx, "read_repository_files", a.RepositoryID, a, func(reader *auditTools) (toolOutput, error) {
		if len(a.Files) < 1 || len(a.Files) > 8 {
			return toolOutput{}, fmt.Errorf("batch must contain 1–8 files")
		}
		for _, file := range a.Files {
			if file.Base {
				return toolOutput{}, fmt.Errorf("context files use the single fixed snapshot; base must be false")
			}
		}
		var text strings.Builder
		out := toolOutput{}
		for _, file := range a.Files {
			if err := ctx.Err(); err != nil {
				return toolOutput{}, err
			}
			part, err := reader.file(ctx, file)
			if err != nil || part.Error != "" {
				return toolOutput{}, fmt.Errorf("batch read unavailable; check paths and ranges")
			}
			block := fmt.Sprintf("File %s (base=false)\n%s", file.Path, part.Text)
			if text.Len()+len(block) > 16000 {
				return toolOutput{}, fmt.Errorf("batch output budget exceeded; reduce batch or ranges")
			}
			text.WriteString(block)
			out.More = out.More || part.More
			out.Remaining = out.Remaining || part.Remaining
		}
		out.Text = text.String()
		return out, nil
	})
}
