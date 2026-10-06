package platform

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/compose"
)

// Stop reasons are derived from errors owned by the runtime, not model text.
func auditStopReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrRepositoryUnavailable):
		return "repository_unavailable"
	case errors.Is(err, compose.ErrExceedMaxSteps):
		return "agent_step_budget"
	case errors.Is(err, ErrContextCompression):
		return "context_compression"
	case errors.Is(err, ErrModelTokenBudget):
		return "model_token_budget"
	case errors.Is(err, ErrModelUsageUnknown):
		return "model_usage_unknown"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		var response *AuditResponseError
		if errors.As(err, &response) {
			switch response.Code {
			case "unknown_field", "json_syntax", "json_type", "json_decode", "trailing_data", "required_fields", "finding_limit", "output_budget":
				return "model_response_" + response.Code
			default:
				return "model_response_invalid_structure"
			}
		}
		return "agent_failure"
	}
}
