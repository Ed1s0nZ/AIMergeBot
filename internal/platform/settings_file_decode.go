package platform

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var errReloadConfiguration = errors.New("invalid saved configuration")

func initialSettingsFile() Settings {
	return Settings{GitAudit: GitAuditSettings{Enabled: true}, GenerateSequenceDiagrams: true, VerifyFindings: true}
}

func applySettingsFileDefaults(cfg *Settings) {
	if cfg.Revision == 0 {
		cfg.Revision = 1
	}
	defaultGitAudit(&cfg.GitAudit)
	defaultAuditQuotas(&cfg.AuditQuotas)
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.AuditWorkers == 0 {
		cfg.AuditWorkers = 2
	}
	if cfg.AuditTimeoutSeconds == 0 {
		cfg.AuditTimeoutSeconds = 300
	}
	if cfg.ReAct.MaxSteps == 0 {
		cfg.ReAct.MaxSteps = 16
	}
}

func decimalRevision(s string, positive bool) (uint64, bool) {
	if s == "" || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && (!positive || n > 0)
}

// Reload accepts the ordinary YAML emitted by Save. Startup keeps its legacy
// parser; this stricter boundary cannot resolve aliases or import project ACLs.
func readSettingsForReload(path string) (Settings, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Settings{}, errReloadConfiguration
	}
	f, err := os.Open(path)
	if err != nil {
		return Settings{}, errReloadConfiguration
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Settings{}, errReloadConfiguration
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return Settings{}, errReloadConfiguration
	}
	return decodeSettingsForReload(raw)
}

func decodeSettingsForReload(raw []byte) (Settings, error) {
	bad := func() (Settings, error) { return Settings{}, errReloadConfiguration }
	if len(raw) > 1<<20 || !utf8.Valid(raw) {
		return bad()
	}
	var root, trailing yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(raw))
	if d.Decode(&root) != nil || d.Decode(&trailing) != io.EOF || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return bad()
	}
	nodes := 0
	var walk func(*yaml.Node, int) bool
	walk = func(n *yaml.Node, depth int) bool {
		nodes++
		if depth > 64 || nodes > 100000 || n.Anchor != "" || n.Kind == yaml.AliasNode {
			return false
		}
		if n.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || k.Value == "<<" || seen[k.Value] {
					return false
				}
				seen[k.Value] = true
			}
		}
		for _, child := range n.Content {
			if !walk(child, depth+1) {
				return false
			}
		}
		return true
	}
	if !walk(&root, 0) {
		return bad()
	}
	revision := uint64(1)
	m := root.Content[0]
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == "config_revision" {
			n := m.Content[i+1]
			value, ok := decimalRevision(n.Value, false)
			if n.Kind != yaml.ScalarNode || n.Tag != "!!int" || !ok {
				return bad()
			}
			if value != 0 {
				revision = value
			}
		}
	}
	cfg := initialSettingsFile()
	d = yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	if d.Decode(&cfg) != nil {
		return bad()
	}
	cfg.Revision = revision
	applySettingsFileDefaults(&cfg)
	return cfg, nil
}
