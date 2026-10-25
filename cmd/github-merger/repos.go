package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BlackDark/github-merger/internal/github"
	"github.com/BlackDark/github-merger/internal/poll"
)

// resolveRepos expands the org into the installation's repositories and adds
// explicit repos. Explicit repos must belong to the installation. Archived
// repositories are skipped for the org because GitHub rejects merges into them;
// an explicitly named repo stays as configured. The result is deduplicated by
// owner and name.
func resolveRepos(installed []github.InstallationRepository, explicit []poll.Repo, org string) ([]poll.Repo, error) {
	have := make(map[string]struct{}, len(installed))
	for _, repo := range installed {
		have[repoKey(repo.Owner, repo.Name)] = struct{}{}
	}
	var missing []string
	for _, repo := range explicit {
		if _, ok := have[repoKey(repo.Owner, repo.Name)]; !ok {
			missing = append(missing, repo.Owner+"/"+repo.Name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("installation missing repos: %s", strings.Join(missing, ", "))
	}

	repos := make([]poll.Repo, 0, len(explicit)+len(installed))
	seen := make(map[string]struct{}, len(explicit)+len(installed))
	for _, repo := range explicit {
		key := repoKey(repo.Owner, repo.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		repos = append(repos, repo)
	}
	if org == "" {
		return repos, nil
	}

	var found []poll.Repo
	for _, repo := range installed {
		if repo.Archived || !strings.EqualFold(repo.Owner, org) {
			continue
		}
		found = append(found, poll.Repo{Owner: repo.Owner, Name: repo.Name})
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("org %q has no repositories in the installation", org)
	}
	sort.Slice(found, func(i, j int) bool {
		return repoKey(found[i].Owner, found[i].Name) < repoKey(found[j].Owner, found[j].Name)
	})
	for _, repo := range found {
		key := repoKey(repo.Owner, repo.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		repos = append(repos, repo)
	}
	return repos, nil
}

func repoKey(owner, name string) string {
	return strings.ToLower(owner + "/" + name)
}
