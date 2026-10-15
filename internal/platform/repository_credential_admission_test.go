package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func credentialAdmissionSnapshot(mr int) Snapshot {
	return Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
}

func TestCredentialAdmissionTransactionNeverCallsNetworkFence(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{})
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	ctx, _, err := svc.legacyOperationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err = svc.legacyOperationContext(ctx)
	if err != nil || len(capturedRepositoryCredentials(ctx)) != 1 {
		t.Fatal("same credential not deduplicated", err)
	}
	callbacks := 0
	ctx = withRepositoryFence(ctx, func(context.Context) error { callbacks++; return nil })
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, created, err := s.enqueueTx(ctx, tx, credentialAdmissionSnapshot(1), 0, false, false)
	if err != nil || !created || id <= 0 {
		tx.Rollback()
		t.Fatal("single-connection transactional admission failed", err)
	}
	if err = tx.Commit(); err != nil || callbacks != 0 {
		t.Fatal("transaction called an opaque network fence", callbacks, err)
	}
	next := svc.Snapshot()
	next.OpenAI.Model = "model-only-change"
	if err = svc.Save(next); err != nil {
		t.Fatal(err)
	}
	if _, created, err = s.Enqueue(ctx, credentialAdmissionSnapshot(2), 0, false); err != nil || !created {
		t.Fatal("model-only save invalidated operation", err)
	}
	next = svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(next); err != nil {
		t.Fatal(err)
	}
	var events int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	background := copyRepositoryCredentials(context.Background(), ctx)
	if len(capturedRepositoryCredentials(background)) != 1 {
		t.Fatal("background lost immutable credential")
	}
	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, created, err = s.enqueueTx(background, tx, credentialAdmissionSnapshot(3), 0, false, false)
	tx.Rollback()
	if !errors.Is(err, ErrRepositoryUnavailable) || created {
		t.Fatal("rotated metadata admitted in transaction", err)
	}
	if err = s.recordLegacyPollSeen(background, credentialAdmissionSnapshot(3)); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("rotated polling metadata became seen", err)
	}
	var count, afterEvents, seen int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 2 {
		t.Fatal("rejected admission created a run", count, err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&afterEvents); err != nil || events != afterEvents {
		t.Fatal("rejected admission created an event", err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_poll_seen`).Scan(&seen); err != nil || seen != 0 {
		t.Fatal("rejected poll consumed seen marker", err)
	}
}

func TestCredentialAdmissionRejectsOtherStoreAndActiveRebinding(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{})
	_, other := activationFixture(t, credentialActivationOptions{})
	ctx, _, err := svc.legacyOperationContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activationGeneration(t, s) != activationGeneration(t, other) {
		t.Fatal("counterexample must use equal generations")
	}
	if _, created, err := other.Enqueue(ctx, credentialAdmissionSnapshot(1), 0, false); !errors.Is(err, ErrRepositoryUnavailable) || created {
		t.Fatal("descriptor from another store authorized admission", err)
	}
	before := activationGeneration(t, s)
	replacement, err := OpenSettings(svc.path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = replacement.bindRepositoryCredentialActivation(context.Background(), s, credentialActivationOptions{proxy: noCredentialProxy}); !errors.Is(err, errCredentialActivationUnavailable) {
		t.Fatal("active binding was replaced", err)
	}
	if activationGeneration(t, s) != before {
		t.Fatal("rejected binding mutated generation")
	}
	if _, err = svc.captureLegacyCredential(context.Background()); err != nil {
		t.Fatal("rejected replacement disabled original service", err)
	}
}

func TestCredentialAdmissionPendingPreservesCheckpointsAndDoesNotRetry(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{persist: func(path string, raw []byte) (bool, error) {
		replaced, err := persistSettingsFile(path, raw, true)
		if err != nil {
			return replaced, err
		}
		return true, errors.New("PRIVATE-DURABILITY-FAILURE")
	}})
	ctx := context.Background()
	running, _, err := s.Enqueue(ctx, credentialAdmissionSnapshot(1), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Checkpoint(ctx, running, AuditResult{Summary: "RETAINED"}, nil); err != nil {
		t.Fatal(err)
	}
	pending, _, err := s.Enqueue(ctx, credentialAdmissionSnapshot(2), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	next := svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(next); !errors.Is(err, errCredentialActivationPending) {
		t.Fatal(err)
	}
	// Changing quota controls must never disable the dedicated credential gate.
	s.BindQuotaSettings(nil)
	if _, _, err = s.Enqueue(ctx, credentialAdmissionSnapshot(3), 0, false); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("unbinding quotas disabled credential admission", err)
	}
	if _, err = s.Claim(ctx); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("pending credentials started another run", err)
	}
	if err = s.Checkpoint(ctx, running, AuditResult{Summary: "REPLACEMENT"}, nil); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("pending checkpoint accepted", err)
	}
	if err = s.Finish(ctx, running, "succeeded", "", AuditResult{Summary: "REPLACEMENT"}, nil); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("pending finish accepted", err)
	}
	if _, err = s.FailAndRetry(ctx, running, "temporary", AuditResult{Summary: "REPLACEMENT"}, nil, 0); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("pending credentials scheduled child", err)
	}
	var status, result, started string
	if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=?`, running).Scan(&status, &result); err != nil || status != "running" || !strings.Contains(result, "RETAINED") {
		t.Fatal("rejected writes replaced checkpoint", status, err)
	}
	if err = s.DB.QueryRow(`SELECT status,COALESCE(started_at,'') FROM platform_runs WHERE id=?`, pending).Scan(&status, &started); err != nil || status != "pending" || started != "" {
		t.Fatal("rejected claim consumed daily start", status, err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal("credential pending incorrectly became a worker lease failure", err)
	}
	if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=?`, running).Scan(&status, &result); err != nil || status != "failed" || !strings.Contains(result, "RETAINED") {
		t.Fatal("orphan stop lost checkpoint", status, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 2 {
		t.Fatal("rejected retry created child", count, err)
	}
}

func TestCredentialRejectedEpochFinishKeepsLatestCheckpoint(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{persist: func(string, []byte) (bool, error) { return false, errors.New("PRIVATE-FILE-ERROR") }})
	ctx, _, err := svc.legacyOperationContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := s.Enqueue(ctx, credentialAdmissionSnapshot(1), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Checkpoint(ctx, id, AuditResult{Summary: "LATEST-CHECKPOINT"}, nil); err != nil {
		t.Fatal(err)
	}
	next := svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(next); err == nil {
		t.Fatal("expected definite file failure")
	}
	runner := &Runner{Store: s, Settings: svc}
	if runner.finishWithContext(ctx, id, "succeeded", "", AuditResult{Summary: "STALE-RESULT"}, nil) {
		t.Fatal("old epoch completed")
	}
	var status, result string
	if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=?`, id).Scan(&status, &result); err != nil || status != "failed" || !strings.Contains(result, "LATEST-CHECKPOINT") || strings.Contains(result, "STALE-RESULT") {
		t.Fatal("old epoch replaced latest checkpoint", status, err)
	}
}
