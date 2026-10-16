package platform

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const legacyFixedSessionPolicy = "legacy_fixed_session_v2"

// RepositoryLegacyEndpoint describes an address and profile, not credentials
// or permission to execute. Scoped execution remains independently gated.
type RepositoryLegacyEndpoint struct {
	ConfigURL       string `json:"config_url"`
	APIBaseURL      string `json:"api_base_url"`
	TransportPolicy string `json:"transport_policy"`
}

func (r RepositoryReference) clone() RepositoryReference {
	if r.LegacyEndpoint != nil {
		endpoint := *r.LegacyEndpoint
		r.LegacyEndpoint = &endpoint
	}
	return r
}

func sameRepositoryReference(a, b RepositoryReference) bool {
	if (a.LegacyEndpoint == nil) != (b.LegacyEndpoint == nil) {
		return false
	}
	if a.LegacyEndpoint != nil && *a.LegacyEndpoint != *b.LegacyEndpoint {
		return false
	}
	a.LegacyEndpoint, b.LegacyEndpoint = nil, nil
	return a == b
}

func (r RepositoryReference) validateVersion(version int) error {
	switch version {
	case 1:
		if r.LegacyEndpoint != nil {
			return ErrRepositoryScope
		}
	case 2:
		if r.Credential.Kind == "legacy_gitlab" && r.LegacyEndpoint == nil {
			return ErrRepositoryScope
		}
	default:
		return ErrRepositoryScope
	}
	return r.validate()
}

func (r RepositoryReference) validateLegacyEndpoint() error {
	e, c := r.LegacyEndpoint, r.Credential
	if e == nil || r.Provider != "gitlab" || r.APIOrigin != "" || r.BindingRevision != 0 || r.RemoteID != int64(r.ProjectID) || c.Kind != "legacy_gitlab" || c.LegacyRepositoryRevision <= 0 || c.IntegrationID != 0 || c.IntegrationRevision != 0 || !validRepositoryFullName(r.Provider, r.FullName) {
		return ErrRepositoryScope
	}
	if e.TransportPolicy != legacyFixedSessionPolicy || len(e.ConfigURL) > 2048 || len(e.APIBaseURL) > 4096 || !utf8.ValidString(e.ConfigURL) || !utf8.ValidString(e.APIBaseURL) || !validURL(e.ConfigURL) || !validURL(e.APIBaseURL) {
		return ErrRepositoryScope
	}
	base, _, err := gitLabScopeEndpoints(e.ConfigURL)
	if err != nil || base != e.APIBaseURL {
		return ErrRepositoryScope
	}
	return nil
}

// Both values come from the same SDK as production. The effective prefix may
// differ from BaseURL escaping. Construction is network-free and token-free.
func gitLabScopeEndpoints(configURL string) (base, effective string, err error) {
	repo, err := NewGitLabRepository("", configURL)
	if err != nil {
		return "", "", ErrRepositoryScope
	}
	request, err := repo.Client.NewRequest(http.MethodGet, "", nil, nil)
	if err != nil {
		return "", "", ErrRepositoryScope
	}
	return repo.Client.BaseURL().String(), request.URL.String(), nil
}

func sameScopeEndpoint(version int, a, b RepositoryReference) bool {
	if a.Provider != b.Provider {
		return false
	}
	if version == 1 {
		return a.APIOrigin == b.APIOrigin
	}
	x, err := a.endpointKey()
	if err != nil {
		return false
	}
	y, err := b.endpointKey()
	return err == nil && x == y
}

func (r RepositoryReference) endpointKey() (string, error) {
	if r.validate() != nil {
		return "", ErrRepositoryScope
	}
	base := r.APIOrigin + "/"
	if r.Provider == "gitlab" {
		configURL := r.APIOrigin
		if r.LegacyEndpoint != nil {
			configURL = r.LegacyEndpoint.ConfigURL
		}
		var err error
		_, base, err = gitLabScopeEndpoints(configURL)
		if err != nil {
			return "", ErrRepositoryScope
		}
	}
	u, err := url.Parse(base)
	if err != nil || !validURL(base) {
		return "", ErrRepositoryScope
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if u.Scheme == "http" && port == "80" || u.Scheme == "https" && port == "443" {
		port = ""
	}
	u.Host = host
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(struct {
		Version    int    `json:"version"`
		Provider   string `json:"provider"`
		APIBaseURL string `json:"api_base_url"`
	}{1, r.Provider, u.String()}) != nil || out.Len()-1 > 4608 {
		return "", ErrRepositoryScope
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}
