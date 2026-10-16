//go:build ignore

// Isolated Git protocol experiment. No production transport is imported.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

func need(ok bool, message string) {
	if !ok {
		fmt.Fprintln(os.Stderr, "URI probe assertion:", message)
		panic("probe failed")
	}
}

func isolatedEnv(dir string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "LANG=C.UTF-8"}
}

func command(dir, input string, extra []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defaults := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "protocol.allow=never", "-c", "protocol.http.allow=always", "-c", "protocol.https.allow=always", "-c", "protocol.version=2", "-c", "http.followRedirects=false", "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
	cmd := exec.CommandContext(ctx, "git", append(defaults, args...)...)
	cmd.Dir = dir
	cmd.Env = append(isolatedEnv(dir), extra...)
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func mustGit(dir string, args ...string) string {
	out, err := command(dir, "", nil, args...)
	need(err == nil, "synthetic/local Git command")
	return out
}

func packet(s string) []byte { return []byte(fmt.Sprintf("%04x%s", len(s)+4, s)) }

func run() map[string]any {
	root, err := os.MkdirTemp("", "aim-git-uri-probe-*")
	need(err == nil, "temporary fixture")
	defer os.RemoveAll(root)
	seed, exported := filepath.Join(root, "seed"), filepath.Join(root, "upstream")
	need(os.Mkdir(seed, 0700) == nil && os.Mkdir(exported, 0700) == nil, "fixture directories")
	bare := filepath.Join(exported, "fixture.git")
	mustGit(seed, "init", "--template=", seed)
	mustGit(seed, "config", "user.name", "Fixture")
	mustGit(seed, "config", "user.email", "fixture@example.invalid")
	need(os.WriteFile(filepath.Join(seed, "evidence.txt"), []byte("synthetic-uri-evidence\n"), 0600) == nil, "fixture blob")
	mustGit(seed, "add", "--", "evidence.txt")
	mustGit(seed, "commit", "-m", "synthetic")
	sha := mustGit(seed, "rev-parse", "HEAD")
	blob := mustGit(seed, "rev-parse", "HEAD:evidence.txt")
	mustGit(seed, "init", "--bare", "--template=", bare)
	mustGit(seed, "-c", "protocol.file.allow=always", "push", bare, "HEAD:refs/heads/main")
	mustGit(bare, "config", "http.getanyfile", "false")
	mustGit(bare, "config", "http.receivepack", "false")
	mustGit(bare, "config", "uploadpack.allowsidebandall", "true")
	packPrefix := filepath.Join(root, "object")
	packHash, err := command(seed, blob+"\n", nil, "pack-objects", packPrefix)
	need(err == nil && len(packHash) == 40, "real isolated blob pack")
	pack, err := os.ReadFile(packPrefix + "-" + packHash + ".pack")
	need(err == nil, "pack bytes")
	bundlePath := filepath.Join(root, "objects.bundle")
	mustGit(seed, "bundle", "create", bundlePath, "--all")
	bundle, err := os.ReadFile(bundlePath)
	need(err == nil, "bundle bytes")
	var packRequests, bundleRequests, listRequests, uriAuth atomic.Int64
	var uriOrigin string
	uriServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Proxy-Authorization") != "" {
			uriAuth.Add(1)
		}
		switch r.URL.Path {
		case "/object.pack":
			packRequests.Add(1)
			w.Write(pack)
		case "/objects.bundle":
			bundleRequests.Add(1)
			w.Write(bundle)
		case "/bundle.list":
			listRequests.Add(1)
			fmt.Fprintf(w, "[bundle]\nversion = 1\nmode = all\nheuristic = creationToken\n[bundle \"fixture\"]\nuri = %s/objects.bundle\ncreationToken = 1\n", uriOrigin)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer uriServer.Close()
	uriOrigin = uriServer.URL
	mustGit(bare, "config", "uploadpack.blobpackfileuri", blob+" "+packHash+" "+uriOrigin+"/object.pack")
	backend := filepath.Join(mustGit(root, "--exec-path"), "git-http-backend")
	var advertised, unsolicited atomic.Bool
	var validURIResponse atomic.Value
	var wrongAuth, fetchCommands atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer SYNTHETIC-LOCAL-CAPABILITY" {
			wrongAuth.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
		r.Body.Close()
		if readErr != nil || len(body) > 1<<20 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if bytes.Contains(body, []byte("command=fetch")) {
			fetchCommands.Add(1)
			if unsolicited.Load() {
				w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
				// Replay a real, successfully consumed sideband/pack response,
				// despite this client not asking for packfile-uris.
				w.Write(validURIResponse.Load().([]byte))
				return
			}
		}
		r.Header.Del("Authorization")
		capture := httptest.NewRecorder()
		(&cgi.Handler{Path: backend, Root: "/", Dir: exported, Env: append(isolatedEnv(exported), "GIT_PROJECT_ROOT="+exported, "GIT_HTTP_EXPORT_ALL=1", "GIT_PROTOCOL="+r.Header.Get("Git-Protocol"))}).ServeHTTP(capture, r)
		if r.Method == http.MethodGet && bytes.Contains(capture.Body.Bytes(), []byte("packfile-uris")) {
			advertised.Store(true)
		}
		if bytes.Contains(body, []byte("packfile-uris http")) {
			validURIResponse.Store(append([]byte{}, capture.Body.Bytes()...))
		}
		for name, values := range capture.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(capture.Code)
		w.Write(capture.Body.Bytes())
	}))
	defer remote.Close()
	extra := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http." + remote.URL + "/.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Bearer SYNTHETIC-LOCAL-CAPABILITY"}
	fetch := func(configure bool, options ...string) error {
		dir, mkdirErr := os.MkdirTemp(root, "consumer-*")
		need(mkdirErr == nil, "consumer directory")
		mustGit(dir, "init", "--bare", "--template=", dir)
		if configure {
			mustGit(dir, "config", "fetch.uriprotocols", "http")
			mustGit(dir, "config", "fetch.bundleURI", uriOrigin+"/bundle.list")
		}
		args := append(append([]string{}, options...), "fetch", "--no-tags", "--no-recurse-submodules", remote.URL+"/fixture.git", sha)
		_, fetchErr := command(dir, "", extra, args...)
		if fetchErr == nil {
			need(mustGit(dir, "show", sha+":evidence.txt") == "synthetic-uri-evidence", "fixed object contents")
		}
		return fetchErr
	}
	// The server really advertises and can serve a pack outside the Git origin.
	need(fetch(false, "-c", "fetch.uriprotocols=http") == nil, "packfile URI positive control fetch")
	need(advertised.Load() && packRequests.Load() > 0, "positive control must actually reach alternate URI")
	positivePackRequests := packRequests.Load()
	need(fetch(false) == nil && packRequests.Load() == positivePackRequests, "default fetch ignores advertised packfile URI")
	// Independently prove configured fetch.bundleURI can reach a second origin.
	need(fetch(false, "-c", "fetch.bundleURI="+uriOrigin+"/bundle.list") == nil, "bundle URI positive control fetch")
	need(listRequests.Load() > 0 && bundleRequests.Load() > 0, "bundle positive control must download list and bundle")
	positiveListRequests, positiveBundleRequests := listRequests.Load(), bundleRequests.Load()
	// Command-level empty values must override a polluted local config.
	need(fetch(true, "-c", "fetch.uriprotocols=", "-c", "fetch.bundleURI=", "-c", "transfer.bundleURI=false") == nil, "explicit disabled URI fetch still reads complete content")
	need(packRequests.Load() == positivePackRequests && listRequests.Load() == positiveListRequests && bundleRequests.Load() == positiveBundleRequests, "explicit disable prevents all fixture URI downloads")
	unsolicited.Store(true)
	before := fetchCommands.Load()
	unsolicitedErr := fetch(false, "-c", "fetch.uriprotocols=", "-c", "fetch.bundleURI=", "-c", "transfer.bundleURI=false")
	need(fetchCommands.Load() > before, "unsolicited test actually consumes replayed server response")
	unsolicitedPackRequests := packRequests.Load() - positivePackRequests
	need(listRequests.Load() == positiveListRequests && bundleRequests.Load() == positiveBundleRequests, "replayed pack response did not introduce bundle downloads")
	// Force a forward proxy for both the normal Git HTTP route and helpers.
	// This is an experiment, not a production proxy or an OS sandbox.
	var forwarded, deniedHTTP, deniedCONNECT atomic.Int64
	proxyTransport := &http.Transport{Proxy: nil, DisableCompression: true}
	defer proxyTransport.CloseIdleConnections()
	proxyClient := &http.Client{Transport: proxyTransport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			deniedCONNECT.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !r.URL.IsAbs() || r.URL.Scheme != "http" || r.URL.Host != strings.TrimPrefix(remote.URL, "http://") {
			deniedHTTP.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		forwarded.Add(1)
		request := r.Clone(r.Context())
		request.RequestURI = ""
		request.Header.Del("Proxy-Authorization")
		request.Header.Del("Proxy-Connection")
		response, requestErr := proxyClient.Do(request)
		if requestErr != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	}))
	defer proxy.Close()
	disable := []string{"-c", "fetch.uriprotocols=", "-c", "fetch.bundleURI=", "-c", "transfer.bundleURI=false"}
	unsolicited.Store(false)
	need(fetch(true, append(append([]string{}, disable...), "-c", "http.proxy="+proxy.URL)...) == nil, "forced proxy preserves authorized fixed object fetch")
	unsolicited.Store(true)
	beforePack, beforeForward := packRequests.Load(), forwarded.Load()
	need(fetch(false, append(append([]string{}, disable...), "-c", "http.proxy="+proxy.URL)...) != nil, "proxy rejects unsolicited HTTP URI fetch")
	need(forwarded.Load() > beforeForward && deniedHTTP.Load() > 0 && packRequests.Load() == beforePack, "HTTP URI reaches proxy rejection rather than alternate origin")
	var tlsConnections, tlsPackRequests atomic.Int64
	tlsURI := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Proxy-Authorization") != "" {
			uriAuth.Add(1)
		}
		tlsPackRequests.Add(1)
		w.Write(pack)
	}))
	tlsURI.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			tlsConnections.Add(1)
		}
	}
	tlsURI.Config.ErrorLog = log.New(io.Discard, "", 0)
	tlsURI.StartTLS()
	defer tlsURI.Close()
	caPath := filepath.Join(root, "fixture-ca.pem")
	need(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsURI.Certificate().Raw}), 0600) == nil, "fixture CA")
	// Rewrite the complete URI packet, preserving its length and sideband byte.
	replay := validURIResponse.Load().([]byte)
	oldPacket := packet("\x01" + packHash + " " + uriOrigin + "/object.pack\n")
	newPacket := packet("\x01" + packHash + " " + tlsURI.URL + "/object.pack\n")
	need(bytes.Count(replay, oldPacket) == 1, "valid URI packet located exactly once")
	validURIResponse.Store(bytes.Replace(replay, oldPacket, newPacket, 1))
	// Trust only this temporary fixture CA: do not disguise TLS failures as denial.
	httpsErr := fetch(false, append(append([]string{}, disable...), "-c", "http.sslCAInfo="+caPath)...)
	if failed, ok := httpsErr.(*exec.ExitError); ok {
		message := strings.ToLower(string(failed.Stderr))
		fmt.Fprintf(os.Stderr, "HTTPS fixture classification: certificate=%v ca=%v ssl=%v protocol=%v tls_connections=%d pack_requests=%d\n", strings.Contains(message, "certificate"), strings.Contains(message, " ca"), strings.Contains(message, "ssl"), strings.Contains(message, "protocol"), tlsConnections.Load(), tlsPackRequests.Load())
	}
	need(httpsErr == nil, "trusted HTTPS URI positive control fetch")
	need(tlsConnections.Load() > 0 && tlsPackRequests.Load() > 0, "HTTPS positive control actually downloads alternate pack")
	beforeTLS, beforeTLSRequests := tlsConnections.Load(), tlsPackRequests.Load()
	need(fetch(false, append(append([]string{}, disable...), "-c", "http.sslCAInfo="+caPath, "-c", "http.proxy="+proxy.URL)...) != nil, "proxy rejects unsolicited HTTPS URI fetch")
	need(deniedCONNECT.Load() > 0 && tlsConnections.Load() == beforeTLS && tlsPackRequests.Load() == beforeTLSRequests, "HTTPS CONNECT rejected before any alternate TLS connection")
	need(wrongAuth.Load() == 0 && uriAuth.Load() == 0, "credentials scoped to Git origin only")
	return map[string]any{"synthetic_only": true, "git_version": mustGit(root, "--version"), "actual_packfile_uri_advertisement": advertised.Load(), "positive_packfile_uri_requests": positivePackRequests, "positive_bundle_list_requests": positiveListRequests, "positive_bundle_requests": positiveBundleRequests, "default_packfile_uri_zero_requests": true, "ordinary_server_config_override": true, "disabled_fetch_reads_fixed_content": true, "replayed_real_server_response": true, "unsolicited_fetch_succeeded": unsolicitedErr == nil, "unsolicited_packfile_uri_requests": unsolicitedPackRequests, "unsolicited_packfile_uri_bypasses_disable": unsolicitedPackRequests > 0, "forced_proxy_http_rejections": deniedHTTP.Load(), "forced_proxy_https_connect_rejections": deniedCONNECT.Load(), "forced_proxy_extra_http_requests": packRequests.Load() - beforePack, "forced_proxy_extra_tls_connections": tlsConnections.Load() - beforeTLS, "alternate_origin_authorization_headers": uriAuth.Load(), "production_adapter_tested": false}
}

func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "Git URI probe failed; raw provider/credential output omitted")
			os.Exit(1)
		}
	}()
	need(json.NewEncoder(os.Stdout).Encode(run()) == nil, "JSON proof")
}
