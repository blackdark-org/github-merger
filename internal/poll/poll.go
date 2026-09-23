package poll

import (
	"context"
	"log/slog"
	"time"

	"github.com/BlackDark/github-merger/internal/decide"
)

type Repo struct {
	Owner string
	Name  string
}

type Options struct {
	Repos  []Repo
	Decide decide.Config
}

type Source interface {
	OpenPRs(ctx context.Context, owner, repo string) ([]decide.PullRequest, error)
	Snapshot(ctx context.Context, owner, repo string, number int) (decide.Snapshot, error)
	Merge(ctx context.Context, owner, repo string, number int, method decide.Method, sha string) error
}

func Tick(ctx context.Context, log *slog.Logger, src Source, opt Options, now time.Time) error {
	if log == nil {
		log = slog.Default()
	}
	for _, repo := range opt.Repos {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := repo.Owner + "/" + repo.Name
		prs, err := src.OpenPRs(ctx, repo.Owner, repo.Name)
		if err != nil {
			log.Error("list pull requests", "repo", name, "err", err)
			continue
		}
		for _, pr := range prs {
			if err := ctx.Err(); err != nil {
				return err
			}
			number := pr.Number
			if reason := decide.Candidate(opt.Decide, pr); reason != "" {
				logSkip(log, name, number, reason)
				continue
			}
			snap, err := src.Snapshot(ctx, repo.Owner, repo.Name, number)
			if err != nil {
				log.Error("load pull request", "repo", name, "pr", number, "err", err)
				continue
			}
			snap.Now = now
			decision := decide.Decide(opt.Decide, snap)
			if !decision.Merge {
				logSkip(log, name, number, decision.Reason)
				continue
			}
			if err := src.Merge(ctx, repo.Owner, repo.Name, number, decision.Method, snap.HeadSHA); err != nil {
				log.Error("merge", "repo", name, "pr", number, "method", decision.Method, "err", err)
				continue
			}
			log.Info("merge", "repo", name, "pr", number, "method", decision.Method, "reason", decision.Reason)
		}
	}
	return nil
}

func logSkip(log *slog.Logger, repo string, number int, reason string) {
	level := slog.LevelInfo
	if reason == "missing-label" {
		level = slog.LevelDebug
	}
	log.Log(context.Background(), level, "skip", "repo", repo, "pr", number, "reason", reason)
}
