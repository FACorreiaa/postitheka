package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/FACorreiaa/postitheka/internal/db"
)

// HandoffTTL is how long an assisted target may wait for the phone to report
// what happened in the share sheet. A handoff takes seconds; an hour covers a
// user who leaves the sheet open, without leaving Sent showing "On the phone"
// for a post that will never be reported.
const HandoffTTL = time.Hour

// HandoffExpiredMessage is shown on the target. It cannot say the post was not
// sent: the user may have tapped Send before the app was killed.
const HandoffExpiredMessage = "The phone never reported back. Check WhatsApp before sending it again."

type ExpireHandoffsArgs struct{}

func (ExpireHandoffsArgs) Kind() string { return "expire_handoffs" }

// ExpireHandoffsWorker fails assisted targets still handing_off after
// HandoffTTL. A report arriving later gets a 409, as any repeat does.
type ExpireHandoffsWorker struct {
	river.WorkerDefaults[ExpireHandoffsArgs]

	q   *db.Queries
	log *slog.Logger
	now func() time.Time
}

func NewExpireHandoffsWorker(q *db.Queries, log *slog.Logger) *ExpireHandoffsWorker {
	return &ExpireHandoffsWorker{q: q, log: log, now: time.Now}
}

// PeriodicExpireHandoffs runs every ten minutes, and once when the server
// starts, so a target expires at most ten minutes past its TTL.
func PeriodicExpireHandoffs() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(10*time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return ExpireHandoffsArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

func (w *ExpireHandoffsWorker) Work(ctx context.Context, _ *river.Job[ExpireHandoffsArgs]) error {
	msg := HandoffExpiredMessage
	n, err := w.q.ExpireHandoffs(ctx, db.ExpireHandoffsParams{
		LastError: &msg,
		Cutoff:    w.now().Add(-HandoffTTL),
	})
	if err != nil {
		return err
	}
	if n > 0 {
		w.log.Info("expired handoffs", "count", n)
	}
	return nil
}
