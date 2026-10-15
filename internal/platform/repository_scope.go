package platform

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var ErrRepositoryScope = errors.New("invalid fixed repository scope")

const repositoryScopeLimit = 16 * 1024

// RepositoryScope contains identities, never credentials. It grants no access.
type RepositoryScope struct {
	Version     int                          `json:"version"`
	Target      RepositoryReference          `json:"target"`
	Source      RepositoryReference          `json:"source"`
	Context     []RepositoryContextReference `json:"context"`
	Observation RepositoryObservation        `json:"observation"`
}

type RepositoryReference struct {
	ProjectID        int                  `json:"project_id"`
	IdentityRevision int64                `json:"identity_revision"`
	Provider         string               `json:"provider"`
	APIOrigin        string               `json:"api_origin"`
	RemoteID         int64                `json:"remote_id"`
	FullName         string               `json:"full_name"`
	BindingRevision  int64                `json:"binding_revision"`
	Credential       RepositoryCredential `json:"credential"`
}

type RepositoryCredential struct {
	Kind                     string `json:"kind"`
	IntegrationID            int64  `json:"integration_id,omitempty"`
	IntegrationRevision      int64  `json:"integration_revision,omitempty"`
	LegacyRepositoryRevision int64  `json:"legacy_repository_revision,omitempty,string"`
}

type RepositoryContextReference struct {
	Repository RepositoryReference `json:"repository"`
	SHA        string              `json:"sha"`
}

type RepositoryObservation struct {
	Provider     string `json:"provider"`
	Number       int    `json:"number"`
	BaseTipSHA   string `json:"base_tip_sha"`
	MergeBaseSHA string `json:"merge_base_sha"`
	HeadSHA      string `json:"head_sha"`
}

func (s *RepositoryScope) clone() *RepositoryScope {
	if s == nil {
		return nil
	}
	out := *s
	if s.Context != nil {
		out.Context = append([]RepositoryContextReference{}, s.Context...)
	}
	return &out
}

// normalizeRepositoryScope is for producers; consumers validate without repair.
func normalizeRepositoryScope(s *RepositoryScope) (*RepositoryScope, error) {
	if s == nil {
		return nil, ErrRepositoryScope
	}
	out := s.clone()
	if out.Context == nil {
		out.Context = []RepositoryContextReference{}
	}
	for _, r := range []*RepositoryReference{&out.Target, &out.Source} {
		origin, err := canonicalRepositoryAPIOrigin(r.Provider, r.APIOrigin)
		if err != nil {
			return nil, ErrRepositoryScope
		}
		r.APIOrigin = origin
	}
	for i := range out.Context {
		r := &out.Context[i].Repository
		origin, err := canonicalRepositoryAPIOrigin(r.Provider, r.APIOrigin)
		if err != nil {
			return nil, ErrRepositoryScope
		}
		r.APIOrigin = origin
		out.Context[i].SHA = strings.ToLower(out.Context[i].SHA)
	}
	sort.Slice(out.Context, func(i, j int) bool { return out.Context[i].Repository.ProjectID < out.Context[j].Repository.ProjectID })
	out.Observation.BaseTipSHA = strings.ToLower(out.Observation.BaseTipSHA)
	out.Observation.MergeBaseSHA = strings.ToLower(out.Observation.MergeBaseSHA)
	out.Observation.HeadSHA = strings.ToLower(out.Observation.HeadSHA)
	return out, out.validate()
}

func validRepositorySHA(provider, sha string) bool {
	return contextSHAPattern.MatchString(sha) && (provider != "github" || len(sha) == 40)
}

func (r RepositoryReference) validate() error {
	if r.ProjectID <= 0 || r.IdentityRevision <= 0 {
		return ErrRepositoryScope
	}
	// Reuse the route and origin grammar without loading a credential.
	b, err := validateRepositoryBinding(RepositoryBinding{Provider: r.Provider, APIOrigin: r.APIOrigin, RemoteID: r.RemoteID, FullName: r.FullName, IntegrationID: 1})
	if err != nil || b.APIOrigin != r.APIOrigin {
		return ErrRepositoryScope
	}
	c := r.Credential
	switch c.Kind {
	case "integration":
		if r.BindingRevision <= 0 || c.IntegrationID <= 0 || c.IntegrationRevision <= 0 || c.LegacyRepositoryRevision != 0 {
			return ErrRepositoryScope
		}
	case "legacy_gitlab":
		if r.Provider != "gitlab" || r.RemoteID != int64(r.ProjectID) || r.BindingRevision != 0 || c.LegacyRepositoryRevision <= 0 || c.IntegrationID != 0 || c.IntegrationRevision != 0 {
			return ErrRepositoryScope
		}
	default:
		return ErrRepositoryScope
	}
	return nil
}

func (s *RepositoryScope) validate() error {
	if s == nil || s.Version != 1 || s.Context == nil || len(s.Context) > 4 || s.Target.validate() != nil || s.Source.validate() != nil {
		return ErrRepositoryScope
	}
	if s.Target.Provider != s.Source.Provider || s.Target.APIOrigin != s.Source.APIOrigin || s.Target.ProjectID == s.Source.ProjectID && s.Target != s.Source {
		return ErrRepositoryScope
	}
	// A remote identity must not acquire two different internal ACL identities.
	refs := map[int]RepositoryReference{s.Target.ProjectID: s.Target, s.Source.ProjectID: s.Source}
	previous := 0
	for _, c := range s.Context {
		r := c.Repository
		if r.validate() != nil || r.ProjectID <= previous || r.ProjectID == s.Target.ProjectID || !validRepositorySHA(r.Provider, c.SHA) {
			return ErrRepositoryScope
		}
		if prior, ok := refs[r.ProjectID]; ok && prior != r {
			return ErrRepositoryScope
		}
		refs[r.ProjectID] = r
		previous = r.ProjectID
	}
	remote := map[string]int{}
	for project, r := range refs {
		key, _ := json.Marshal([]any{r.Provider, r.APIOrigin, r.RemoteID})
		if other, ok := remote[string(key)]; ok && other != project {
			return ErrRepositoryScope
		}
		remote[string(key)] = project
	}
	o := s.Observation
	if o.Provider != s.Target.Provider || o.Number <= 0 || !validRepositorySHA(o.Provider, o.BaseTipSHA) || !validRepositorySHA(o.Provider, o.MergeBaseSHA) || !validRepositorySHA(o.Provider, o.HeadSHA) {
		return ErrRepositoryScope
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > repositoryScopeLimit {
		return ErrRepositoryScope
	}
	return nil
}

func validateRepositoryScopeSnapshot(snap Snapshot) error {
	if snap.AuditPolicy == nil || snap.AuditPolicy.RepositoryScope == nil {
		return nil
	}
	s := snap.AuditPolicy.RepositoryScope
	if s.validate() != nil || s.Target.ProjectID != snap.ProjectID || s.Source.ProjectID != snap.SourceProjectID || s.Observation.Number != snap.MRIID || s.Observation.MergeBaseSHA != snap.BaseSHA || s.Observation.HeadSHA != snap.HeadSHA || s.Observation.Provider == "github" && snap.DiffVersionID != 0 {
		return ErrRepositoryScope
	}
	items := snap.AuditPolicy.ContextRepositories
	if len(items) != len(s.Context) {
		return ErrRepositoryScope
	}
	for i, c := range s.Context {
		if items[i].ProjectID != c.Repository.ProjectID || items[i].SHA != c.SHA {
			return ErrRepositoryScope
		}
	}
	return nil
}
