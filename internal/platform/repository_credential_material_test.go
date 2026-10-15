package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func noCredentialProxy(*http.Request) (*url.URL, error) { return nil, nil }

func TestLegacyCredentialMaterialUsesEffectiveSDKEndpoint(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	makeMaterial := func(token, endpoint, proxy string) legacyRepositoryMaterial {
		t.Helper()
		m, err := newLegacyRepositoryMaterial(token, endpoint, func(r *http.Request) (*url.URL, error) {
			repo, err := NewGitLabRepository(token, endpoint)
			if err != nil || r.URL.String() != repo.Client.BaseURL().String() {
				t.Fatal("proxy policy did not observe actual SDK endpoint")
			}
			if proxy == "" {
				return nil, nil
			}
			return url.Parse(proxy)
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := makeMaterial("PRIVATE-TOKEN", "http://gitlab.example/Prefix", "")
	b := makeMaterial("PRIVATE-TOKEN", "http://gitlab.example/Prefix/", "")
	fingerprint := func(m legacyRepositoryMaterial) []byte {
		v, err := m.fingerprint(key)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if !bytes.Equal(fingerprint(a), fingerprint(b)) {
		t.Fatal("ignored trailing slash changed identity")
	}
	for _, m := range []legacyRepositoryMaterial{
		makeMaterial("PRIVATE-TOKEN-2", "http://gitlab.example/Prefix", ""),
		makeMaterial("PRIVATE-TOKEN ", "http://gitlab.example/Prefix", ""),
		makeMaterial("PRIVATE-TOKEN", "http://gitlab.example/prefix", ""),
		makeMaterial("PRIVATE-TOKEN", "https://gitlab.example/Prefix", ""),
		makeMaterial("PRIVATE-TOKEN", "http://gitlab.example/Prefix", "http://user:PRIVATE-PROXY@proxy.example:8080"),
	} {
		if bytes.Equal(fingerprint(a), fingerprint(m)) {
			t.Fatal("effective credential or transport change lost")
		}
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%x"} {
			if strings.Contains(fmt.Sprintf(format, m), "PRIVATE") {
				t.Fatal("material diagnostic exposed secret")
			}
		}
		raw, err := json.Marshal(m)
		if err != nil || string(raw) != "{}" {
			t.Fatal("material has public JSON fields")
		}
	}
	if bytes.Equal(fingerprint(a), func() []byte { v, _ := a.fingerprint(bytes.Repeat([]byte{8}, 32)); return v }()) {
		t.Fatal("fingerprint not keyed")
	}
}

func TestLegacyCredentialMaterialRejectsLossyAndSecretErrors(t *testing.T) {
	for _, input := range []struct{ token, endpoint string }{
		{"PRIVATE\xff", "https://gitlab.example"},
		{"PRIVATE", "https://gitlab.example/\xff"},
		{"PRIVATE", "https://user:PRIVATE@gitlab.example"},
		{"PRIVATE", "https://gitlab.example?PRIVATE"},
	} {
		if _, err := newLegacyRepositoryMaterial(input.token, input.endpoint, noCredentialProxy); !errors.Is(err, errLegacyCredentialMaterial) || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("invalid material accepted or secret error exposed", err)
		}
	}
	if _, err := newLegacyRepositoryMaterial("PRIVATE", "https://gitlab.example", func(*http.Request) (*url.URL, error) { return nil, errors.New("PRIVATE-PROXY-ERROR") }); !errors.Is(err, errLegacyCredentialMaterial) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("proxy error exposed", err)
	}
	if _, err := (legacyRepositoryMaterial{}).fingerprint(make([]byte, 32)); !errors.Is(err, errLegacyCredentialMaterial) {
		t.Fatal("empty material accepted", err)
	}
	for _, key := range [][]byte{nil, make([]byte, 31), make([]byte, 33)} {
		if _, err := credentialMaterialFixture(t, "PRIVATE").fingerprint(key); !errors.Is(err, errLegacyCredentialMaterial) {
			t.Fatal("invalid key accepted", err)
		}
	}
}
