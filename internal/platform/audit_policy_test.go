package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditIdentityIncludesBaseAndPolicy(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{RepositoryURL: "https://one.example", Model: "one", Excluded: []string{"md"}}}
	first, created, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil || !created {
		t.Fatal(err)
	}
	same, created, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil || created || same != first {
		t.Fatal("duplicate identity")
	}
	for _, change := range []func(*Snapshot){func(v *Snapshot) { v.BaseSHA = "new-base" }, func(v *Snapshot) { p := *v.AuditPolicy; p.Model = "two"; v.AuditPolicy = &p }, func(v *Snapshot) { p := *v.AuditPolicy; p.RepositoryURL = "https://two.example"; v.AuditPolicy = &p }} {
		next := snap
		change(&next)
		id, created, err := s.Enqueue(ctx, next, 1, false)
		if err != nil || !created || id == first {
			t.Fatalf("changed identity reused: %v", err)
		}
	}
	run, err := s.Run(ctx, first)
	if err != nil || run.AuditPolicy == nil || run.AuditPolicy.Model != "one" {
		t.Fatalf("captured policy lost: %v", err)
	}
}
func TestFailedAndCancelledRunsCanBeResubmitted(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
			id, _, err := s.Enqueue(ctx, snap, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(ctx, id, status, "", AuditResult{}, nil); err != nil {
				t.Fatal(err)
			}
			next, created, err := s.Enqueue(ctx, snap, 1, false)
			if err != nil || !created || next == id {
				t.Fatalf("terminal failure prevents retry: %v", err)
			}
		})
	}
}
func TestCapturedPolicyContainsNoCredentials(t *testing.T) {
	cfg := Settings{}
	cfg.GitLab.Token = "PRIVATE-GIT-TOKEN"
	cfg.OpenAI.APIKey = "PRIVATE-MODEL-KEY"
	cfg.WebhookToken = "PRIVATE-WEBHOOK"
	cfg.OpenAI.Model = "model"
	cfg.WhitelistExtensions = []string{"md"}
	p := capturePolicy(cfg)
	cfg.WhitelistExtensions[0] = "go"
	if p.Excluded[0] != "md" {
		t.Fatal("policy aliases mutable config")
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("credentials persisted")
	}
}
