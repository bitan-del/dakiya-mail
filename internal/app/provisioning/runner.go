package provisioning

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bitan-del/dakiya-mail/internal/infrastructure/cloudprovider"
	"github.com/bitan-del/dakiya-mail/internal/models"
	"github.com/bitan-del/dakiya-mail/internal/repository"
	"github.com/rs/zerolog/log"
)

// Runner is the background loop that actually drives provisioning jobs.
//
// Previously nothing called Service.Run, so a job created from the admin UI sat
// in "pending" forever. The Runner polls for in-flight jobs and drives each one
// to a terminal state (completed/failed). Run is idempotent and resumes from a
// job's current state, so a backend restart mid-provision is recoverable.
type Runner struct {
	Jobs     repository.ProvisioningJobRepository
	Svc      *Service
	Interval time.Duration // default 15s
	// DryRun is informational — it only affects the boot log line. The actual
	// dry-run behaviour comes from the Service's provider/installer wiring.
	DryRun bool
}

// Run blocks until ctx is cancelled, processing in-flight jobs on each tick.
func (r *Runner) Run(ctx context.Context) {
	interval := r.Interval
	if interval == 0 {
		interval = 15 * time.Second
	}
	log.Info().
		Bool("dry_run", r.DryRun).
		Dur("interval", interval).
		Msg("provisioning runner started")

	tick := time.NewTicker(interval)
	defer tick.Stop()
	r.processOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			r.processOnce(ctx)
		}
	}
}

func (r *Runner) processOnce(ctx context.Context) {
	if r.Svc == nil || r.Jobs == nil {
		return
	}
	jobs, err := r.Jobs.ListInFlight(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("provisioning runner: list in-flight failed")
		return
	}
	for i := range jobs {
		job := jobs[i]
		// rolling_back is a transient state the Service sets just before it
		// fails a job; skip it so we don't log a spurious "already terminal".
		if job.State == models.ProvJobRollingBack {
			continue
		}
		if err := r.Svc.Run(ctx, job.ID); err != nil {
			log.Warn().Err(err).Str("job", job.ID.String()).Msg("provisioning job failed")
		}
	}
}

// ---------------------------------------------------------------------------
// Dry-run provider
// ---------------------------------------------------------------------------

var dryRunSeq atomic.Uint64

// DryRunProvider implements cloudprovider.Provider without touching any real
// cloud API. It returns plausible fake server/IP identifiers so the state
// machine can run end-to-end in local dev (and in any environment where
// PROVISIONING_DRY_RUN is on) without creating — or billing — real machines.
type DryRunProvider struct{}

func (DryRunProvider) Name() string { return "dry-run" }

func (DryRunProvider) Locations(context.Context) ([]cloudprovider.Location, error) {
	return []cloudprovider.Location{}, nil
}
func (DryRunProvider) ServerTypes(context.Context) ([]cloudprovider.ServerType, error) {
	return []cloudprovider.ServerType{}, nil
}
func (DryRunProvider) Images(context.Context) ([]cloudprovider.Image, error) {
	return []cloudprovider.Image{}, nil
}
func (DryRunProvider) Verify(context.Context) error { return nil }

func (DryRunProvider) CreateServer(_ context.Context, req cloudprovider.CreateServerRequest) (*cloudprovider.Server, error) {
	n := dryRunSeq.Add(1)
	return &cloudprovider.Server{
		ID:         fmt.Sprintf("dryrun-%d", n),
		Name:       req.Name,
		Status:     "running",
		PublicIPv4: dryRunIP(n),
	}, nil
}

func (DryRunProvider) DeleteServer(context.Context, string) error { return nil }

func (DryRunProvider) CreatePrimaryIP(_ context.Context, req cloudprovider.CreatePrimaryIPRequest) (*cloudprovider.PrimaryIP, error) {
	n := dryRunSeq.Add(1)
	return &cloudprovider.PrimaryIP{
		ID:   fmt.Sprintf("dryrun-ip-%d", n),
		Type: req.Type,
		IP:   dryRunIP(n),
	}, nil
}

func (DryRunProvider) AssignPrimaryIP(context.Context, string, string) error { return nil }
func (DryRunProvider) UnassignPrimaryIP(context.Context, string) error       { return nil }
func (DryRunProvider) DeletePrimaryIP(context.Context, string) error         { return nil }
func (DryRunProvider) SetReverseDNS(context.Context, string, string) error   { return nil }

// dryRunIP maps a sequence number into a deterministic 10.x.x.x address so the
// inet[] columns get a valid value.
func dryRunIP(n uint64) string {
	return fmt.Sprintf("10.%d.%d.%d", (n>>16)&0xff, (n>>8)&0xff, (n&0xfe)+1)
}

var _ cloudprovider.Provider = DryRunProvider{}
