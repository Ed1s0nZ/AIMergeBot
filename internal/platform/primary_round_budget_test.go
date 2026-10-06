package platform

import (
	"context"
	"errors"
	"fmt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"strings"
	"testing"
)

func TestPrimaryRoundGuidanceDoesNotAccumulateOrMutate(t *testing.T) {
	original := []*schema.Message{schema.SystemMessage("trusted"), schema.UserMessage("untrusted source")}
	messages := original
	for round := 1; round <= 4; round++ {
		messages = primaryDecisionMessages(messages, "trusted", round, 4)
		if strings.Count(messages[0].Content, "Server-owned decision budget") != 1 || !strings.Contains(messages[0].Content, fmt.Sprintf("decision %d of 4", round)) {
			t.Fatalf("unbounded or wrong guidance: %s", messages[0].Content)
		}
		if messages[1] != original[1] || original[0].Content != "trusted" {
			t.Fatal("source or original prompt mutated")
		}
		if (round == 4) != strings.Contains(messages[0].Content, "do not request more tools") {
			t.Fatal("wrong final boundary")
		}
	}
}
func TestAuditStopReasonUsesTypedCause(t *testing.T) {
	if auditStopReason(fmt.Errorf("wrapped: %w", compose.ErrExceedMaxSteps)) != "agent_step_budget" {
		t.Fatal("typed step cause lost")
	}
	if auditStopReason(errors.New(compose.ErrExceedMaxSteps.Error())) != "agent_failure" {
		t.Fatal("string guessed as typed cause")
	}
	if auditStopReason(context.DeadlineExceeded) != "deadline" || auditStopReason(nil) != "" {
		t.Fatal("incorrect cause")
	}
}

func TestResponseStopReasonBoundedAndTyped(t *testing.T) {
	if auditStopReason(fmt.Errorf("wrapped: %w", responseError("unknown_field"))) != "model_response_unknown_field" {
		t.Fatal("wrapped parse cause lost")
	}
	if auditStopReason(errors.New("invalid audit response: unknown_field")) != "agent_failure" {
		t.Fatal("untyped string accepted")
	}
	if auditStopReason(responseError("sensitive arbitrary provider content")) != "model_response_invalid_structure" {
		t.Fatal("unbounded code exposed")
	}
}
