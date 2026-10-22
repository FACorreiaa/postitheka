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

func TestValidate(t *testing.T) {
	png := func(size int64) Media { return Media{ContentType: "image/png", Size: size} }

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
	rules.CountText = func(s string) int { return len(s) * 2 }
	if got := fields(Validate(rules, Post{Text: "123456"})); got != "text" {
		t.Fatalf("custom counter ignored: issues on %q", got)
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
	if err := reg.AcceptsUpload("image/png", 0); err == nil {
		t.Fatal("accepted a zero-byte upload")
	}
}
