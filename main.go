package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"pr_agent/internal/platform"
	"pr_agent/web"
)

func main() {
	location := flag.String("audit-repo", "", "Audit a local Git repository or HTTP(S) clone URL and emit JSON; no GitLab API required")
	base := flag.String("base", "", "Base ref (remote mode: full SHA)")
	head := flag.String("head", "", "Head ref (remote mode: full SHA)")
	flag.Parse()
	if *location != "" {
		cfg, e := platform.OpenSettings("config.yaml", "config.example.yaml")
		if e != nil {
			log.Fatal(e)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result, e := platform.AuditGit(ctx, *location, *base, *head, os.Getenv("AIM_GIT_TOKEN"), cfg.Snapshot())
		if e != nil && result.Status == "" {
			result.Status = "failed"
			result.Error = e.Error()
			result.Result = platform.AuditResult{Findings: []platform.Finding{}, Summary: "Audit failed before investigation", CoverageNotes: []string{}}
		}
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			log.Fatal(err)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, "Git audit failed:", e)
		}
		if code := auditExitCode(result, e); code != 0 {
			os.Exit(code)
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	settings, err := platform.OpenSettings("config.yaml", "config.example.yaml")
	if err != nil {
		return err
	}
	store, err := platform.OpenStore("pr_agent.db")
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = store.Bootstrap(ctx, os.Getenv("AIM_ADMIN_USERNAME"), os.Getenv("AIM_ADMIN_PASSWORD")); err != nil {
		return err
	}
	if err = store.ImportLegacy(ctx); err != nil {
		return err
	}
	importProjects, syncErr := store.RestorePendingProjects(ctx, settings)
	if syncErr != nil {
		log.Printf("project config synchronization pending; database changes retained")
	}
	cfg := settings.Snapshot()
	if !importProjects {
		cfg.Projects = nil
	}
	legacyProjects, err := store.ImportConfiguredProjects(ctx, cfg.Projects)
	if err != nil {
		return err
	}
	if err = store.ImportContextRepositories(ctx, legacyProjects); err != nil {
		return err
	}
	repo := &platform.DynamicRepository{Settings: settings}
	runner := &platform.Runner{Store: store, Repository: repo, Auditor: &platform.DynamicAuditor{Settings: settings, Repository: repo}, Settings: settings, Workers: cfg.AuditWorkers, Timeout: time.Duration(cfg.AuditTimeoutSeconds) * time.Second, Excluded: cfg.WhitelistExtensions}
	if err = runner.Start(ctx); err != nil {
		return err
	}
	defer runner.Stop()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if err = router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return err
	}
	api := &platform.HTTP{Store: store, Runner: runner, Settings: settings}
	api.Register(router)
	web.Register(router)
	server := newHTTPServer(cfg.Listen, router)
	serveErrors := make(chan error, 1)
	go func() { log.Printf("AIMergeBot listening on %s", cfg.Listen); serveErrors <- server.ListenAndServe() }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-runner.Failures():
		stop()
	case serveErr = <-serveErrors:
		stop()
		if serveErr == http.ErrServerClosed {
			serveErr = nil
		}
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdown); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	return serveErr
}
