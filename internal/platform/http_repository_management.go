package platform

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type repositoryManagementStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// Native execution and identity recovery must be complete before enabling writes.
// This is not a setting that credentials or an administrator can toggle.
func repositoryManagementAvailability() repositoryManagementStatus {
	return repositoryManagementStatus{Available: false, Reason: "native_audit_not_ready"}
}

// Keep these routes behind the existing session/admin/CSRF middleware so old
// clients receive an explicit refusal without changing project identity.
func rejectRepositoryManagementWrite(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "native repository audit and recovery are not ready; existing bindings are read-only",
		"code":  "repository_management_unavailable",
	})
}
