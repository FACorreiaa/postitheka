package publisher

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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

	// Limits on a video's length and shape; zero means no limit. They are
	// checked against what the client declared with the upload, and skipped
	// for a video whose client declared nothing. Aspect is width divided by
	// height.
	MinVideoLength, MaxVideoLength time.Duration
	MinVideoAspect, MaxVideoAspect float64
	MaxVideoWidth                  int

	// Options are what an author can set for one destination beyond the
	// post itself. A platform lists only what its Publish reads.
	Options []Option

	// TextCounting is how the platform measures MaxTextLength. It is a
	// named mode rather than a function because the clients count too, and
	// each mode is written three times: here, web/src/rules.ts and the iOS
	// kit's Rules.swift.
	TextCounting TextCounting
}

// Option is one value an author can set per destination: the title of a
// forum post, a subreddit. Every option is a line of text; the clients build
// their form from this, so adding one needs no client change.
type Option struct {
	Key       string
	Label     string
	MaxLength int // in code points
	Required  bool
}

type TextCounting string

const (
	CountCodePoints TextCounting = "code_points"
	// CountEmojiBytes counts a character as one and an emoji as its UTF-8
	// length (Threads).
	CountEmojiBytes TextCounting = "emoji_bytes"
	// CountXWeighted is X's count: a link is 23 whatever its length, and a
	// character outside X's short list of ranges (CJK, emoji) is 2.
	CountXWeighted TextCounting = "x_weighted"
)

// CountText measures s the way mode does. An unset mode counts code points.
func CountText(mode TextCounting, s string) int {
	switch mode {
	case CountEmojiBytes:
		return countEmojiBytes(s)
	case CountXWeighted:
		return countXWeighted(s)
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

// xLinkLength is what any link counts as on X, which wraps every one in its
// own shortener.
const xLinkLength = 23

// countXWeighted follows twitter-text's configuration (version 3, read
// 2026-11-15): text is normalised to NFC, a link counts 23, a code point in
// U+0000-10FF, U+2000-200D, U+2010-201F or U+2032-2037 counts 1, and any
// other counts 2.
//
// Two places it differs from X, both known:
//   - X counts a whole emoji as 2, however many code points it is made of.
//     Each code point is counted here, so a flag or an emoji with a skin
//     tone counts 4 and a family 11. A post near the limit may be refused
//     that X would take, never the reverse.
//   - X also treats a bare domain (example.com) as a link. Only what
//     linkPattern finds is a link here, so a bare domain shorter than 23
//     characters is under-counted, and X then refuses the post itself.
func countXWeighted(s string) int {
	s = norm.NFC.String(s)
	n, last := 0, 0
	for _, loc := range linkPattern.FindAllStringIndex(s, -1) {
		end := loc[0] + len(strings.TrimRight(s[loc[0]:loc[1]], linkTrailing))
		n += xWeight(s[last:loc[0]]) + xLinkLength
		last = end
	}
	return n + xWeight(s[last:])
}

func xWeight(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r <= 0x10FF, r >= 0x2000 && r <= 0x200D, r >= 0x2010 && r <= 0x201F, r >= 0x2032 && r <= 0x2037:
			n++
		default:
			n += 2
		}
	}
	return n
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

// linkTrailing is punctuation that ends a sentence, not a link.
const linkTrailing = ".,;:!?)]}'"

// CountLinks counts the distinct links in s, the way Threads counts toward
// its limit: the same URL twice is one link.
func CountLinks(s string) int {
	seen := map[string]bool{}
	for _, l := range linkPattern.FindAllString(s, -1) {
		seen[strings.ToLower(strings.TrimRight(l, linkTrailing))] = true
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

	issues = append(issues, rules.optionIssues(post.Options)...)

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
			for _, msg := range rules.videoIssues(m) {
				add(field, "%s", msg)
			}
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

// optionIssues checks what the author set for this destination against what
// the platform takes. A key the platform does not list is refused rather
// than dropped: the author meant something by it, and it would not happen.
func (r Rules) optionIssues(values map[string]string) []Issue {
	var out []Issue
	known := map[string]bool{}
	for _, o := range r.Options {
		known[o.Key] = true
		v := strings.TrimSpace(values[o.Key])
		switch n := utf8.RuneCountInString(v); {
		case v == "" && o.Required:
			out = append(out, Issue{Field: "options." + o.Key, Message: o.Label + " is required"})
		case n > o.MaxLength:
			out = append(out, Issue{Field: "options." + o.Key, Message: fmt.Sprintf("%s is %d characters, limit is %d", o.Label, n, o.MaxLength)})
		}
	}
	unknown := make([]string, 0, len(values))
	for k := range values {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	for _, k := range unknown {
		out = append(out, Issue{Field: "options." + k, Message: k + " is not an option of this platform"})
	}
	return out
}

// videoIssues checks one video against the length and shape limits, using
// only what its client declared.
func (r Rules) videoIssues(m Media) []string {
	var out []string
	if m.Duration > 0 {
		if r.MinVideoLength > 0 && m.Duration < r.MinVideoLength {
			out = append(out, fmt.Sprintf("the video is %s long, the shortest accepted is %s", clock(m.Duration), clock(r.MinVideoLength)))
		}
		if r.MaxVideoLength > 0 && m.Duration > r.MaxVideoLength {
			out = append(out, fmt.Sprintf("the video is %s long, limit is %s", clock(m.Duration), clock(r.MaxVideoLength)))
		}
	}
	if m.Width > 0 && m.Height > 0 {
		aspect := float64(m.Width) / float64(m.Height)
		if (r.MinVideoAspect > 0 && aspect < r.MinVideoAspect) || (r.MaxVideoAspect > 0 && aspect > r.MaxVideoAspect) {
			out = append(out, fmt.Sprintf("the video is %d by %d, a shape that is not accepted", m.Width, m.Height))
		}
		if r.MaxVideoWidth > 0 && m.Width > r.MaxVideoWidth {
			out = append(out, fmt.Sprintf("the video is %d pixels wide, limit is %d", m.Width, r.MaxVideoWidth))
		}
	}
	return out
}

// clock writes a length as m:ss, to the tenth of a second under ten seconds
// where a limit of three seconds makes the tenths matter.
func clock(d time.Duration) string {
	if d < 10*time.Second {
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	s := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
