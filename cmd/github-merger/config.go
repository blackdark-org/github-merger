package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/BlackDark/github-merger/internal/decide"
	"github.com/BlackDark/github-merger/internal/poll"
)

type fileConfig struct {
	Interval           string   `yaml:"interval"`
	Settle             string   `yaml:"settle"`
	NoChecksAfter      string   `yaml:"merge_without_checks_after"`
	Repos              []string `yaml:"repos"`
	Org                string   `yaml:"org"`
	DefaultMergeMethod string   `yaml:"default_merge_method"`
	Labels             struct {
		Require []string `yaml:"require"`
		Block   []string `yaml:"block"`
		Squash  string   `yaml:"squash"`
	} `yaml:"labels"`
}

// config is the validated configuration. An org selects repositories at startup;
// repos adds specific owner/name entries on top of the org.
type config struct {
	Interval time.Duration
	Repos    []poll.Repo
	Org      string
	Decide   decide.Config
}

func loadConfig(path string) (config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var file fileConfig
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return config{}, fmt.Errorf("parse config: %w", err)
	}
	interval, err := parseDuration(file.Interval, time.Minute)
	if err != nil {
		return config{}, fmt.Errorf("interval: %w", err)
	}
	if interval <= 0 {
		return config{}, fmt.Errorf("interval must be positive")
	}
	settle, err := parseDuration(file.Settle, time.Minute)
	if err != nil {
		return config{}, fmt.Errorf("settle: %w", err)
	}
	if settle < 0 {
		return config{}, fmt.Errorf("settle must not be negative")
	}
	noChecksAfter, err := parseDuration(file.NoChecksAfter, 0)
	if err != nil {
		return config{}, fmt.Errorf("merge_without_checks_after: %w", err)
	}
	if noChecksAfter < 0 {
		return config{}, fmt.Errorf("merge_without_checks_after must not be negative")
	}
	method := decide.Method(file.DefaultMergeMethod)
	if method == "" {
		method = decide.MethodMerge
	}
	if method != decide.MethodMerge && method != decide.MethodSquash {
		return config{}, fmt.Errorf("default_merge_method must be merge or squash")
	}
	require := file.Labels.Require
	if len(require) == 0 {
		require = []string{"automerge"}
	}
	squash := file.Labels.Squash
	if squash == "" {
		squash = "squash"
	}
	repos := make([]poll.Repo, 0, len(file.Repos))
	for _, name := range file.Repos {
		name = strings.TrimSpace(name)
		owner, repo, ok := strings.Cut(name, "/")
		if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
			return config{}, fmt.Errorf("repo %q must be owner/name", name)
		}
		repos = append(repos, poll.Repo{Owner: owner, Name: repo})
	}
	org := strings.TrimSpace(file.Org)
	if strings.Contains(org, "/") {
		return config{}, fmt.Errorf("org %q must be an owner name", org)
	}
	if len(repos) == 0 && org == "" {
		return config{}, fmt.Errorf("set repos or org")
	}
	return config{
		Interval: interval,
		Repos:    repos,
		Org:      org,
		Decide: decide.Config{
			Settle:             settle,
			NoChecksAfter:      noChecksAfter,
			DefaultMergeMethod: method,
			Require:            require,
			Block:              file.Labels.Block,
			SquashLabel:        squash,
		},
	}, nil
}

func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return time.ParseDuration(raw)
}
