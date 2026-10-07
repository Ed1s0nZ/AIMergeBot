package platform

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errNotificationTarget = errors.New("notification target rejected")

func allowedNotificationIP(ip net.IP, allow []string) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || !ip.IsGlobalUnicast() {
		return false
	}
	if !ip.IsPrivate() {
		return true
	}
	for _, value := range allow {
		_, block, err := net.ParseCIDR(value)
		if err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}
func notificationDial(ctx context.Context, address string, allow []string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errNotificationTarget
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errNotificationTarget
	}
	for _, ip := range ips {
		if !allowedNotificationIP(ip.IP, allow) {
			return nil, errNotificationTarget
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errNotificationTarget
}
func notificationHTTPClient(allow []string) *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errNotificationTarget }, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return notificationDial(ctx, address, allow)
	}}}
}
func sendHTTPNotification(ctx context.Context, kind string, c IntegrationCredentials, summary NotificationSummary, client *http.Client) (string, string) {
	if err := validateIntegrationCredentials(kind, c, true); err != nil {
		return "failed", "invalid_configuration"
	}
	payload, err := buildNotificationRequest(kind, summary, c.Secret, time.Now())
	if err != nil {
		return "failed", "invalid_configuration"
	}
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return "failed", "invalid_configuration"
	}
	query := endpoint.Query()
	for key, values := range payload.Query {
		query[key] = values
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload.Body))
	if err != nil {
		return "failed", "invalid_configuration"
	}
	for key, value := range payload.Headers {
		req.Header.Set(key, value)
	}
	if client == nil {
		client = notificationHTTPClient(c.AllowedNetworks)
	}
	res, err := client.Do(req)
	if err != nil {
		return "unknown", "transport_unknown"
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return "unknown", "transport_unknown"
	}
	return notificationResponse(kind, res.StatusCode, body)
}
func sendEmailNotification(ctx context.Context, c IntegrationCredentials, summary NotificationSummary) (string, string) {
	if err := validateIntegrationCredentials("email", c, true); err != nil {
		return "failed", "invalid_configuration"
	}
	if strings.ContainsAny(summary.Text, "\x00") || len(summary.Text) > 6000 {
		return "failed", "invalid_configuration"
	}
	if summary.URL != "" {
		u, err := url.Parse(summary.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || strings.ContainsAny(summary.URL, "\r\n") {
			return "failed", "invalid_configuration"
		}
	}
	conn, err := notificationDial(ctx, net.JoinHostPort(c.SMTPHost, strconv.Itoa(c.SMTPPort)), c.AllowedNetworks)
	if err != nil {
		return "unknown", "transport_unknown"
	}
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if v, ok := ctx.Deadline(); ok && v.Before(deadline) {
		deadline = v
	}
	conn.SetDeadline(deadline)
	rawConnection := conn
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			rawConnection.Close()
		case <-done:
		}
	}()
	tlsConfig := &tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12}
	if c.SMTPMode == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err = secure.HandshakeContext(ctx); err != nil {
			return "unknown", "transport_unknown"
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, c.SMTPHost)
	if err != nil {
		return "unknown", "transport_unknown"
	}
	defer client.Close()
	if c.SMTPMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return "failed", "invalid_configuration"
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return "unknown", "transport_unknown"
		}
	}
	if c.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.SMTPHost)); err != nil {
			return "failed", "provider_business_rejected"
		}
	}
	if err = client.Mail(c.From); err != nil {
		return "failed", "provider_business_rejected"
	}
	for _, to := range c.Recipients {
		if _, err = mail.ParseAddress(to); err != nil {
			return "failed", "invalid_configuration"
		}
		if err = client.Rcpt(to); err != nil {
			return "failed", "provider_business_rejected"
		}
	}
	writer, err := client.Data()
	if err != nil {
		return "unknown", "transport_unknown"
	}
	text := strings.ReplaceAll(strings.ReplaceAll(summary.Text, "\r\n", "\n"), "\n", "\r\n")
	if summary.URL != "" {
		text += "\r\n" + summary.URL
	}
	// Fixed headers; never put repository-controlled titles into SMTP headers.
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: AIMergeBot audit notification\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", c.From, strings.Join(c.Recipients, ", "), text)
	if err != nil {
		return "unknown", "transport_unknown"
	}
	if err = writer.Close(); err != nil {
		return "unknown", "transport_unknown"
	}
	return "accepted", ""
}
func (s *Store) DispatchNotification(ctx context.Context) (bool, error) {
	d, err := s.ClaimNotification(ctx)
	if err != nil {
		return false, err
	}
	integration, c, err := scanIntegration(s.DB.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, d.IntegrationID))
	if err != nil {
		return true, s.FinishNotification(ctx, d, "failed", "invalid_configuration")
	}
	if !integration.Enabled || integration.Revision != d.IntegrationRevision {
		return true, s.FinishNotification(ctx, d, "cancelled", "configuration_changed")
	}
	if err = s.requireNotificationAccess(ctx, d); err != nil {
		return true, s.FinishNotification(ctx, d, "cancelled", "permission_changed")
	}

	state, code := "failed", "unsupported_channel"
	if integration.Kind == "email" {
		state, code = sendEmailNotification(ctx, c, d.Payload)
	} else {
		state, code = sendHTTPNotification(ctx, integration.Kind, c, d.Payload, nil)
	}
	return true, s.FinishNotification(ctx, d, state, code)
}

func (r *Runner) notificationLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publicURL := ""
			if r.Settings != nil {
				publicURL = r.Settings.Snapshot().PublicURL
			}
			if err := r.Store.ExpireDispositions(ctx, time.Now()); err != nil {
				continue
			}
			if err := r.Store.CollectNotifications(ctx, publicURL, time.Now()); err == nil {
				_, _ = r.Store.DispatchNotification(ctx)
			}
		}
	}
}
