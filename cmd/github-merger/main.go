package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BlackDark/github-merger/internal/github"
	"github.com/BlackDark/github-merger/internal/poll"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("start", "version", version)
	if err := run(log); err != nil {
		log.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfgPath := os.Getenv("GITHUB_MERGER_CONFIG")
	if cfgPath == "" {
		return fmt.Errorf("GITHUB_MERGER_CONFIG is required")
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return err
	}
	appID := os.Getenv("GITHUB_APP_ID")
	installationID := os.Getenv("GITHUB_APP_INSTALLATION_ID")
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	if appID == "" || installationID == "" || keyPath == "" {
		return fmt.Errorf("GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, and GITHUB_APP_PRIVATE_KEY are required")
	}
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read private key: %w", err)
	}
	client, err := github.New("", appID, installationID, pemBytes)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	installed, err := client.InstallationRepos(ctx)
	if err != nil {
		return err
	}
	repos, err := resolveRepos(installed, cfg.Repos, cfg.Org)
	if err != nil {
		return err
	}
	log.Info("watch", "repos", len(repos))
	opt := poll.Options{Repos: repos, Decide: cfg.Decide}

	for {
		if err := poll.Tick(ctx, log, client, opt, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		timer := time.NewTimer(cfg.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
