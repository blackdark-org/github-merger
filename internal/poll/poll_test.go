package poll

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BlackDark/github-merger/internal/decide"
	"github.com/BlackDark/github-merger/internal/github"
)

func TestTick(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Minute).Format(time.RFC3339)
	var merges []mergeCall

	_, pemBytes := githubTestKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			writeJSON(w, map[string]string{
				"token":      "install-token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls":
			writeJSON(w, []map[string]int{{"number": 1}, {"number": 2}, {"number": 3}})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/1":
			writeJSON(w, pullJSON("aaa", true, "clean"))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/2":
			writeJSON(w, pullJSON("bbb", false, "dirty"))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/3":
			writeJSON(w, pullJSON("ccc", true, "clean"))
		case strings.HasSuffix(r.URL.Path, "/check-suites"):
			status := "completed"
			if strings.Contains(r.URL.Path, "/ccc/") {
				status = "queued"
			}
			writeJSON(w, map[string]any{
				"check_suites": []map[string]string{{"status": status, "created_at": old}},
			})
		case strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(w, map[string]any{
				"check_runs": []map[string]any{{
					"id": 10, "name": "lint", "status": "completed", "conclusion": "success",
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(w, map[string]any{"state": "pending", "total_count": 0})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/merge"):
			var body mergeCall
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode merge: %v", err)
			}
			body.Path = r.URL.Path
			merges = append(merges, body)
			writeJSON(w, map[string]any{"merged": true})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := github.New(srv.URL, "99", "7", pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	err = Tick(t.Context(), slog.New(slog.DiscardHandler), client, Options{
		Repos: []Repo{{Owner: "acme", Name: "app"}},
		Decide: decide.Config{
			Settle:             time.Minute,
			DefaultMergeMethod: decide.MethodMerge,
			Require:            []string{"automerge"},
			SquashLabel:        "squash",
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(merges) != 1 {
		t.Fatalf("merges = %+v, want 1", merges)
	}
	if merges[0].Path != "/repos/acme/app/pulls/1/merge" || merges[0].MergeMethod != "merge" || merges[0].SHA != "aaa" {
		t.Fatalf("merge = %+v", merges[0])
	}
}

type mergeCall struct {
	Path        string `json:"-"`
	MergeMethod string `json:"merge_method"`
	SHA         string `json:"sha"`
}

func pullJSON(sha string, mergeable bool, state string) map[string]any {
	return map[string]any{
		"draft":  false,
		"labels": []map[string]string{{"name": "automerge"}},
		"head": map[string]any{
			"sha": sha,
			"repo": map[string]any{
				"name":  "app",
				"owner": map[string]string{"login": "acme"},
			},
		},
		"base": map[string]any{
			"repo": map[string]any{
				"name":  "app",
				"owner": map[string]string{"login": "acme"},
			},
		},
		"mergeable":       mergeable,
		"mergeable_state": state,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
