package platform

import (
	"context"
	"github.com/cloudwego/eino/components/tool"
)

// Supplemental phases have no authority to mutate primary recording state.
// Unknown tools are excluded by default, even when newly registered upstream.
func readOnlyTools(ctx context.Context, registered []tool.BaseTool) []tool.BaseTool {
	reads := []tool.BaseTool{}
	for _, entry := range registered {
		info, err := entry.Info(ctx)
		if err != nil {
			continue
		}
		allowed := isSourceTool(info.Name)
		switch info.Name {
		case "list_files", "list_directory", "list_repositories", "list_repository_directory", "get_risk_checklist":
			allowed = true
		}
		if allowed {
			reads = append(reads, entry)
		}
	}
	return reads
}
