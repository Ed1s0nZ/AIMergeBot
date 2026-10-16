package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xanzy/go-gitlab"
)

func TestRepositoryScopeV1SerializedIdentityUnchanged(t *testing.T) {
	// Digest of the pre-v2 fixture's exact JSON field order and values. Adding
	// optional v2 fields must not rewrite old policy identity.
	raw, err := json.Marshal(repositoryScopeFixture())
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != "c2f0247b74c0560f5f2892b6e28a462c8dfdf92f1913d5e733cb546e3460debc" {
		t.Fatal("v1 serialized identity changed", err)
	}
	var out RepositoryScope
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(&out)
	if err != nil || string(raw) != string(again) {
		t.Fatal("v1 roundtrip changed bytes", err)
	}
}

func legacyScopeReference(t *testing.T, endpoint string, project int) RepositoryReference {
	t.Helper()
	base, _, err := gitLabScopeEndpoints(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return RepositoryReference{ProjectID: project, IdentityRevision: 1, Provider: "gitlab", RemoteID: int64(project), FullName: "org/repo", Credential: RepositoryCredential{Kind: "legacy_gitlab", LegacyRepositoryRevision: 1}, LegacyEndpoint: &RepositoryLegacyEndpoint{ConfigURL: endpoint, APIBaseURL: base, TransportPolicy: legacyFixedSessionPolicy}}
}

func legacyScopeFixture(t *testing.T) *RepositoryScope {
	s := repositoryScopeFixture()
	s.Version, s.Observation.Provider = 2, "gitlab"
	s.Target = legacyScopeReference(t, "http://gitlab.invalid/Prefix", 1)
	s.Source = s.Target.clone()
	return s
}

func TestRepositoryScopeV2StrictUnionAndCopy(t *testing.T) {
	s := legacyScopeFixture(t)
	s.Source.ProjectID, s.Source.RemoteID = 2, 2
	s.Context = []RepositoryContextReference{{Repository: s.Source.clone(), SHA: s.Observation.HeadSHA}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RepositoryScope
	if err = json.Unmarshal(raw, &decoded); err != nil || !sameRepositoryReference(decoded.Source, decoded.Context[0].Repository) {
		t.Fatal("independently decoded identical references differ", err)
	}
	clone := decoded.clone()
	clone.Target.LegacyEndpoint.ConfigURL += "/changed"
	clone.Source.LegacyEndpoint.ConfigURL += "/changed"
	clone.Context[0].Repository.LegacyEndpoint.ConfigURL += "/changed"
	if decoded.Target.LegacyEndpoint.ConfigURL != s.Target.LegacyEndpoint.ConfigURL || decoded.Source.LegacyEndpoint.ConfigURL != s.Source.LegacyEndpoint.ConfigURL || decoded.Context[0].Repository.LegacyEndpoint.ConfigURL != s.Source.LegacyEndpoint.ConfigURL {
		t.Fatal("clone aliased a nested endpoint")
	}
	normal, err := normalizeRepositoryScope(&decoded)
	if err != nil || normal.Target.LegacyEndpoint.ConfigURL != s.Target.LegacyEndpoint.ConfigURL {
		t.Fatal("producer changed a legacy endpoint", err)
	}
	body := string(raw)
	for name, bad := range map[string]string{
		"old version with new union": strings.Replace(body, `"version":2`, `"version":1`, 1),
		"old profile":                strings.Replace(body, legacyFixedSessionPolicy, "legacy_http_default_v1", 1),
		"unknown profile":            strings.Replace(body, legacyFixedSessionPolicy, "UNKNOWN-PRIVATE", 1),
		"null field":                 strings.Replace(body, `"config_url":"http://gitlab.invalid/Prefix"`, `"config_url":null`, 1),
		"case alias":                 strings.Replace(body, `"config_url":`, `"CONFIG_URL":`, 1),
		"duplicate nested field":     strings.Replace(body, `"transport_policy":`, `"config_url":"http://gitlab.invalid/Prefix","transport_policy":`, 1),
		"unknown nested field":       strings.Replace(body, `"transport_policy":`, `"extra":true,"transport_policy":`, 1),
		"mismatched SDK base":        strings.Replace(body, `/Prefix/api/v4/`, `/prefix/api/v4/`, 1),
		"nonempty legacy origin":     strings.Replace(body, `"api_origin":""`, `"api_origin":"https://gitlab.invalid"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var out RepositoryScope
			if err := json.Unmarshal([]byte(bad), &out); !errors.Is(err, ErrRepositoryScope) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("invalid scope accepted or input leaked", err)
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing endpoint":        func(r map[string]any) { delete(r, "legacy_endpoint") },
		"null endpoint":           func(r map[string]any) { r["legacy_endpoint"] = nil },
		"missing endpoint config": func(r map[string]any) { delete(r["legacy_endpoint"].(map[string]any), "config_url") },
		"integration endpoint forbidden": func(r map[string]any) {
			r["credential"] = map[string]any{"kind": "integration", "integration_id": 1, "integration_revision": 1}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			mutate(object["target"].(map[string]any))
			bad, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			var out RepositoryScope
			if json.Unmarshal(bad, &out) == nil {
				t.Fatal("invalid endpoint union accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*RepositoryScope){
		"wrong remote ID":     func(s *RepositoryScope) { s.Target.RemoteID++ },
		"cross prefix":        func(s *RepositoryScope) { s.Source = legacyScopeReference(t, "http://gitlab.invalid/Other", 2) },
		"invalid name":        func(s *RepositoryScope) { s.Target.FullName = "org/../repo" },
		"credential overflow": func(s *RepositoryScope) { s.Target.Credential.LegacyRepositoryRevision = -1 },
		"secret URL":          func(s *RepositoryScope) { s.Target.LegacyEndpoint.ConfigURL = "http://user:PRIVATE@gitlab.invalid" },
	} {
		t.Run(name, func(t *testing.T) {
			out := s.clone()
			mutate(out)
			if out.validate() == nil {
				t.Fatal("invalid reference accepted")
			}
		})
	}
	github := repositoryScopeFixture()
	github.Version = 2
	if github.validate() != nil {
		t.Fatal("integration-only v2 rejected")
	}
	if !errors.Is(requireLegacyRepositorySnapshot(context.Background(), nil, repositoryScopeSnapshot(s)), ErrRepositoryUnavailable) {
		t.Fatal("codec enabled scoped execution")
	}
}

func TestRepositoryEndpointKeyUsesActualSDKWireIdentity(t *testing.T) {
	requests := make(chan string, 8)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"path_with_namespace":"org/repo"}`))
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	keys, bases := map[string]string{}, map[string]string{}
	for name, prefix := range map[string]string{"root": "", "dot": "/%2E", "upper": "/Prefix", "lower": "/prefix", "encoded_dash": "/tenant%2DOne", "literal_dash": "/tenant-One", "encoded_slash": "/tenant%2FOne", "literal_slash": "/tenant/One"} {
		r := legacyScopeReference(t, server.URL+prefix, 1)
		before := *r.LegacyEndpoint
		key, err := r.endpointKey()
		if err != nil || *r.LegacyEndpoint != before {
			t.Fatal("descriptor mutated", err)
		}
		keys[name], bases[name] = key, r.LegacyEndpoint.APIBaseURL
		if len(requests) != 0 {
			t.Fatal("key construction made a network request")
		}
		repo, err := gitlab.NewClient("", gitlab.WithBaseURL(r.LegacyEndpoint.APIBaseURL), gitlab.WithoutRetries(), gitlab.WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = repo.Projects.GetProject(1, nil); err != nil {
			t.Fatal(err)
		}
		wire := <-requests
		if !strings.HasSuffix(wire, "projects/1") {
			t.Fatal("unexpected wire route")
		}
		var projected struct {
			Version  int    `json:"version"`
			Provider string `json:"provider"`
			Base     string `json:"api_base_url"`
		}
		if err := json.Unmarshal([]byte(key), &projected); err != nil || projected.Version != 1 || projected.Provider != "gitlab" || projected.Base != server.URL+strings.TrimSuffix(wire, "projects/1") {
			t.Fatal("key differs from actual wire prefix", name, err)
		}
	}
	for _, pair := range [][2]string{{"encoded_dash", "literal_dash"}, {"encoded_slash", "literal_slash"}} {
		if bases[pair[0]] == bases[pair[1]] || keys[pair[0]] != keys[pair[1]] {
			t.Fatal("wire alias escaped identity comparison", pair)
		}
	}
	if keys["upper"] == keys["lower"] || keys["dot"] == keys["root"] {
		t.Fatal("distinct wire prefixes merged")
	}
}

func TestRepositoryEndpointKeyCanonicalAuthorityAndACLIdentity(t *testing.T) {
	legacy := legacyScopeReference(t, "https://GitLab.EXAMPLE:443/", 1)
	integration := RepositoryReference{ProjectID: 2, IdentityRevision: 1, Provider: "gitlab", APIOrigin: "https://gitlab.example", RemoteID: 2, FullName: "org/repo", BindingRevision: 1, Credential: RepositoryCredential{Kind: "integration", IntegrationID: 1, IntegrationRevision: 1}}
	a, err := legacy.endpointKey()
	b, err2 := integration.endpointKey()
	if err != nil || err2 != nil || a != b {
		t.Fatal("equivalent endpoints differ", err, err2)
	}
	ipv6 := legacyScopeReference(t, "http://[::1]:80/Prefix", 1)
	key, err := ipv6.endpointKey()
	if err != nil || !strings.Contains(key, `http://[::1]/Prefix/api/v4/`) {
		t.Fatal("IPv6/default port", err)
	}
	s := legacyScopeFixture(t)
	s.Target, s.Source = legacy, legacy.clone()
	integration.RemoteID = 1
	s.Context = []RepositoryContextReference{{Repository: integration, SHA: s.Observation.HeadSHA}}
	if s.validate() == nil {
		t.Fatal("mixed credentials gave one remote two ACL identities")
	}
	// Endpoint equality is insufficient when the same internal project has
	// contradictory frozen config/credential references.
	s.Context = []RepositoryContextReference{}
	s.Source.LegacyEndpoint.ConfigURL = "https://gitlab.example"
	s.Source.LegacyEndpoint.APIBaseURL = "https://gitlab.example/api/v4/"
	if s.validate() == nil {
		t.Fatal("same project accepted conflicting references")
	}
}
