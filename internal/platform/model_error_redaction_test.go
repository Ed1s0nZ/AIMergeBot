package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPermanentModelErrorsDoNotExposeProviderBodyOrRetry(t *testing.T) {
	for _, status := range []int{401, 403, 404, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"secret-provider-body sk-private-fixture-key","type":"invalid_request_error","code":"invalid_api_key"}}`)
			}))
			defer server.Close()
			auditor := &EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL + "/v1", Model: "synthetic-redaction"}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _, err := auditor.Audit(ctx, Snapshot{ProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
			if err == nil {
				t.Fatal("permanent upstream failure treated as success")
			}
			if strings.Contains(err.Error(), "secret-provider-body") || strings.Contains(err.Error(), "sk-private-fixture-key") {
				t.Fatal("provider content escaped into persisted error path")
			}
			info := UpstreamFailureInfo(err)
			if info == nil || info.Source != "model" || info.HTTPStatus != status || info.Kind != "upstream_client" {
				t.Fatal("normalized permanent status lost", info)
			}
			if retryableError(err) || calls.Load() != 1 {
				t.Fatal("permanent failure retried", calls.Load())
			}
		})
	}
}
