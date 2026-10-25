package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BlackDark/github-merger/internal/decide"
)

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("repos:\n  - acme/app\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	opt, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if opt.Interval != time.Minute || opt.Decide.Settle != time.Minute || opt.Decide.NoChecksAfter != 0 {
		t.Fatalf("interval %s settle %s no-checks %s", opt.Interval, opt.Decide.Settle, opt.Decide.NoChecksAfter)
	}
	if opt.Decide.DefaultMergeMethod != decide.MethodMerge || opt.Decide.SquashLabel != "squash" {
		t.Fatalf("method config = %+v", opt.Decide)
	}
	if len(opt.Decide.Require) != 1 || opt.Decide.Require[0] != "automerge" {
		t.Fatalf("require = %v", opt.Decide.Require)
	}
	if len(opt.Repos) != 1 || opt.Repos[0].Owner != "acme" || opt.Repos[0].Name != "app" {
		t.Fatalf("repo = %+v", opt.Repos)
	}
	if opt.Org != "" {
		t.Fatalf("org = %q", opt.Org)
	}
}

func TestLoadConfigEmptyRequireDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("repos:\n  - acme/app\nlabels:\n  require: []\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	opt, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(opt.Decide.Require) != 1 || opt.Decide.Require[0] != "automerge" {
		t.Fatalf("require = %v, want [automerge]", opt.Decide.Require)
	}
}

func TestLoadConfigNoChecksAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("merge_without_checks_after: 10m\nrepos:\n  - acme/app\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	opt, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if opt.Decide.NoChecksAfter != 10*time.Minute {
		t.Fatalf("no-checks = %s", opt.Decide.NoChecksAfter)
	}
	if err := os.WriteFile(path, []byte("merge_without_checks_after: -1m\nrepos:\n  - acme/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected negative duration error")
	}
}

func TestLoadConfigRejectsBadRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("repos:\n  - noslash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigOrg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("org: acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Org != "acme" {
		t.Fatalf("org = %q", cfg.Org)
	}
	if len(cfg.Repos) != 0 {
		t.Fatalf("repos = %+v", cfg.Repos)
	}
}

func TestLoadConfigRejectsBadOrg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("org: acme/team\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRequiresReposOrOrg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("interval: 30s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected error")
	}
}
