package publisher

import (
	"context"
	"strings"
	"testing"
)

var testRules = Rules{
	MaxTextLength:      10,
	MaxMedia:           2,
	MaxMediaBytes:      100,
	MaxTotalMediaBytes: 150,
	MediaTypes:         []string{"image/png"},
}

func fields(issues []Issue) string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Field)
	}
	return strings.Join(out, ",")
}

var videoRules = Rules{
	MaxTextLength: 10,
	MaxMedia:      4,
	MaxMediaBytes: 100,
	MaxVideoBytes: 1000,
	MediaTypes:    []string{"image/png", "video/mp4"},
	VideoAlone:    true,
}

func TestValidate(t *testing.T) {
	png := func(size int64) Media { return Media{ContentType: "image/png", Size: size} }
	mp4 := func(size int64) Media { return Media{ContentType: "video/mp4", Size: size} }

	cases := map[string]struct {
		rules Rules
		post  Post
		want  string
	}{
		"ok":            {testRules, Post{Text: "hi", Media: []Media{png(10)}}, ""},
		"empty":         {testRules, Post{Text: "   "}, "post"},
		"text too long": {testRules, Post{Text: "01234567890"}, "text"},
		// Code points, not bytes: ten emoji are ten characters.
		"multibyte at limit": {testRules, Post{Text: strings.Repeat("🙂", 10)}, ""},
		"too many files":     {testRules, Post{Media: []Media{png(1), png(1), png(1)}}, "media"},
		"wrong type":         {testRules, Post{Media: []Media{{ContentType: "video/mp4", Size: 1}}}, "media[0]"},
		"file too big":       {testRules, Post{Media: []Media{png(101)}}, "media[0]"},
		"total too big":      {testRules, Post{Media: []Media{png(80), png(80)}}, "media"},
		"requires text":      {Rules{MaxTextLength: 10, MaxMedia: 1, MaxMediaBytes: 100, MediaTypes: []string{"image/png"}, RequiresText: true}, Post{Media: []Media{png(1)}}, "text"},
		"requires media":     {Rules{MaxTextLength: 10, RequiresMedia: true}, Post{Text: "hi"}, "media"},
		"no media allowed":   {Rules{MaxTextLength: 10}, Post{Text: "hi", Media: []Media{png(1)}}, "media"},
		"reports all":        {testRules, Post{Text: "01234567890", Media: []Media{png(101)}}, "text,media[0]"},
		// A video's own limit is larger than an image's: the same size is
		// fine as a video and too big as an image.
		"video under its own limit":  {videoRules, Post{Media: []Media{mp4(500)}}, ""},
		"video over its own limit":   {videoRules, Post{Media: []Media{mp4(1001)}}, "media[0]"},
		"image over the image limit": {videoRules, Post{Media: []Media{png(500)}}, "media[0]"},
		"video with an image":        {videoRules, Post{Media: []Media{mp4(10), png(10)}}, "media"},
		"several images":             {videoRules, Post{Media: []Media{png(10), png(10)}}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := fields(Validate(tc.rules, tc.post)); got != tc.want {
				t.Fatalf("issues on %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateUsesPlatformCounter(t *testing.T) {
	rules := testRules
	rules.MaxTextLength = 4
	rules.TextCounting = CountEmojiBytes
	if got := fields(Validate(rules, Post{Text: "ab👍"})); got != "text" {
		t.Fatalf("emoji counted as one: issues on %q", got)
	}
	rules.TextCounting = CountCodePoints
	if got := fields(Validate(rules, Post{Text: "ab👍"})); got != "" {
		t.Fatalf("code points over-counted: issues on %q", got)
	}
}

func TestCountText(t *testing.T) {
	for _, c := range []struct {
		text string
		want int
	}{
		{"hello", 5},
		{"olá", 3},    // accented letters are one each, not their bytes
		{"日本", 2},     // so is CJK
		{"👍", 4},      // U+1F44D, 4 bytes
		{"👍🏽", 8},     // with a skin tone
		{"❤️", 6},     // U+2764 (So, 3 bytes) + variation selector (3)
		{"1️⃣", 7},    // digit + variation selector + keycap
		{"👨‍👩‍👧", 18}, // three people (4 each) joined by two ZWJs (3 each)
		{"🇵🇹", 8},     // two regional indicators
		{"a © b", 6},  // the documented over-count: © is So
	} {
		if got := CountText(CountEmojiBytes, c.text); got != c.want {
			t.Errorf("CountText(emoji_bytes, %q) = %d, want %d", c.text, got, c.want)
		}
	}
	if got := CountText(CountCodePoints, "👍🏽"); got != 2 {
		t.Errorf("code points of a toned emoji = %d, want 2", got)
	}
}

func TestCountLinks(t *testing.T) {
	for _, c := range []struct {
		text string
		want int
	}{
		{"no links here", 0},
		{"see https://example.com/a.", 1},
		{"https://example.com and https://example.com again", 1},
		{"HTTPS://Example.com and https://example.com", 1},
		{"www.threads.com, http://a.test (https://b.test)", 3},
		{"example.com is not counted", 0},
	} {
		if got := CountLinks(c.text); got != c.want {
			t.Errorf("CountLinks(%q) = %d, want %d", c.text, got, c.want)
		}
	}
	rules := testRules
	rules.MaxLinks = 1
	rules.MaxTextLength = 100
	if got := fields(Validate(rules, Post{Text: "https://a.test https://b.test"})); got != "text" {
		t.Errorf("two links past a limit of one: issues on %q", got)
	}
}

type fakePublisher struct {
	platform Platform
	rules    Rules
}

func (f fakePublisher) Platform() Platform { return f.platform }
func (f fakePublisher) Rules() Rules       { return f.rules }
func (f fakePublisher) Publish(context.Context, []byte, Post, MediaSource) (Result, error) {
	return Result{}, nil
}

func TestAcceptsUpload(t *testing.T) {
	reg := NewRegistry(
		fakePublisher{"a", Rules{MediaTypes: []string{"image/png"}, MaxMediaBytes: 100}},
		fakePublisher{"b", Rules{MediaTypes: []string{"image/png", "video/mp4"}, MaxMediaBytes: 1000}},
	)
	if err := reg.AcceptsUpload("image/png", 500); err != nil {
		t.Fatalf("500-byte png fits platform b: %v", err)
	}
	if err := reg.AcceptsUpload("image/png", 5000); err == nil {
		t.Fatal("accepted a png larger than every platform allows")
	}
	if err := reg.AcceptsUpload("application/pdf", 1); err == nil {
		t.Fatal("accepted a type no platform takes")
	}
	video := NewRegistry(fakePublisher{"c", videoRules})
	if err := video.AcceptsUpload("video/mp4", 900); err != nil {
		t.Fatalf("a video under the video limit: %v", err)
	}
	if err := video.AcceptsUpload("image/png", 900); err == nil {
		t.Fatal("an image was held to the video limit")
	}
	if err := reg.AcceptsUpload("image/png", 0); err == nil {
		t.Fatal("accepted a zero-byte upload")
	}
}
