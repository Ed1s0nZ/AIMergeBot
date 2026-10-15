package platform

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func repositoryActivationError(c *gin.Context, err error) bool {
	status, code := 0, ""
	switch {
	case errors.Is(err, errCredentialActivationPending):
		status, code = 503, "credential_activation_pending"
	case errors.Is(err, errCredentialActivationUnavailable):
		status, code = 503, "credential_activation_unavailable"
	case errors.Is(err, errCredentialActivationExhausted):
		status, code = 409, "activation_version_exhausted"
	case errors.Is(err, errReloadPermission):
		status, code = 403, "permission_changed"
	case errors.Is(err, ErrSettingsConflict):
		status, code = 409, "settings_changed"
	case errors.Is(err, errReloadConfiguration):
		status, code = 400, "invalid_configuration"
	default:
		return false
	}
	c.JSON(status, gin.H{"error": code, "code": code})
	return true
}

func (h *HTTP) reloadRepositoryActivation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	raw, err := io.ReadAll(c.Request.Body)
	invalid := func() {
		c.JSON(400, gin.H{"error": "invalid activation request", "code": "invalid_activation_request"})
	}
	if err != nil || !utf8.Valid(raw) || uniqueScopeJSON(raw) != nil {
		invalid()
		return
	}
	fields, err := scopeObject(raw, "expected_config_revision")
	if err != nil {
		invalid()
		return
	}
	var revision string
	if json.Unmarshal(fields["expected_config_revision"], &revision) != nil {
		invalid()
		return
	}
	expected, ok := decimalRevision(revision, true)
	if !ok {
		invalid()
		return
	}
	if h.Settings == nil {
		repositoryActivationError(c, errCredentialActivationUnavailable)
		return
	}
	if err = h.Settings.reloadRepositoryCredentials(c.Request.Context(), expected, currentUser(c).ID); err != nil {
		if !repositoryActivationError(c, err) {
			repositoryActivationError(c, errCredentialActivationUnavailable)
		}
		return
	}
	public := h.publicSettingsWithContext(c.Request.Context())
	c.JSON(200, gin.H{"settings": public, "restart_required": public["restart_required"]})
}
