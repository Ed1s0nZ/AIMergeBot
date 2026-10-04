package main

import (
	"context"
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
	cfg := settings.Snapshot()
	projects, err := store.Projects(ctx)
	if err != nil {
		return err
	}
	known := map[int]platform.Project{}
	for _, p := range projects {
		known[p.ID] = p
	}
	for _, p := range cfg.Projects {
		enabled := true
		if previous, ok := known[p.ID]; ok {
			enabled = previous.Enabled
		}
		if p.Enabled != nil {
			enabled = *p.Enabled
		}
		if err = store.SaveProject(ctx, platform.Project{ID: p.ID, Name: p.Name, Enabled: enabled}); err != nil {
			return err
		}
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
	if err = router.SetTrustedProxies(nil); err != nil {
		return err
	}
	api := &platform.HTTP{Store: store, Runner: runner, Settings: settings}
	api.Register(router)
	web.Register(router)
	server := &http.Server{Addr: cfg.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	serveErrors := make(chan error, 1)
	go func() { log.Printf("AIMergeBot listening on %s", cfg.Listen); serveErrors <- server.ListenAndServe() }()
	var serveErr error
	select {
	case <-ctx.Done():
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
