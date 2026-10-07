package platform

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTicketRunnerStartStopAndRestart(t *testing.T) {
	for _, provider := range []string{"linear", "jira"} {
		t.Run(provider, func(t *testing.T) { testTicketRunnerLifecycle(t, provider) })
	}
}
func testTicketRunnerLifecycle(t *testing.T, provider string) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	team := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
	credentials := IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: team}
	if provider == "jira" {
		credentials = IntegrationCredentials{Endpoint: "https://fixture.atlassian.net", Username: "fixture@example.com", Token: "fixture", JiraProjectID: "10001", JiraIssueTypeID: "10002"}
	}
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: provider, Kind: provider, Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &credentials})
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	runner := &Runner{Store: s, Workers: 1, Repository: runRepo{}, ticketClient: &http.Client{Transport: retryTransportFunc(func(req *http.Request) (*http.Response, error) {
		posts.Add(1)
		if provider == "jira" {
			return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"10003","key":"AUDIT-12","self":"https://fixture.atlassian.net/rest/api/3/issue/10003"}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"issueCreate":{"success":true,"issue":{"id":"` + team + `","url":"https://linear.app/example/issue/LIN-123/finding"}}}}`))}, nil
	})}}
	defer runner.Stop()
	create := func(mr int) int64 {
		t.Helper()
		head := strings.Repeat("b", 40)
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, HeadSHA: head, BaseSHA: strings.Repeat("a", 40)}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(`UPDATE platform_runs SET status='succeeded',result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ReserveFindingTicket(ctx, id, 1, integration.ID, integration.Revision, "f", head); err != nil {
			t.Fatal(err)
		}
		return id
	}
	waitCreated := func(id int64, want int32) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			link, err := s.FindingTicket(ctx, id, 1, integration.ID, "f")
			if err == nil && link.State == "created" {
				if posts.Load() != want {
					t.Fatal("duplicate creation", posts.Load())
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("ticket not consumed", id, posts.Load())
	}
	first := create(1)
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitCreated(first, 1)
	done := make(chan struct{})
	go func() { runner.Stop(); runner.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ticket loop prevented shutdown")
	}
	second := create(2)
	time.Sleep(2200 * time.Millisecond)
	link, err := s.FindingTicket(ctx, second, 1, integration.ID, "f")
	if err != nil || link.State != "pending" || posts.Load() != 1 {
		t.Fatal("stopped runner sent ticket", link, err, posts.Load())
	}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitCreated(second, 2)
	// A completed record remains terminal across a second worker lifetime.
	runner.Stop()
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2200 * time.Millisecond)
	if posts.Load() != 2 {
		t.Fatal("restart replayed ticket", posts.Load())
	}
}
