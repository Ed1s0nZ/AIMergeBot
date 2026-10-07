package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Verify the untouched request bytes before parsing form/JSON content. This
// establishes provider authenticity only; binding, replay consumption and
// current project authorization are separate mandatory callback gates.
func verifySlackCallback(secret, timestamp, signature string, body []byte, now time.Time) bool {
	if secret == "" || len(secret) > 4096 || len(body) > 65536 || len(timestamp) == 0 || len(timestamp) > 20 || len(signature) != 67 || !strings.HasPrefix(signature, "v0=") {
		return false
	}
	for _, c := range timestamp {
		if c < '0' || c > '9' {
			return false
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+300 {
		return false
	}
	given, err := hex.DecodeString(strings.TrimPrefix(signature, "v0="))
	if err != nil || len(given) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	return hmac.Equal(given, mac.Sum(nil))
}
