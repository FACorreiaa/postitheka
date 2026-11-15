package jobs

import (
	"math"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

func TestPublishBudget(t *testing.T) {
	const mib = 1 << 20
	video := func(sizes ...int64) publisher.Post {
		var p publisher.Post
		for _, s := range sizes {
			p.Media = append(p.Media, publisher.Media{Size: s})
		}
		return p
	}
	cases := []struct {
		name string
		post publisher.Post
		want time.Duration
	}{
		{"text only", publisher.Post{Text: "hello"}, 5 * time.Minute},
		{"ten 20 MiB files for Discord", video(20*mib, 20*mib, 20*mib, 20*mib, 20*mib, 20*mib, 20*mib, 20*mib, 20*mib, 20*mib), 5*time.Minute + 200*time.Second},
		{"a 500 MiB LinkedIn video", video(500 * mib), 5*time.Minute + 500*time.Second},
		{"sizes add up", video(60*mib, 60*mib), 7 * time.Minute},
		{"several large videos stop at the ceiling", video(1000*mib, 1000*mib), 30 * time.Minute},
		{"an absurd size stops at the ceiling", video(math.MaxInt64), 30 * time.Minute},
	}
	for _, c := range cases {
		if got := publishBudget(c.post); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// The job's own limit has to outlast the longest publish, or River cancels a
// publish that was still within its budget.
func TestJobTimeoutOutlastsThePublish(t *testing.T) {
	if got := (&PublishWorker{}).Timeout(nil); got <= maxPublishTime || got >= time.Hour {
		t.Errorf("job timeout %s must be above %s and under River's one-hour rescue", got, maxPublishTime)
	}
}
