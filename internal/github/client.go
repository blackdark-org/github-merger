package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/BlackDark/github-merger/internal/decide"
)

const apiVersion = "2022-11-28"

type Client struct {
	base           string
	appID          string
	installationID string
	key            *rsa.PrivateKey
	http           *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api status %d: %s", e.Status, e.Body)
}

func New(baseURL, appID, installationID string, pemBytes []byte) (*Client, error) {
	if appID == "" || installationID == "" {
		return nil, fmt.Errorf("app id and installation id are required")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("parse app private key: %w", err)
	}
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return &Client{
		base:           base,
		appID:          appID,
		installationID: installationID,
		key:            key,
		http:           &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) EnsureRepos(ctx context.Context, want []string) error {
	got, err := c.installationRepos(ctx)
	if err != nil {
		return err
	}
	have := make(map[string]struct{}, len(got))
	for _, name := range got {
		have[name] = struct{}{}
	}
	var missing []string
	for _, name := range want {
		if _, ok := have[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("installation missing repos: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Client) OpenPRNumbers(ctx context.Context, owner, repo string) ([]int, error) {
	var nums []int
	next := c.repo(owner, repo) + "/pulls?state=open&per_page=100"
	for next != "" {
		var page []struct {
			Number int `json:"number"`
		}
		var err error
		next, err = c.get(ctx, next, &page)
		if err != nil {
			return nil, err
		}
		for _, pr := range page {
			nums = append(nums, pr.Number)
		}
	}
	return nums, nil
}

func (c *Client) Snapshot(ctx context.Context, owner, repo string, number int) (decide.Snapshot, error) {
	var pr pullPayload
	if _, err := c.get(ctx, fmt.Sprintf("%s/pulls/%d", c.repo(owner, repo), number), &pr); err != nil {
		return decide.Snapshot{}, err
	}
	snap := pr.toSnapshot()
	if snap.HeadSHA == "" {
		return snap, nil
	}
	suites, err := c.checkSuites(ctx, owner, repo, snap.HeadSHA)
	if err != nil {
		return decide.Snapshot{}, err
	}
	runs, err := c.checkRuns(ctx, owner, repo, snap.HeadSHA)
	if err != nil {
		return decide.Snapshot{}, err
	}
	total, state, err := c.combinedStatus(ctx, owner, repo, snap.HeadSHA)
	if err != nil {
		return decide.Snapshot{}, err
	}
	snap.Suites = suites
	snap.Runs = runs
	snap.StatusTotal = total
	snap.StatusState = state
	return snap, nil
}

func (c *Client) Merge(ctx context.Context, owner, repo string, number int, method decide.Method, sha string) error {
	body, err := json.Marshal(struct {
		MergeMethod string `json:"merge_method"`
		SHA         string `json:"sha"`
	}{MergeMethod: string(method), SHA: sha})
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/pulls/%d/merge", c.repo(owner, repo), number)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: redact(respBody)}
	}
	return nil
}

func (c *Client) installationRepos(ctx context.Context) ([]string, error) {
	var names []string
	next := c.base + "/installation/repositories?per_page=100"
	for next != "" {
		var page struct {
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		var err error
		next, err = c.get(ctx, next, &page)
		if err != nil {
			return nil, err
		}
		for _, repo := range page.Repositories {
			names = append(names, repo.FullName)
		}
	}
	return names, nil
}

func (c *Client) checkSuites(ctx context.Context, owner, repo, sha string) ([]decide.CheckSuite, error) {
	var suites []decide.CheckSuite
	next := c.commit(owner, repo, sha) + "/check-suites?per_page=100"
	for next != "" {
		var page struct {
			CheckSuites []struct {
				ID        int64     `json:"id"`
				Status    string    `json:"status"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"check_suites"`
		}
		var err error
		next, err = c.get(ctx, next, &page)
		if err != nil {
			return nil, err
		}
		for _, suite := range page.CheckSuites {
			suites = append(suites, decide.CheckSuite{ID: suite.ID, Status: suite.Status, CreatedAt: suite.CreatedAt})
		}
	}
	return suites, nil
}

func (c *Client) checkRuns(ctx context.Context, owner, repo, sha string) ([]decide.CheckRun, error) {
	var runs []decide.CheckRun
	next := c.commit(owner, repo, sha) + "/check-runs?filter=all&per_page=100"
	for next != "" {
		var page struct {
			CheckRuns []runPayload `json:"check_runs"`
		}
		var err error
		next, err = c.get(ctx, next, &page)
		if err != nil {
			return nil, err
		}
		for _, run := range page.CheckRuns {
			runs = append(runs, run.toRun())
		}
	}
	return runs, nil
}

func (c *Client) combinedStatus(ctx context.Context, owner, repo, sha string) (int, string, error) {
	var body struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	if _, err := c.get(ctx, c.commit(owner, repo, sha)+"/status", &body); err != nil {
		return 0, "", err
	}
	return body.TotalCount, body.State, nil
}

type pullPayload struct {
	Draft  bool `json:"draft"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Head struct {
		SHA  string    `json:"sha"`
		Repo *repoSide `json:"repo"`
	} `json:"head"`
	Base struct {
		Repo *repoSide `json:"repo"`
	} `json:"base"`
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
}

type repoSide struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (p pullPayload) toSnapshot() decide.Snapshot {
	snap := decide.Snapshot{
		Draft:          p.Draft,
		HeadSHA:        p.Head.SHA,
		Mergeable:      p.Mergeable,
		MergeableState: p.MergeableState,
	}
	for _, label := range p.Labels {
		snap.Labels = append(snap.Labels, label.Name)
	}
	if p.Head.Repo != nil {
		snap.HeadOwner = p.Head.Repo.Owner.Login
		snap.HeadName = p.Head.Repo.Name
	}
	if p.Base.Repo != nil {
		snap.BaseOwner = p.Base.Repo.Owner.Login
		snap.BaseName = p.Base.Repo.Name
	}
	return snap
}

type runPayload struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	CompletedAt *time.Time `json:"completed_at"`
	CheckSuite  struct {
		ID int64 `json:"id"`
	} `json:"check_suite"`
}

func (r runPayload) toRun() decide.CheckRun {
	run := decide.CheckRun{ID: r.ID, SuiteID: r.CheckSuite.ID, Name: r.Name, Status: r.Status}
	if r.Conclusion != nil {
		run.Conclusion = *r.Conclusion
	}
	if r.CompletedAt != nil {
		run.CompletedAt = *r.CompletedAt
	}
	return run
}

func (c *Client) repo(owner, repo string) string {
	return c.base + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func (c *Client) commit(owner, repo, sha string) string {
	return c.repo(owner, repo) + "/commits/" + url.PathEscape(sha)
}

func (c *Client) get(ctx context.Context, rawURL string, dest any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{Status: resp.StatusCode, Body: redact(body)}
	}
	if dest != nil && len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, dest); err != nil {
			return "", fmt.Errorf("decode %s: %w", req.URL.Path, err)
		}
	}
	return nextLink(resp.Header), nil
}

func (c *Client) do(ctx context.Context, req *http.Request, appJWT bool) (*http.Response, error) {
	var auth string
	if appJWT {
		signed, err := c.signJWT(time.Now())
		if err != nil {
			return nil, err
		}
		auth = "Bearer " + signed
	} else {
		tok, err := c.installationToken(ctx)
		if err != nil {
			return nil, err
		}
		auth = "Bearer " + tok
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", "github-merger")
	return c.http.Do(req)
}

func (c *Client) installationToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExp) > time.Minute {
		return c.token, nil
	}
	signed, err := c.signJWT(time.Now())
	if err != nil {
		return "", err
	}
	u := c.base + "/app/installations/" + url.PathEscape(c.installationID) + "/access_tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+signed)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", "github-merger")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &APIError{Status: resp.StatusCode, Body: redact(body)}
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode installation token")
	}
	if out.Token == "" {
		return "", fmt.Errorf("empty installation token")
	}
	if out.ExpiresAt.IsZero() {
		out.ExpiresAt = time.Now().Add(50 * time.Minute)
	}
	c.token = out.Token
	c.tokenExp = out.ExpiresAt
	return c.token, nil
}

func (c *Client) signJWT(now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    c.appID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(c.key)
}

func nextLink(h http.Header) string {
	for _, part := range strings.Split(h.Get("Link"), ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

func redact(body []byte) string {
	if bytes.Contains(body, []byte(`"token"`)) {
		return "(redacted)"
	}
	s := string(body)
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}
