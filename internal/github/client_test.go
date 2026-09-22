package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/BlackDark/github-merger/internal/decide"
)

func TestInstallationToken(t *testing.T) {
	key, pemBytes := testKey(t)
	var tokenHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens"):
			tokenHits++
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			parsed, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(tok *jwt.Token) (any, error) {
				if tok.Method != jwt.SigningMethodRS256 {
					t.Errorf("alg %v", tok.Header["alg"])
				}
				return &key.PublicKey, nil
			})
			if err != nil {
				t.Errorf("parse jwt: %v", err)
			}
			claims := parsed.Claims.(*jwt.RegisteredClaims)
			if claims.Issuer != "99" {
				t.Errorf("iss %q", claims.Issuer)
			}
			age := time.Since(claims.IssuedAt.Time)
			if age < 50*time.Second || age > 70*time.Second {
				t.Errorf("iat age %s, want about 60s", age)
			}
			if claims.ExpiresAt == nil {
				t.Fatal("missing exp")
			}
			until := time.Until(claims.ExpiresAt.Time)
			if until <= 0 || until > 10*time.Minute {
				t.Errorf("exp in %s, want under 10m", until)
			}
			writeJSON(w, map[string]string{
				"token":      "install-token",
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
		case r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer install-token" {
				t.Errorf("repo auth %q", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("per_page %q", r.URL.Query().Get("per_page"))
			}
			writeJSON(w, map[string]any{
				"repositories": []map[string]string{{"full_name": "acme/app"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "99", "7", pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureRepos(t.Context(), []string{"acme/app"}); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureRepos(t.Context(), []string{"acme/missing"}); err == nil {
		t.Fatal("expected missing repo error")
	}
	if tokenHits != 1 {
		t.Fatalf("token requests = %d, want 1", tokenHits)
	}
}

func TestCheckRunPagination(t *testing.T) {
	_, pemBytes := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			writeToken(w)
			return
		}
		switch r.URL.Path {
		case "/repos/acme/app/pulls/1":
			writeJSON(w, map[string]any{
				"draft": false,
				"labels": []map[string]string{
					{"name": "automerge"},
				},
				"head": map[string]any{
					"sha": "abc",
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
				"mergeable":       true,
				"mergeable_state": "clean",
			})
		case "/repos/acme/app/commits/abc/check-suites":
			writeJSON(w, map[string]any{
				"check_suites": []map[string]any{{
					"id": 42, "status": "completed", "created_at": "2026-09-22T12:00:00Z",
				}},
			})
		case "/repos/acme/app/commits/abc/check-runs":
			if r.URL.Query().Get("filter") != "all" || r.URL.Query().Get("per_page") != "100" {
				t.Errorf("check run query %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("page") == "2" {
				writeJSON(w, map[string]any{
					"check_runs": []map[string]any{{
						"id": 2, "name": "test", "status": "completed", "conclusion": nil,
					}},
				})
				return
			}
			w.Header().Set("Link", `<http://`+r.Host+`/repos/acme/app/commits/abc/check-runs?filter=all&per_page=100&page=2>; rel="next"`)
			writeJSON(w, map[string]any{
				"check_runs": []map[string]any{{
					"id": 1, "name": "lint", "status": "completed", "conclusion": "success",
					"check_suite": map[string]any{"id": 42},
				}},
			})
		case "/repos/acme/app/commits/abc/status":
			writeJSON(w, map[string]any{"state": "pending", "total_count": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv.URL, pemBytes)
	snap, err := c.Snapshot(t.Context(), "acme", "app", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Runs) != 2 {
		t.Fatalf("runs = %+v, want 2", snap.Runs)
	}
	if snap.Runs[1].Conclusion != "" {
		t.Fatalf("null conclusion decoded as %q", snap.Runs[1].Conclusion)
	}
	if len(snap.Suites) != 1 || snap.Suites[0].ID != 42 || snap.Runs[0].SuiteID != 42 {
		t.Fatalf("suite link = suites %+v runs %+v", snap.Suites, snap.Runs)
	}
}

func TestSnapshotNullHeadRepo(t *testing.T) {
	_, pemBytes := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			writeToken(w)
			return
		}
		if r.URL.Path != "/repos/acme/app/pulls/4" {
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{
			"draft":           false,
			"head":            map[string]any{"sha": "", "repo": nil},
			"base":            map[string]any{"repo": map[string]any{"name": "app", "owner": map[string]string{"login": "acme"}}},
			"mergeable":       nil,
			"mergeable_state": "unknown",
		})
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv.URL, pemBytes)
	snap, err := c.Snapshot(t.Context(), "acme", "app", 4)
	if err != nil {
		t.Fatal(err)
	}
	if snap.HeadOwner != "" || snap.HeadName != "" || snap.Mergeable != nil {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestMergeBody(t *testing.T) {
	_, pemBytes := testKey(t)
	var got struct {
		MergeMethod string `json:"merge_method"`
		SHA         string `json:"sha"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			writeToken(w)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/repos/acme/app/pulls/8/merge" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer install-token" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writeJSON(w, map[string]any{"merged": true})
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv.URL, pemBytes)
	if err := c.Merge(t.Context(), "acme", "app", 8, decide.MethodSquash, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if got.MergeMethod != "squash" || got.SHA != "deadbeef" {
		t.Fatalf("body = %+v", got)
	}
}

func TestMergeErrors(t *testing.T) {
	_, pemBytes := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			writeToken(w)
			return
		}
		switch r.URL.Path {
		case "/repos/acme/app/pulls/1/merge":
			http.Error(w, `{"message":"head sha mismatch"}`, http.StatusConflict)
		case "/repos/acme/app/pulls/2/merge":
			http.Error(w, `{"message":"nope"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := newClient(t, srv.URL, pemBytes)
	err := c.Merge(t.Context(), "acme", "app", 1, decide.MethodMerge, "abc")
	var api *APIError
	if !errors.As(err, &api) || api.Status != http.StatusConflict {
		t.Fatalf("409 error = %v", err)
	}
	err = c.Merge(t.Context(), "acme", "app", 2, decide.MethodMerge, "abc")
	api = nil
	if !errors.As(err, &api) || api.Status != http.StatusInternalServerError {
		t.Fatalf("500 error = %v", err)
	}
}

func newClient(t *testing.T, base string, pemBytes []byte) *Client {
	t.Helper()
	c, err := New(base, "99", "7", pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeToken(w http.ResponseWriter) {
	writeJSON(w, map[string]string{
		"token":      "install-token",
		"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func testKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return key, raw
}
