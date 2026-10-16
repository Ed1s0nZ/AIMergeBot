//go:build ignore

// A synthetic wire oracle for the existing GitLab SDK's base-prefix handling.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/xanzy/go-gitlab"
)

func main() {
	paths := make(chan string, 16)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"path_with_namespace":"org/repo"}`))
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	bases, effective, wire := map[string]string{}, map[string]string{}, map[string]string{}
	for name, prefix := range map[string]string{
		"root": "", "dot": "/%2E", "upper": "/Prefix", "lower": "/prefix",
		"encoded_dash": "/tenant%2DOne", "literal_dash": "/tenant-One",
		"encoded_slash": "/tenant%2FOne", "literal_slash": "/tenant/One",
	} {
		repo, err := gitlab.NewClient("SYNTHETIC", gitlab.WithBaseURL(strings.TrimRight(server.URL+prefix, "/")+"/api/v4"), gitlab.WithoutRetries(), gitlab.WithHTTPClient(client))
		if err != nil {
			panic("SDK construction failed")
		}
		bases[name] = repo.BaseURL().String()
		baseRequest, err := repo.NewRequest(http.MethodGet, "", nil, nil)
		if err != nil {
			panic("SDK request construction failed")
		}
		effective[name] = baseRequest.URL.String()
		project, _, err := repo.Projects.GetProject(1, nil)
		if err != nil || project.ID != 1 {
			panic("synthetic project read failed")
		}
		wire[name] = <-paths
		if wire[name] != baseRequest.URL.EscapedPath()+"projects/1" {
			panic("request builder and actual wire prefix differ")
		}
	}
	result := map[string]bool{
		"path_case_remains_distinct":   wire["upper"] != wire["lower"],
		"encoded_dash_base_distinct":   bases["encoded_dash"] != bases["literal_dash"],
		"encoded_dash_same_wire":       wire["encoded_dash"] == wire["literal_dash"],
		"encoded_slash_base_distinct":  bases["encoded_slash"] != bases["literal_slash"],
		"encoded_slash_same_wire":      wire["encoded_slash"] == wire["literal_slash"],
		"encoded_dot_base_distinct":    bases["dot"] != bases["root"],
		"encoded_dot_wire_distinct":    wire["dot"] != wire["root"],
		"request_builder_matches_wire": true,
		"effective_base_merges_dash":   effective["encoded_dash"] == effective["literal_dash"],
		"effective_base_merges_slash":  effective["encoded_slash"] == effective["literal_slash"],
		"effective_base_keeps_case":    effective["upper"] != effective["lower"],
	}
	for _, key := range []string{"path_case_remains_distinct", "encoded_dash_base_distinct", "encoded_dash_same_wire", "encoded_slash_base_distinct", "encoded_slash_same_wire", "encoded_dot_base_distinct", "encoded_dot_wire_distinct"} {
		if !result[key] {
			panic("wire identity counterexample changed: " + key)
		}
	}
	for _, key := range []string{"effective_base_merges_dash", "effective_base_merges_slash", "effective_base_keeps_case"} {
		if !result[key] {
			panic("effective base identity mismatch: " + key)
		}
	}
	raw, _ := json.Marshal(result)
	fmt.Println(string(raw))
}
