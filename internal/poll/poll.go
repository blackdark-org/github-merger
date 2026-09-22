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
	OpenPRNumbers(ctx context.Context, owner, repo string) ([]int, error)
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
		numbers, err := src.OpenPRNumbers(ctx, repo.Owner, repo.Name)
		if err != nil {
			log.Error("list pull requests", "repo", name, "err", err)
			continue
		}
		for _, number := range numbers {
			if err := ctx.Err(); err != nil {
				return err
			}
			snap, err := src.Snapshot(ctx, repo.Owner, repo.Name, number)
			if err != nil {
				log.Error("load pull request", "repo", name, "pr", number, "err", err)
				continue
			}
			snap.Now = now
			decision := decide.Decide(opt.Decide, snap)
			if !decision.Merge {
				log.Info("skip", "repo", name, "pr", number, "reason", decision.Reason)
				continue
			}
			if err := src.Merge(ctx, repo.Owner, repo.Name, number, decision.Method, snap.HeadSHA); err != nil {
				log.Error("merge", "repo", name, "pr", number, "method", decision.Method, "err", err)
				continue
			}
			log.Info("merge", "repo", name, "pr", number, "method", decision.Method)
		}
	}
	return nil
}
