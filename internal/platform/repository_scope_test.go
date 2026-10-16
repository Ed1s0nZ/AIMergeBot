package platform

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func repositoryScopeFixture() *RepositoryScope {
	r := RepositoryReference{ProjectID: 1, IdentityRevision: 1, Provider: "github", APIOrigin: "https://api.github.com", RemoteID: 71, FullName: "org/repo", BindingRevision: 1, Credential: RepositoryCredential{Kind: "integration", IntegrationID: 1, IntegrationRevision: 1}}
	return &RepositoryScope{Version: 1, Target: r, Source: r, Context: []RepositoryContextReference{}, Observation: RepositoryObservation{Provider: "github", Number: 7, BaseTipSHA: strings.Repeat("a", 40), MergeBaseSHA: strings.Repeat("b", 40), HeadSHA: strings.Repeat("c", 40)}}
}

func repositoryScopeSnapshot(scope *RepositoryScope) Snapshot {
	items := []ContextRepository{}
	for _, c := range scope.Context {
		items = append(items, ContextRepository{ProjectID: c.Repository.ProjectID, SHA: c.SHA})
	}
	return Snapshot{ProjectID: scope.Target.ProjectID, SourceProjectID: scope.Source.ProjectID, MRIID: scope.Observation.Number, BaseSHA: scope.Observation.MergeBaseSHA, HeadSHA: scope.Observation.HeadSHA, AuditPolicy: &AuditPolicy{RepositoryScope: scope, ContextRepositories: items}}
}

