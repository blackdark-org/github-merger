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
	DefaultMergeMethod string   `yaml:"default_merge_method"`
	Labels             struct {
		Require []string `yaml:"require"`
		Block   []string `yaml:"block"`
		Squash  string   `yaml:"squash"`
	} `yaml:"labels"`
}

func loadConfig(path string) (poll.Options, time.Duration, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return poll.Options{}, 0, err
	}
	var file fileConfig
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return poll.Options{}, 0, fmt.Errorf("parse config: %w", err)
	}
	interval, err := parseDuration(file.Interval, time.Minute)
	if err != nil {
		return poll.Options{}, 0, fmt.Errorf("interval: %w", err)
	}
	if interval <= 0 {
		return poll.Options{}, 0, fmt.Errorf("interval must be positive")
	}
	settle, err := parseDuration(file.Settle, time.Minute)
	if err != nil {
		return poll.Options{}, 0, fmt.Errorf("settle: %w", err)
	}
	if settle < 0 {
		return poll.Options{}, 0, fmt.Errorf("settle must not be negative")
	}
	noChecksAfter, err := parseDuration(file.NoChecksAfter, 0)
	if err != nil {
		return poll.Options{}, 0, fmt.Errorf("merge_without_checks_after: %w", err)
	}
	if noChecksAfter < 0 {
		return poll.Options{}, 0, fmt.Errorf("merge_without_checks_after must not be negative")
	}
	method := decide.Method(file.DefaultMergeMethod)
	if method == "" {
		method = decide.MethodMerge
	}
	if method != decide.MethodMerge && method != decide.MethodSquash {
		return poll.Options{}, 0, fmt.Errorf("default_merge_method must be merge or squash")
	}
	require := file.Labels.Require
	if require == nil {
		require = []string{"automerge"}
	}
	squash := file.Labels.Squash
	if squash == "" {
		squash = "squash"
	}
	repos := make([]poll.Repo, 0, len(file.Repos))
	for _, name := range file.Repos {
		owner, repo, ok := strings.Cut(name, "/")
		if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
			return poll.Options{}, 0, fmt.Errorf("repo %q must be owner/name", name)
		}
		repos = append(repos, poll.Repo{Owner: owner, Name: repo})
	}
	if len(repos) == 0 {
		return poll.Options{}, 0, fmt.Errorf("repos is empty")
	}
	return poll.Options{
		Repos: repos,
		Decide: decide.Config{
			Settle:             settle,
			NoChecksAfter:      noChecksAfter,
			DefaultMergeMethod: method,
			Require:            require,
			Block:              file.Labels.Block,
			SquashLabel:        squash,
		},
	}, interval, nil
}

func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	return time.ParseDuration(raw)
}
