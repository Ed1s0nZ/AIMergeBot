package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"unicode/utf8"
)

var errLegacyCredentialMaterial = errors.New("legacy repository credential material unavailable")

// This value contains secrets. It has no public serialization and always
// redacts itself in diagnostics. Only its keyed fingerprint may be persisted.
type legacyRepositoryMaterial struct{ encoded []byte }

func (legacyRepositoryMaterial) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository credential material]")
}

// Constructing the SDK client does not contact the repository. Its BaseURL is
// authoritative: native origin normalization would discard legacy prefixes.
func newLegacyRepositoryMaterial(token, endpoint string, proxy func(*http.Request) (*url.URL, error)) (legacyRepositoryMaterial, error) {
	if !utf8.ValidString(token) || !utf8.ValidString(endpoint) || !validURL(endpoint) {
		return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
	}
	repo, err := NewGitLabRepository(token, endpoint)
	if err != nil {
		return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
	}
	base := repo.Client.BaseURL().String()
	req, err := http.NewRequest(http.MethodGet, base, nil)
	if err != nil {
		return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
	}
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	selected, err := proxy(req)
	if err != nil {
		return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
	}
	proxyURL := ""
	if selected != nil {
		proxyURL = selected.String()
		if !utf8.ValidString(proxyURL) {
			return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
		}
	}
	// The current legacy transport has no configurable CIDR allowlist. Do not
	// borrow a notification integration's policy or imply native restrictions.
	input := struct {
		Version         int      `json:"version"`
		TransportPolicy string   `json:"transport_policy"`
		BaseURL         string   `json:"base_url"`
		Token           string   `json:"token"`
		Networks        []string `json:"networks"`
		Proxy           string   `json:"proxy"`
	}{1, "legacy_http_default_v1", base, token, []string{}, proxyURL}
	raw, err := json.Marshal(input)
	if err != nil {
		return legacyRepositoryMaterial{}, errLegacyCredentialMaterial
	}
	return legacyRepositoryMaterial{encoded: raw}, nil
}

func (m legacyRepositoryMaterial) fingerprint(key []byte) ([]byte, error) {
	if len(key) != 32 || len(m.encoded) == 0 {
		return nil, errLegacyCredentialMaterial
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(m.encoded)
	return mac.Sum(nil), nil
}