func TestRepositoryScopeStrictJSON(t *testing.T) {
	s := repositoryScopeFixture()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RepositoryScope
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded.validate() != nil {
		t.Fatal(err)
	}
	base := string(raw)
	bad := []string{
		`null`, `{}`, `[]`, base + `{}`, strings.Replace(base, `"version":1`, `"version":99`, 1),
		strings.Replace(base, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(base, `"version":1`, `"version":1,"VERSION":1`, 1),
		strings.Replace(base, `"context":[]`, `"context":null`, 1),
		strings.Replace(base, `"binding_revision":1,`, ``, 1),
		strings.Replace(base, `"binding_revision":1`, `"binding_revision":null`, 1),
		strings.Replace(base, `"kind":"integration"`, `"kind":"integration","kind":"integration"`, 1),
		strings.Replace(base, `"integration_id":1`, `"integration_id":1,"legacy_repository_revision":null`, 1),
		strings.Replace(base, `"integration_id":1`, `"integration_id":"1"`, 1),
		strings.Replace(base, `"integration_id":1`, `"integration_id":1.0`, 1),
		strings.Replace(base, `"provider":"github"`, `"provider":"UNKNOWN-PRIVATE"`, 1),
		strings.Replace(base, `"full_name":"org/repo"`, `"full_name":"org/../PRIVATE"`, 1),
		"{" + strings.Repeat(" ", repositoryScopeLimit) + base[1:],
	}
	for i, body := range bad {
		if err := json.Unmarshal([]byte(body), &decoded); err == nil {
			t.Fatalf("accepted invalid scope case %d", i)
		}
	}
	if err := json.Unmarshal([]byte(strings.Replace(base, "org/repo", "org/\xffrepo", 1)), &decoded); !errors.Is(err, ErrRepositoryScope) {
		t.Fatal("invalid UTF8", err)
	}
	if err := uniqueScopeJSON([]byte(strings.Repeat("[", 20) + `0` + strings.Repeat("]", 20))); !errors.Is(err, ErrRepositoryScope) {
		t.Fatal("depth", err)
	}
	for _, body := range []string{`{"repository_scope":null}`, `{"REPOSITORY_SCOPE":null}`, `{"repository_scope":` + base + `,"repository_scope":null}`} {
		var policy AuditPolicy
		if err := json.Unmarshal([]byte(body), &policy); !errors.Is(err, ErrRepositoryScope) {
			t.Fatal("policy accepted hidden scope", err)
		}
	}
	var old AuditPolicy
	if err := json.Unmarshal([]byte(`{"model":"legacy","unknown_legacy_field":true}`), &old); err != nil || old.Model != "legacy" || old.RepositoryScope != nil {
		t.Fatal("old optional contract", err)
	}
	stored, _ := json.Marshal(old)
	if strings.Contains(string(stored), "repository_scope") {
		t.Fatal("nil changed old JSON")
	}
}

func TestRepositoryScopeIdentityAndSnapshot(t *testing.T) {
	s := repositoryScopeFixture()
	if validateRepositoryScopeSnapshot(repositoryScopeSnapshot(s)) != nil {
		t.Fatal("valid same repository")
	}
	s.Source.ProjectID = 2
	s.Source.RemoteID = 72
	s.Source.FullName = "fork/repo"
	contextRef := s.Source
	contextRef.ProjectID = 3
	contextRef.Provider = "gitlab"
	contextRef.APIOrigin = "https://gitlab.example"
	contextRef.RemoteID = 3
	contextRef.BindingRevision = 0
	contextRef.Credential = RepositoryCredential{Kind: "legacy_gitlab", LegacyRepositoryRevision: 9007199254740993}
	s.Context = []RepositoryContextReference{{Repository: contextRef, SHA: strings.Repeat("d", 64)}}
	if validateRepositoryScopeSnapshot(repositoryScopeSnapshot(s)) != nil {
		t.Fatal("valid mixed fork context")
	}
	raw, _ := json.Marshal(s)
	if !strings.Contains(string(raw), `"legacy_repository_revision":"9007199254740993"`) {
		t.Fatal("generation precision lost")
	}
	var roundtrip RepositoryScope
	if err := json.Unmarshal(raw, &roundtrip); err != nil || roundtrip.Context[0].Repository.Credential.LegacyRepositoryRevision != 9007199254740993 {
		t.Fatal(err)
	}
	for _, value := range []string{`1`, `"01"`, `"+1"`, `"0"`, `"9223372036854775808"`, `null`} {
		body := strings.Replace(string(raw), `"9007199254740993"`, value, 1)
		if json.Unmarshal([]byte(body), &roundtrip) == nil {
			t.Fatal("accepted invalid generation", value)
		}
	}
	mutations := []func(*RepositoryScope){
		func(v *RepositoryScope) { v.Source.APIOrigin = "https://other.example" },
		func(v *RepositoryScope) { v.Source.RemoteID = v.Target.RemoteID },
		func(v *RepositoryScope) { v.Source.ProjectID = v.Target.ProjectID },
		func(v *RepositoryScope) { v.Context = append(v.Context, v.Context[0]) },
		func(v *RepositoryScope) { v.Context[0].Repository.Credential.IntegrationID = 1 },
		func(v *RepositoryScope) { v.Context[0].Repository.RemoteID = 999 },
		func(v *RepositoryScope) { v.Context[0].SHA = strings.Repeat("A", 40) },
		func(v *RepositoryScope) { v.Observation.HeadSHA = strings.Repeat("a", 64) },
	}
	for i, mutate := range mutations {
		copy := s.clone()
		mutate(copy)
		if copy.validate() == nil {
			t.Fatalf("accepted contradictory identity %d", i)
		}
	}
	for _, mutate := range []func(*Snapshot){func(v *Snapshot) { v.BaseSHA = v.HeadSHA }, func(v *Snapshot) { v.MRIID++ }, func(v *Snapshot) { v.DiffVersionID = 1 }, func(v *Snapshot) { v.AuditPolicy.ContextRepositories = nil }} {
		snap := repositoryScopeSnapshot(s.clone())
		mutate(&snap)
		if validateRepositoryScopeSnapshot(snap) == nil {
			t.Fatal("accepted inconsistent snapshot")
		}
	}
	before := policyDigest(repositoryScopeSnapshot(s).AuditPolicy)
	copy := s.clone()
	copy.Context[0].Repository.Credential.LegacyRepositoryRevision++
	if policyDigest(repositoryScopeSnapshot(copy).AuditPolicy) == before || s.Context[0].Repository.Credential.LegacyRepositoryRevision != 9007199254740993 {
		t.Fatal("digest or clone aliases")
	}
	copy = s.clone()
	copy.Target.APIOrigin = "https://API.GITHUB.COM:443/"
	copy.Source.APIOrigin = "https://API.GITHUB.COM:443/"
	normalized, err := normalizeRepositoryScope(copy)
	if err != nil || normalized.Target.APIOrigin != "https://api.github.com" || copy.Target.APIOrigin == normalized.Target.APIOrigin {
		t.Fatal("normalization mutated source", err)
	}
}
