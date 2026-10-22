package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/FACorreiaa/postitheka/internal/db"
)

// UnattachedMediaTTL is how long an upload may wait to be attached to a post.
// An upload URL lasts 30 minutes; a day leaves room for a draft written slowly.
const UnattachedMediaTTL = 24 * time.Hour

// sweepBatch bounds how many rows one transaction locks while it talks to
// storage.
const sweepBatch = 200

type SweepMediaArgs struct{}

// Not unique: River enqueues periodic jobs from the leader only, and two
// sweeps that do overlap skip each other's locked rows.
func (SweepMediaArgs) Kind() string { return "sweep_unattached_media" }

// ObjectDeleter is the slice of media.Store the sweep uses.
type ObjectDeleter interface {
	Delete(ctx context.Context, key string) error
}

// SweepMediaWorker deletes uploads that were reserved and never attached to a
// post: the media row and the object in the bucket.
type SweepMediaWorker struct {
	river.WorkerDefaults[SweepMediaArgs]

	pool    *pgxpool.Pool
	q       *db.Queries
	storage ObjectDeleter
	log     *slog.Logger
}

func NewSweepMediaWorker(pool *pgxpool.Pool, storage ObjectDeleter, log *slog.Logger) *SweepMediaWorker {
	return &SweepMediaWorker{pool: pool, q: db.New(pool), storage: storage, log: log}
}

// PeriodicSweepMedia runs the sweep hourly, and once when the server starts.
func PeriodicSweepMedia() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Hour),
		func() (river.JobArgs, *river.InsertOpts) { return SweepMediaArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

func (w *SweepMediaWorker) Timeout(*river.Job[SweepMediaArgs]) time.Duration { return 10 * time.Minute }

func (w *SweepMediaWorker) Work(ctx context.Context, _ *river.Job[SweepMediaArgs]) error {
	cutoff := time.Now().Add(-UnattachedMediaTTL)
	total := 0
	for {
		locked, deleted, err := w.sweepBatch(ctx, cutoff)
		total += deleted
		if err != nil {
			return err
		}
		// Rows locked but not deleted were attached meanwhile; the next
		// batch no longer sees them, so a short batch means the end.
		if locked < sweepBatch {
			break
		}
	}
	if total > 0 {
		w.log.Info("swept unattached uploads", "count", total)
	}
	return nil
}

// sweepBatch deletes objects before committing the row deletes. If the commit
// then fails, the rows outlive their objects until the next sweep removes
// them; the other order would leave objects no row points to, which nothing
// would ever find again.
func (w *SweepMediaWorker) sweepBatch(ctx context.Context, cutoff time.Time) (locked, deleted int, err error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.q.WithTx(tx)

	ids, err := qtx.LockUnattachedMedia(ctx, db.LockUnattachedMediaParams{CreatedAt: cutoff, Limit: sweepBatch})
	if err != nil || len(ids) == 0 {
		return 0, 0, err
	}
	keys, err := qtx.DeleteUnattachedMedia(ctx, ids)
	if err != nil {
		return 0, 0, err
	}
	for _, key := range keys {
		if err := w.storage.Delete(ctx, key); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit sweep: %w", err)
	}
	return len(ids), len(keys), nil
}
