package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSlackCallbackSignatureRejectsTamperingAndStaleRequests(t *testing.T) {
	now := time.Unix(1800000000, 0)
	secret := "fixture-signing-secret"
	body := []byte("team_id=T1&user_id=U1&text=run+1")
	sign := func(ts string, b []byte) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte("v0:" + ts + ":"))
		m.Write(b)
		return "v0=" + hex.EncodeToString(m.Sum(nil))
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := sign(ts, body)
	if !verifySlackCallback(secret, ts, sig, body, now) {
		t.Fatal("valid request rejected")
	}
	for _, delta := range []int64{-301, 301} {
		stamp := strconv.FormatInt(now.Unix()+delta, 10)
		if verifySlackCallback(secret, stamp, sign(stamp, body), body, now) {
			t.Fatal("stale/future accepted", delta)
		}
	}
	for _, test := range []struct {
		secret, stamp, signature string
		body                     []byte
	}{
		{"", ts, sig, body}, {"other", ts, sig, body}, {secret, ts, sig, []byte("team_id=T1&user_id=U2&text=run+1")},
		{secret, ts, sig, []byte("team_id=T1&user_id=U1&text=run%201")},
		{secret, "+" + ts, sig, body}, {secret, "99999999999999999999", sig, body}, {secret, ts, "v1=" + sig[3:], body},
		{secret, ts, "v0=" + strings.Repeat("z", 64), body}, {secret, ts, sig, []byte(strings.Repeat("x", 65537))},
	} {
		if verifySlackCallback(test.secret, test.stamp, test.signature, test.body, now) {
			t.Fatal("invalid request accepted")
		}
	}
}
