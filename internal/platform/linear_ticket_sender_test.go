package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLinearTicketSenderBoundariesAndRevocation(t *testing.T) {
	id := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
	valid := `{"data":{"issueCreate":{"success":true,"issue":{"id":"` + id + `","url":"https://linear.app/example/issue/LIN-123/finding"}}}}`
	for _, mode := range []string{"ack", "invalid_endpoint", "missing_authorization", "revoked", "post_revoked", "transport_error", "oversized", "graphql_error"} {
		t.Run(mode, func(t *testing.T) {
			writes, checks := 0, 0
			credentials := IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture-api-key"}
			if mode == "invalid_endpoint" {
				credentials.Endpoint = "https://attacker.test/graphql"
			}
			authorize := func(context.Context) error {
				checks++
				if mode == "revoked" || mode == "post_revoked" && checks > 1 {
					return ErrProjectPermission
				}
				return nil
			}
			if mode == "missing_authorization" {
				authorize = nil
			}
			client := &http.Client{Transport: retryTransportFunc(func(req *http.Request) (*http.Response, error) {
				writes++
				if req.Method != "POST" || req.URL.String() != credentials.Endpoint || req.Header.Get("Authorization") != credentials.Token {
					t.Error("incorrect request")
				}
				if _, ok := req.Context().Deadline(); !ok {
					t.Error("missing deadline")
				}
				if mode == "transport_error" {
					return nil, errors.New("PRIVATE credential")
				}
				body := valid
				if mode == "oversized" {
					body = strings.Repeat("x", 65537)
				}
				if mode == "graphql_error" {
					body = `{"errors":[{"message":"PRIVATE credential"}]}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			result := sendLinearTicket(context.Background(), credentials, id, "Finding", "Link only", authorize, client)
			want := "unknown"
			if mode == "ack" {
				want = "created"
			}
			if mode == "invalid_endpoint" || mode == "missing_authorization" || mode == "revoked" {
				want = "failed"
			}
			if result.State != want || strings.Contains(result.Code, "PRIVATE") {
				t.Fatal(result, want)
			}
			if want == "failed" && writes != 0 || want != "failed" && writes != 1 {
				t.Fatal("incorrect creation count", writes)
			}
			if mode == "post_revoked" && result.RemoteID != id {
				t.Fatal("accepted receipt lost", result)
			}
		})
	}
}
