package main

import (
	"strings"
	"testing"

	"github.com/BlackDark/github-merger/internal/github"
	"github.com/BlackDark/github-merger/internal/poll"
)

func TestResolveReposExplicitOnly(t *testing.T) {
	installed := []github.InstallationRepository{
		{Owner: "acme", Name: "app"},
		{Owner: "acme", Name: "web"},
	}
	got, err := resolveRepos(installed, []poll.Repo{{Owner: "acme", Name: "app"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, got, "acme/app")
}

func TestResolveReposMissingExplicit(t *testing.T) {
	installed := []github.InstallationRepository{{Owner: "acme", Name: "app"}}
	_, err := resolveRepos(installed, []poll.Repo{{Owner: "acme", Name: "missing"}}, "")
	if err == nil || !strings.Contains(err.Error(), "acme/missing") {
		t.Fatalf("err = %v, want missing repo", err)
	}
}

func TestResolveReposOrg(t *testing.T) {
	installed := []github.InstallationRepository{
		{Owner: "acme", Name: "web"},
		{Owner: "acme", Name: "app"},
		{Owner: "acme", Name: "old", Archived: true},
		{Owner: "beta", Name: "tool"},
	}
	got, err := resolveRepos(installed, nil, "acme")
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, got, "acme/app", "acme/web")
}

func TestResolveReposOrgSkipsExplicit(t *testing.T) {
	installed := []github.InstallationRepository{
		{Owner: "acme", Name: "app"},
		{Owner: "acme", Name: "web"},
	}
	got, err := resolveRepos(installed, []poll.Repo{{Owner: "ACME", Name: "App"}}, "acme")
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, got, "ACME/App", "acme/web")
}

func TestResolveReposDeduplicatesExplicit(t *testing.T) {
	installed := []github.InstallationRepository{{Owner: "acme", Name: "app"}}
	explicit := []poll.Repo{{Owner: "acme", Name: "app"}, {Owner: "acme", Name: "app"}}
	got, err := resolveRepos(installed, explicit, "")
	if err != nil {
		t.Fatal(err)
	}
	assertRepos(t, got, "acme/app")
}

func TestResolveReposOrgWithNoRepositories(t *testing.T) {
	installed := []github.InstallationRepository{
		{Owner: "acme", Name: "old", Archived: true},
		{Owner: "beta", Name: "tool"},
	}
	_, err := resolveRepos(installed, nil, "acme")
	if err == nil || !strings.Contains(err.Error(), "acme") {
		t.Fatalf("err = %v, want empty org error", err)
	}
}

func assertRepos(t *testing.T, got []poll.Repo, want ...string) {
	t.Helper()
	names := make([]string, 0, len(got))
	for _, repo := range got {
		names = append(names, repo.Owner+"/"+repo.Name)
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("repos = %v, want %v", names, want)
	}
}
