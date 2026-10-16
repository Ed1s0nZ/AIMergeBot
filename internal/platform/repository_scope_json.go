package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

func (s *RepositoryScope) UnmarshalJSON(raw []byte) error {
	if len(raw) > repositoryScopeLimit || !utf8.Valid(raw) || !json.Valid(raw) || uniqueScopeJSON(raw) != nil || repositoryScopeShape(raw) != nil {
		return ErrRepositoryScope
	}
	type plain RepositoryScope
	var out plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || (*RepositoryScope)(&out).validate() != nil {
		return ErrRepositoryScope
	}
	*s = RepositoryScope(out)
	return nil
}

func uniqueScopeJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return ErrRepositoryScope
		}
		token, err := d.Token()
		if err != nil {
			return ErrRepositoryScope
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return ErrRepositoryScope
				}
				seen[name] = true
				if walk(depth+1) != nil {
					return ErrRepositoryScope
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim('}') {
				return ErrRepositoryScope
			}
		case json.Delim('['):
			for d.More() {
				if walk(depth+1) != nil {
					return ErrRepositoryScope
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return ErrRepositoryScope
			}
		}
		return nil
	}
	if walk(0) != nil {
		return ErrRepositoryScope
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrRepositoryScope
	}
	return nil
}

// Exact keys avoid encoding/json's case-insensitive aliases and null defaults.
func scopeObject(raw json.RawMessage, keys ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(keys) {
		return nil, ErrRepositoryScope
	}
	for _, key := range keys {
		v, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, ErrRepositoryScope
		}
	}
	return fields, nil
}

func repositoryScopeShape(raw json.RawMessage) error {
	s, err := scopeObject(raw, "version", "target", "source", "context", "observation")
	if err != nil {
		return ErrRepositoryScope
	}
	var version int
	if json.Unmarshal(s["version"], &version) != nil || (version != 1 && version != 2) || repositoryReferenceShape(s["target"], version) != nil || repositoryReferenceShape(s["source"], version) != nil {
		return ErrRepositoryScope
	}
	if _, err = scopeObject(s["observation"], "provider", "number", "base_tip_sha", "merge_base_sha", "head_sha"); err != nil {
		return ErrRepositoryScope
	}
	var contexts []json.RawMessage
	if json.Unmarshal(s["context"], &contexts) != nil || contexts == nil || len(contexts) > 4 {
		return ErrRepositoryScope
	}
	for _, c := range contexts {
		fields, err := scopeObject(c, "repository", "sha")
		if err != nil || repositoryReferenceShape(fields["repository"], version) != nil {
			return ErrRepositoryScope
		}
	}
	return nil
}

func repositoryReferenceShape(raw json.RawMessage, version int) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrRepositoryScope
	}
	keys := []string{"project_id", "identity_revision", "provider", "api_origin", "remote_id", "full_name", "binding_revision", "credential"}
	if _, exists := fields["legacy_endpoint"]; exists {
		if version != 2 {
			return ErrRepositoryScope
		}
		keys = append(keys, "legacy_endpoint")
		if _, err := scopeObject(fields["legacy_endpoint"], "config_url", "api_base_url", "transport_policy"); err != nil {
			return ErrRepositoryScope
		}
	}
	r, err := scopeObject(raw, keys...)
	if err != nil {
		return ErrRepositoryScope
	}
	var credential map[string]json.RawMessage
	var kind string
	if json.Unmarshal(r["credential"], &credential) != nil || json.Unmarshal(credential["kind"], &kind) != nil {
		return ErrRepositoryScope
	}
	switch kind {
	case "integration":
		_, err = scopeObject(r["credential"], "kind", "integration_id", "integration_revision")
	case "legacy_gitlab":
		_, err = scopeObject(r["credential"], "kind", "legacy_repository_revision")
		var value string
		if json.Unmarshal(credential["legacy_repository_revision"], &value) != nil || len(value) == 0 || value[0] < '1' || value[0] > '9' || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return ErrRepositoryScope
		}
	default:
		return ErrRepositoryScope
	}
	return err
}

// Legacy policies retain their optional fields. A new scope cannot be hidden
// by a duplicate root key, case alias, or explicit null.
func (p *AuditPolicy) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return ErrRepositoryScope
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return ErrRepositoryScope
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return ErrRepositoryScope
		}
		if strings.EqualFold(name, "repository_scope") && (name != "repository_scope" || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return ErrRepositoryScope
		}
	}
	type plain AuditPolicy
	var out plain
	if json.Unmarshal(raw, &out) != nil {
		return ErrRepositoryScope
	}
	*p = AuditPolicy(out)
	return nil
}
