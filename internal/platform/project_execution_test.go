package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRevokingOperatorInterruptsRunningAuditAndPreservesCheckpoint(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	a := &blockingAuditor{started: make(chan struct{}, 1)}
	r := &Runner{Store: s, Repository: runRepo{}, Auditor: a, Workers: 1, Timeout: time.Minute}
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	id, _, err := r.Submit(ctx, 1, 1, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.started:
	case <-time.After(3 * time.Second):
		t.Fatal("audit did not start")
	}
	checkpoint := AuditResult{Summary: "investigation checkpoint", Findings: []Finding{{ID: "checkpoint-finding"}}}
	if err = s.CheckpointOwned(ctx, id, r.owner, checkpoint, []ToolTrace{{ObservationID: "obs1", Name: "read_file"}}); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "admin-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Runner: r}).Register(router)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/projects/1/members/%d", member.ID), strings.NewReader(`{"role":"viewer"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		_, active := r.active[id]
		r.mu.Unlock()
		if !active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.mu.Lock()
	_, active := r.active[id]
	r.mu.Unlock()
	if active {
		t.Fatal("revoked audit retained worker despite context cancellation")
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "cancelled" || run.Result.Summary != checkpoint.Summary || len(run.Trace) != 1 {
		t.Fatal("revocation lost checkpoint", run, err)
	}
}

func TestDisabledRequesterCannotExecuteQueuedRunOrWriteReview(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	id, _, err := s.EnqueueUser(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateUser(ctx, member.ID, "member", true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Store: s, Repository: runRepo{}, Auditor: &transientAuditor{}, active: map[int64]context.CancelFunc{}}
	r.execute(ctx, id)
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "cancelled" {
		t.Fatal("disabled requester executed", run.Status, err)
	}
	if err = s.UpdateUser(ctx, member.ID, "member", false, ""); err != nil {
		t.Fatal(err)
	}
	id, _, err = s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 2, BaseSHA: "base", HeadSHA: "head"}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f1"}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveReviewUser(ctx, Review{RunID: id, FindingID: "f1", Actor: member.ID, Status: "accepted"}); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer wrote review through store", err)
	}
	reviews, err := s.Reviews(ctx, id)
	if err != nil || len(reviews) != 0 {
		t.Fatal("rejected review persisted")
	}
}
