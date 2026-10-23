package publisher

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Rules are the limits a platform puts on a post. Clients receive them through
// GET /v1/platforms and validate while the user writes; the server applies the
// same rules again before anything is queued.
type Rules struct {
	MaxTextLength      int
	MaxMedia           int
	MaxMediaBytes      int64
	MaxVideoBytes      int64 // per video file; 0 means MaxMediaBytes
	MaxTotalMediaBytes int64 // 0 means only the per-file limit applies
	MediaTypes         []string
	RequiresText       bool
	RequiresMedia      bool
	// VideoAlone means a video must be the only file in its post: LinkedIn
	// takes one image, several images, or one video, never a mix.
	VideoAlone bool

	// CountText measures text the way the platform does. Nil counts Unicode
	// code points. X is the reason this is pluggable: it weights URLs and
	// most non-Latin characters differently.
	CountText func(string) int
}

// MaxBytes is the per-file limit for a file of this type.
func (r Rules) MaxBytes(contentType string) int64 {
	if r.MaxVideoBytes > 0 && IsVideo(contentType) {
		return r.MaxVideoBytes
	}
	return r.MaxMediaBytes
}

func IsVideo(contentType string) bool { return strings.HasPrefix(contentType, "video/") }

func (r Rules) count(s string) int {
	if r.CountText != nil {
		return r.CountText(s)
	}
	return utf8.RuneCountInString(s)
}

// Issue is one reason a platform would reject a post.
type Issue struct {
	Field   string
	Message string
}

// Validate checks post against rules. It reports every problem rather than the
// first, so the user can fix them in one pass.
func Validate(rules Rules, post Post) []Issue {
	var issues []Issue
	add := func(field, format string, args ...any) {
		issues = append(issues, Issue{Field: field, Message: fmt.Sprintf(format, args...)})
	}

	hasText := strings.TrimSpace(post.Text) != ""
	if !hasText && len(post.Media) == 0 {
		add("post", "a post needs text or media")
		return issues
	}

	if rules.RequiresText && !hasText {
		add("text", "text is required")
	}
	if n := rules.count(post.Text); n > rules.MaxTextLength {
		add("text", "%d characters, limit is %d", n, rules.MaxTextLength)
	}

	switch {
	case rules.RequiresMedia && len(post.Media) == 0:
		add("media", "at least one file is required")
	case rules.MaxMedia == 0 && len(post.Media) > 0:
		add("media", "media is not accepted")
	case len(post.Media) > rules.MaxMedia:
		add("media", "%d files, limit is %d", len(post.Media), rules.MaxMedia)
	}
	if rules.MaxMedia == 0 {
		// Per-file complaints about a platform that takes no files at all
		// would only repeat the issue above, once per file.
		return issues
	}

	var total int64
	videos := 0
	for i, m := range post.Media {
		field := fmt.Sprintf("media[%d]", i)
		if !slices.Contains(rules.MediaTypes, m.ContentType) {
			add(field, "%s is not accepted", m.ContentType)
		}
		if limit := rules.MaxBytes(m.ContentType); m.Size > limit {
			add(field, "%d bytes, limit is %d", m.Size, limit)
		}
		if IsVideo(m.ContentType) {
			videos++
		}
		total += m.Size
	}
	if rules.VideoAlone && videos > 0 && len(post.Media) > 1 {
		add("media", "a video must be the only file in the post")
	}
	if rules.MaxTotalMediaBytes > 0 && total > rules.MaxTotalMediaBytes {
		add("media", "%d bytes in total, limit is %d", total, rules.MaxTotalMediaBytes)
	}

	return issues
}
