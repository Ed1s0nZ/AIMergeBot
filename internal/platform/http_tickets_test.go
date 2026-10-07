package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestTicketHTTPAuthenticatedReservationAndReadPermissions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser(ctx, "viewer", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, HeadSHA: head, BaseSHA: strings.Repeat("a", 40)}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "private-fixture", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}})
	if err != nil {
		t.Fatal(err)
	}
	_, adminToken, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	_, viewerToken, err := s.Login(ctx, "viewer", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	path := fmt.Sprintf("/api/v1/runs/%d/findings/f/tickets", run)
	body := fmt.Sprintf(`{"integration_id":%d,"expected_revision":%d,"head_sha":"%s"}`, integration.ID, integration.Revision, head)
	request := func(method, path, token, body, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := request("POST", path, "", body, ""); res.Code != 401 {
		t.Fatal("unauthenticated reservation", res.Code)
	}
	if res := request("POST", path, viewerToken, body, ""); res.Code != 403 {
		t.Fatal("viewer wrote", res.Code)
	}
	if res := request("POST", path, adminToken, body, "https://attacker.example"); res.Code != 403 {
		t.Fatal("cross-origin write", res.Code)
	}
	first := request("POST", path, adminToken, body, "")
	if first.Code != 202 {
		t.Fatal(first.Code, first.Body.String())
	}
	var reply struct {
		Ticket   TicketLink
		Reserved bool
	}
	if err := json.Unmarshal(first.Body.Bytes(), &reply); err != nil || !reply.Reserved || reply.Ticket.State != "pending" {
		t.Fatal(reply, err)
	}
	if strings.Contains(first.Body.String(), "idempotency_key") || strings.Contains(first.Body.String(), "private-fixture") {
		t.Fatal("internal values exposed")
	}
	if _, err := s.DB.Exec(`UPDATE platform_ticket_links SET state='unknown' WHERE id=?`, reply.Ticket.ID); err != nil {
		t.Fatal(err)
	}
	repeated := request("POST", path, adminToken, body, "")
	if repeated.Code != 200 {
		t.Fatal(repeated.Code)
	}
	if err := json.Unmarshal(repeated.Body.Bytes(), &reply); err != nil || reply.Reserved || reply.Ticket.State != "unknown" {
		t.Fatal("unknown replay reset", reply, err)
	}
	readPath := fmt.Sprintf("%s/%d", path, integration.ID)
	if res := request("GET", readPath, viewerToken, "", ""); res.Code != 200 {
		t.Fatal("viewer read denied", res.Code)
	}
	if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE user_id=?`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	if res := request("GET", readPath, viewerToken, "", ""); res.Code != 404 {
		t.Fatal("revoked viewer read", res.Code)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_ticket_links`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate reservation", count, err)
	}
}
