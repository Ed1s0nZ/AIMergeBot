package platform

import (
	"context"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"testing"
)

func TestSupplementalToolsExcludeUnknownMutationByDefault(t *testing.T) {
	ctx := context.Background()
	tools := &auditTools{}
	registered, err := tools.register()
	if err != nil {
		t.Fatal(err)
	}
	future, err := utils.InferTool("future_record_mutation", "synthetic future tool", func(context.Context, struct{}) (string, error) { return "unused", nil })
	if err != nil {
		t.Fatal(err)
	}
	registered = append(registered, tool.BaseTool(future))
	names := map[string]bool{}
	for _, entry := range readOnlyTools(ctx, registered) {
		info, err := entry.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"future_record_mutation", "record_hypothesis", "update_investigation", "submit_finding", "resolve_recording_errors"} {
		if names[name] {
			t.Fatal("supplemental mutation exposed", name)
		}
	}
	for _, name := range []string{"read_file", "read_files", "get_diff", "get_risk_checklist", "list_files", "list_directory"} {
		if !names[name] {
			t.Fatal("read capability lost", name)
		}
	}
}
