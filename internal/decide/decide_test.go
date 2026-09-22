package decide

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		mutate func(cfg *Config, snap *Snapshot)
		merge  bool
		method Method
		reason string
	}{
		{
			name: "missing required label",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Labels = nil
			},
			reason: "missing-label",
		},
		{
			name: "blocked label",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Labels = []string{"automerge", "wip"}
			},
			reason: "blocked-label",
		},
		{
			name: "draft",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Draft = true
			},
			reason: "draft",
		},
		{
			name: "fork",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.HeadOwner = "other"
			},
			reason: "fork",
		},
		{
			name: "null head repo",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.HeadOwner = ""
				snap.HeadName = ""
			},
			reason: "fork",
		},
		{
			name: "squash label",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Labels = []string{"automerge", "squash"}
			},
			merge:  true,
			method: MethodSquash,
			reason: "ready",
		},
		{
			name:   "no squash label",
			merge:  true,
			method: MethodMerge,
			reason: "ready",
		},
		{
			name: "mergeable false",
			mutate: func(_ *Config, snap *Snapshot) {
				f := false
				snap.Mergeable = &f
			},
			reason: "conflict",
		},
		{
			name: "mergeable state dirty",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.MergeableState = "dirty"
			},
			reason: "conflict",
		},
		{
			name: "mergeable null",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Mergeable = nil
			},
			reason: "unknown",
		},
		{
			name: "zero suites",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Suites = nil
			},
			reason: "no-suites",
		},
		{
			name: "suite not completed",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Suites[0].Status = "queued"
			},
			reason: "suites-pending",
		},
		{
			name: "newest suite inside settle",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Suites[0].CreatedAt = now.Add(-30 * time.Second)
			},
			reason: "settling",
		},
		{
			name: "zero check runs",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Runs = nil
			},
			reason: "no-checks",
		},
		{
			name: "check not completed",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Runs[0].Status = "in_progress"
				snap.Runs[0].Conclusion = ""
			},
			reason: "checks-pending",
		},
		{
			name: "failed conclusion",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Runs[0].Conclusion = "failure"
			},
			reason: "checks-failed",
		},
		{
			name: "cancelled run loses to higher id success",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Runs = []CheckRun{
					{ID: 1, Name: "lint", Status: "completed", Conclusion: "cancelled", CompletedAt: now},
					{ID: 2, Name: "lint", Status: "completed", Conclusion: "success", CompletedAt: now.Add(-time.Minute)},
				}
			},
			merge:  true,
			method: MethodMerge,
			reason: "ready",
		},
		{
			name: "skipped check",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.Runs[0].Conclusion = "skipped"
			},
			merge:  true,
			method: MethodMerge,
			reason: "ready",
		},
		{
			name:   "combined status absent",
			merge:  true,
			method: MethodMerge,
			reason: "ready",
		},
		{
			name: "combined status failure",
			mutate: func(_ *Config, snap *Snapshot) {
				snap.StatusTotal = 2
				snap.StatusState = "failure"
			},
			reason: "status",
		},
		{
			name:   "happy path",
			merge:  true,
			method: MethodMerge,
			reason: "ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := readyConfig()
			snap := readySnapshot(now)
			if tt.mutate != nil {
				tt.mutate(&cfg, &snap)
			}
			got := Decide(cfg, snap)
			if got.Merge != tt.merge || got.Method != tt.method || got.Reason != tt.reason {
				t.Fatalf("Decide() = %+v, want merge=%v method=%q reason=%q", got, tt.merge, tt.method, tt.reason)
			}
		})
	}
}

func readyConfig() Config {
	return Config{
		Settle:             60 * time.Second,
		DefaultMergeMethod: MethodMerge,
		Require:            []string{"automerge"},
		Block:              []string{"wip"},
		SquashLabel:        "squash",
	}
}

func readySnapshot(now time.Time) Snapshot {
	mergeable := true
	return Snapshot{
		Labels:         []string{"automerge"},
		HeadSHA:        "abc",
		HeadOwner:      "acme",
		HeadName:       "app",
		BaseOwner:      "acme",
		BaseName:       "app",
		Mergeable:      &mergeable,
		MergeableState: "clean",
		Suites: []CheckSuite{{
			Status:    "completed",
			CreatedAt: now.Add(-2 * time.Minute),
		}},
		Runs: []CheckRun{{
			ID:          2,
			Name:        "lint",
			Status:      "completed",
			Conclusion:  "success",
			CompletedAt: now.Add(-time.Minute),
		}},
		StatusTotal: 0,
		StatusState: "pending",
		Now:         now,
	}
}
