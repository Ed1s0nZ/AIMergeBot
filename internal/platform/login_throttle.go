package platform

import (
	"time"
)

const loginWindow = 5 * time.Minute
const maxLoginBuckets = 8192

type loginKey struct {
	Kind, IP, Username string
}

// admitLogin serializes admission, limits memory and counts concurrent requests
// before password verification. Call once for the IP and then for the account.
func (h *HTTP) admitLogin(ip, username string, account bool, now time.Time) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loginAttempts == nil {
		h.loginAttempts = make(map[loginKey]loginBucket)
	}
	for key, b := range h.loginAttempts {
		if !b.Reset.After(now) {
			delete(h.loginAttempts, key)
		}
	}
	keys := []loginKey{{Kind: "ip", IP: ip}}
	limits := []int{100}
	if account {
		keys = []loginKey{{Kind: "account", Username: username}, {Kind: "pair", IP: ip, Username: username}}
		limits = []int{30, 10}
	}
	missing := 0
	for _, key := range keys {
		if _, ok := h.loginAttempts[key]; !ok {
			missing++
		}
	}
	if len(h.loginAttempts)+missing > maxLoginBuckets {
		// Fail closed until an entry expires; do not evict active protection.
		wait := loginWindow
		for _, b := range h.loginAttempts {
			if remaining := b.Reset.Sub(now); remaining < wait {
				wait = remaining
			}
		}
		return wait
	}
	var wait time.Duration
	for i, key := range keys {
		b := h.loginAttempts[key]
		if b.Reset.IsZero() {
			b.Reset = now.Add(loginWindow)
		}
		// Saturation avoids overflow or unlimited growth from repeated rejections.
		if b.Count <= limits[i] {
			b.Count++
		}
		h.loginAttempts[key] = b
		if b.Count > limits[i] && b.Reset.Sub(now) > wait {
			wait = b.Reset.Sub(now)
		}
	}
	return wait
}

func (h *HTTP) loginSucceeded(ip, username string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.loginAttempts, loginKey{Kind: "pair", IP: ip, Username: username})
}
