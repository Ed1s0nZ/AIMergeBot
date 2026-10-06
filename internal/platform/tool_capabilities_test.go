package platform

import (
	"encoding/json"
	"testing"
)

func assertRequiredToolCapabilities(t *testing.T, tools any, names ...string) {
	t.Helper()
	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err = json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, entry := range entries {
		present[entry.Function.Name] = true
	}
	for _, name := range names {
		if !present[name] {
			t.Errorf("missing required capability %s", name)
		}
	}
}
