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
	opt, interval, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if interval != time.Minute || opt.Decide.Settle != time.Minute || opt.Decide.NoChecksAfter != 0 {
		t.Fatalf("interval %s settle %s no-checks %s", interval, opt.Decide.Settle, opt.Decide.NoChecksAfter)
	}
	if opt.Decide.DefaultMergeMethod != decide.MethodMerge || opt.Decide.SquashLabel != "squash" {
		t.Fatalf("method config = %+v", opt.Decide)
	}
	if len(opt.Decide.Require) != 1 || opt.Decide.Require[0] != "automerge" {
		t.Fatalf("require = %v", opt.Decide.Require)
	}
	if opt.Repos[0].Owner != "acme" || opt.Repos[0].Name != "app" {
		t.Fatalf("repo = %+v", opt.Repos[0])
	}
}

func TestLoadConfigEmptyRequireDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("repos:\n  - acme/app\nlabels:\n  require: []\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	opt, _, err := loadConfig(path)
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
	opt, _, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if opt.Decide.NoChecksAfter != 10*time.Minute {
		t.Fatalf("no-checks = %s", opt.Decide.NoChecksAfter)
	}
	if err := os.WriteFile(path, []byte("merge_without_checks_after: -1m\nrepos:\n  - acme/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("expected negative duration error")
	}
}

func TestLoadConfigRejectsBadRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("repos:\n  - noslash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("expected error")
	}
}
