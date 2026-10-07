package platform

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTicketHTTPRejectsMissingIdentityAndOversizedBodies(t *testing.T) {
	router := gin.New()
	h := &HTTP{}
	router.POST("/runs/:id/findings/:finding_id/tickets", h.reserveFindingTicket)
	for _, body := range []string{`{}`, `{"integration_id":1,"head_sha":"bad","expected_revision":1}`, `{"integration_id":1,"head_sha":"` + strings.Repeat("a", 40) + `"}`, strings.Repeat("x", 8193)} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/runs/1/findings/f/tickets", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
}
