package publisher

import (
	"strings"
	"testing"
	"time"
)

func TestValidateVideoLengthAndShape(t *testing.T) {
	rules := Rules{
		MaxTextLength: 100, MaxMedia: 4, MaxMediaBytes: 1 << 30,
		MediaTypes:     []string{"video/mp4", "image/jpeg"},
		MinVideoLength: 3 * time.Second, MaxVideoLength: 5 * time.Minute,
		MinVideoAspect: 0.5, MaxVideoAspect: 2, MaxVideoWidth: 1920,
	}
	video := func(d time.Duration, w, h int) Post {
		return Post{Media: []Media{{ContentType: "video/mp4", Size: 1, Duration: d, Width: w, Height: h}}}
	}
	cases := []struct {
		name string
		post Post
		want string // a part of the one issue expected; empty for none
	}{
		{"within every limit", video(time.Minute, 1080, 1920), ""},
		{"exactly the shortest", video(3*time.Second, 1080, 1920), ""},
		{"exactly the longest", video(5*time.Minute, 1080, 1920), ""},
		{"too short", video(2500*time.Millisecond, 1080, 1920), "2.5 s long, the shortest accepted is 3.0 s"},
		{"too long", video(6*time.Minute+12*time.Second, 1080, 1920), "6:12 long, limit is 5:00"},
		{"too tall for its width", video(time.Minute, 400, 1000), "400 by 1000"},
		{"too wide for its height", video(time.Minute, 1900, 800), "1900 by 800"},
		{"more pixels across than accepted", video(time.Minute, 3840, 2160), "3840 pixels wide, limit is 1920"},
		{"nothing declared is not checked", video(0, 0, 0), ""},
		{"a length alone is checked", video(10*time.Minute, 0, 0), "10:00 long"},
		{"a width without a height is not a shape", video(time.Minute, 4000, 0), ""},
		{"an image is never measured as a video", Post{Media: []Media{{ContentType: "image/jpeg", Size: 1, Duration: time.Hour, Width: 9000, Height: 10}}}, ""},
	}
	for _, c := range cases {
		issues := Validate(rules, c.post)
		switch {
		case c.want == "" && len(issues) != 0:
			t.Errorf("%s: unexpected issues %v", c.name, issues)
		case c.want != "" && (len(issues) != 1 || issues[0].Field != "media[0]" || !strings.Contains(issues[0].Message, c.want)):
			t.Errorf("%s: got %v, want one issue on media[0] containing %q", c.name, issues, c.want)
		}
	}

	// No limits set means no video is refused for its length or shape.
	if issues := Validate(Rules{MaxTextLength: 1, MaxMedia: 1, MaxMediaBytes: 1 << 30, MediaTypes: []string{"video/mp4"}}, video(9*time.Hour, 50000, 1)); len(issues) != 0 {
		t.Errorf("rules without video limits refused a video: %v", issues)
	}
}
