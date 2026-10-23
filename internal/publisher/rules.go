package publisher

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Rules are the limits a platform puts on a post. Clients receive them through
// GET /v1/platforms and validate while the user writes; the server applies the
// same rules again before anything is queued.
type Rules struct {
	MaxTextLength      int
	MaxLinks           int // distinct links in the text; 0 means no limit
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

	// TextCounting is how the platform measures MaxTextLength. It is a
	// named mode rather than a function because the clients count too, and
	// each mode is written three times: here, web/src/rules.ts and the iOS
	// kit's Rules.swift.
	TextCounting TextCounting
}

type TextCounting string

const (
	CountCodePoints TextCounting = "code_points"
	// CountEmojiBytes counts a character as one and an emoji as its UTF-8
	// length (Threads).
	CountEmojiBytes TextCounting = "emoji_bytes"
)

// CountText measures s the way mode does. An unset mode counts code points.
func CountText(mode TextCounting, s string) int {
	if mode == CountEmojiBytes {
		return countEmojiBytes(s)
	}
	return utf8.RuneCountInString(s)
}

// countEmojiBytes has no emoji property to go on, so every symbol (category
// So) and every emoji component (joiner, variation selector, skin tone,
// keycap, tag) counts by bytes. That over-counts a few symbols such as ©
// (2 instead of 1): a post near the limit may be refused that the platform
// would take, never the reverse.
func countEmojiBytes(s string) int {
	n := 0
	for _, r := range s {
		if isEmojiPart(r) {
			n += utf8.RuneLen(r)
		} else {
			n++
		}
	}
	return n
}

func isEmojiPart(r rune) bool {
	switch {
	case unicode.Is(unicode.So, r),
		r == 0x200D,                  // zero-width joiner
		r == 0x20E3,                  // combining keycap
		r >= 0xFE00 && r <= 0xFE0F,   // variation selectors
		r >= 0x1F3FB && r <= 0x1F3FF, // skin tones (category Sk)
		r >= 0xE0020 && r <= 0xE007F: // tags, for flags of regions
		return true
	}
	return false
}

// MaxBytes is the per-file limit for a file of this type.
func (r Rules) MaxBytes(contentType string) int64 {
	if r.MaxVideoBytes > 0 && IsVideo(contentType) {
		return r.MaxVideoBytes
	}
	return r.MaxMediaBytes
}

func IsVideo(contentType string) bool { return strings.HasPrefix(contentType, "video/") }

// linkPattern finds what CountLinks treats as a link: a URL with a scheme,
// or one starting www. Platforms also link bare domains (example.com); those
// are not counted, since guessing at them would refuse ordinary sentences.
var linkPattern = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"]+`)

// CountLinks counts the distinct links in s, the way Threads counts toward
// its limit: the same URL twice is one link.
func CountLinks(s string) int {
	seen := map[string]bool{}
	for _, l := range linkPattern.FindAllString(s, -1) {
		seen[strings.ToLower(strings.TrimRight(l, ".,;:!?)]}'"))] = true
	}
	return len(seen)
}

func (r Rules) count(s string) int { return CountText(r.TextCounting, s) }

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
	if rules.MaxLinks > 0 {
		if n := CountLinks(post.Text); n > rules.MaxLinks {
			add("text", "%d links, limit is %d", n, rules.MaxLinks)
		}
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
