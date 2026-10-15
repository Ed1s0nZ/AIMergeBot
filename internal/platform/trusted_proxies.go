package platform

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
)

var ErrTrustedProxies = errors.New("invalid trusted proxy configuration")

func validateTrustedProxies(proxies []string) error {
	if len(proxies) > 32 {
		return fmt.Errorf("%w: at most 32 IP addresses or CIDRs", ErrTrustedProxies)
	}
	for _, raw := range proxies {
		if ip, err := netip.ParseAddr(raw); err == nil {
			if ip.Unmap().IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
				return fmt.Errorf("%w: explicit proxy addresses required", ErrTrustedProxies)
			}
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().IsMulticast() || prefix.Addr().Is4In6() {
			return fmt.Errorf("%w: IP addresses or non-universal CIDRs required", ErrTrustedProxies)
		}
	}
	return nil
}

func sameTrustedProxies(a, b []string) bool {
	return slices.Equal(a, b)
}

func (h *HTTP) settingsRequireRestart(cfg Settings) bool {
	if h.startupSettings == nil {
		return false
	}
	active := h.startupSettings
	return active.Listen != cfg.Listen || active.AuditWorkers != cfg.AuditWorkers || !sameTrustedProxies(active.TrustedProxies, cfg.TrustedProxies)
}

func (h *HTTP) publicSettingsWithContext(ctx context.Context) map[string]any {
	cfg := h.Settings.Snapshot()
	public := publicSettingsSnapshot(cfg)
	public["restart_required"] = h.settingsRequireRestart(cfg)
	if status := h.Settings.repositoryActivationStatus(ctx, cfg.Revision); status != nil {
		public["repository_activation"] = status
	}
	return public
}
