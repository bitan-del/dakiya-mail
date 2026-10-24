package jobs

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/bitan-del/dakiya-mail/internal/app/postmaster"
)

// PostmasterSyncJob refreshes Gmail Postmaster Tools stats for every workspace's sending domains.
type PostmasterSyncJob struct {
	svc postmaster.Service
}

func NewPostmasterSyncJob(svc postmaster.Service) *PostmasterSyncJob {
	return &PostmasterSyncJob{svc: svc}
}

// Run performs one pass. A no-op until Google credentials are connected.
func (j *PostmasterSyncJob) Run(ctx context.Context) error {
	if j.svc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := j.svc.Sync(ctx, postmaster.SyncDays); err != nil {
		sentry.CaptureException(err)
		return err
	}
	return nil
}

// Start runs once on boot and then on the interval until ctx ends.
// ponytail: every backend replica syncs, the upsert is idempotent; add a lock if Google quota ever bites.
func (j *PostmasterSyncJob) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	_ = j.Run(ctx)
	for {
		select {
		case <-ticker.C:
			_ = j.Run(ctx)
		case <-ctx.Done():
			return
		}
	}
}
