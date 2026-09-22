package decide

import "time"

type Method string

const (
	MethodMerge  Method = "merge"
	MethodSquash Method = "squash"
)

type Config struct {
	Settle             time.Duration
	DefaultMergeMethod Method
	Require            []string
	Block              []string
	SquashLabel        string
}

type CheckRun struct {
	ID          int64
	Name        string
	Status      string
	Conclusion  string
	CompletedAt time.Time
}

type CheckSuite struct {
	Status    string
	CreatedAt time.Time
}

type Snapshot struct {
	Draft          bool
	Labels         []string
	HeadSHA        string
	HeadOwner      string
	HeadName       string
	BaseOwner      string
	BaseName       string
	Mergeable      *bool
	MergeableState string
	Suites         []CheckSuite
	Runs           []CheckRun
	StatusTotal    int
	StatusState    string
	Now            time.Time
}

type Decision struct {
	Merge  bool
	Method Method
	Reason string
}

func Decide(cfg Config, snap Snapshot) Decision {
	switch {
	case snap.Draft:
		return skip("draft")
	case snap.HeadOwner == "" || snap.HeadName == "":
		return skip("fork")
	case snap.HeadOwner != snap.BaseOwner || snap.HeadName != snap.BaseName:
		return skip("fork")
	case hasAny(snap.Labels, cfg.Block):
		return skip("blocked-label")
	case !hasAll(snap.Labels, cfg.Require):
		return skip("missing-label")
	case snap.Mergeable == nil:
		return skip("unknown")
	case !*snap.Mergeable || snap.MergeableState == "dirty":
		return skip("conflict")
	case len(snap.Suites) == 0:
		return skip("no-suites")
	case !suitesCompleted(snap.Suites):
		return skip("suites-pending")
	case !settled(cfg.Settle, snap.Now, snap.Suites):
		return skip("settling")
	}

	kept := latestRuns(snap.Runs)
	if len(kept) == 0 {
		return skip("no-checks")
	}
	for _, run := range kept {
		if run.Status != "completed" {
			return skip("checks-pending")
		}
		if run.Conclusion != "success" && run.Conclusion != "skipped" {
			return skip("checks-failed")
		}
	}
	if snap.StatusTotal > 0 && snap.StatusState != "success" {
		return skip("status")
	}

	method := cfg.DefaultMergeMethod
	if method == "" {
		method = MethodMerge
	}
	if cfg.SquashLabel != "" && hasLabel(snap.Labels, cfg.SquashLabel) {
		method = MethodSquash
	}
	return Decision{Merge: true, Method: method, Reason: "ready"}
}

func skip(reason string) Decision {
	return Decision{Reason: reason}
}

func suitesCompleted(suites []CheckSuite) bool {
	for _, suite := range suites {
		if suite.Status != "completed" {
			return false
		}
	}
	return true
}

func settled(settle time.Duration, now time.Time, suites []CheckSuite) bool {
	var newest time.Time
	for _, suite := range suites {
		if suite.CreatedAt.After(newest) {
			newest = suite.CreatedAt
		}
	}
	if newest.IsZero() {
		return false
	}
	return now.Sub(newest) >= settle
}

func latestRuns(runs []CheckRun) []CheckRun {
	best := make(map[string]CheckRun)
	for _, run := range runs {
		if run.Name == "" {
			continue
		}
		prev, ok := best[run.Name]
		if !ok || run.ID > prev.ID {
			best[run.Name] = run
		}
	}
	out := make([]CheckRun, 0, len(best))
	for _, run := range best {
		out = append(out, run)
	}
	return out
}

func hasAll(labels, want []string) bool {
	for _, name := range want {
		if !hasLabel(labels, name) {
			return false
		}
	}
	return true
}

func hasAny(labels, want []string) bool {
	for _, name := range want {
		if hasLabel(labels, name) {
			return true
		}
	}
	return false
}

func hasLabel(labels []string, name string) bool {
	for _, label := range labels {
		if label == name {
			return true
		}
	}
	return false
}
