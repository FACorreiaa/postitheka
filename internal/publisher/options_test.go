package publisher

import (
	"strings"
	"testing"
)

func TestValidateOptions(t *testing.T) {
	rules := Rules{MaxTextLength: 100, Options: []Option{
		{Key: "title", Label: "Title", MaxLength: 5, Required: true},
		{Key: "flair", Label: "Flair", MaxLength: 5},
	}}
	cases := []struct {
		name    string
		options map[string]string
		want    string
	}{
		{"all set", map[string]string{"title": "Hello", "flair": "news"}, ""},
		{"optional left out", map[string]string{"title": "Hello"}, ""},
		{"required missing", nil, "options.title: Title is required"},
		{"required blank", map[string]string{"title": "  "}, "options.title: Title is required"},
		{"counted in characters", map[string]string{"title": "ééééé"}, ""},
		{"too long", map[string]string{"title": "Hello!"}, "options.title: Title is 6 characters, limit is 5"},
		{"unknown key", map[string]string{"title": "Hello", "zeta": "1", "alpha": "2"}, "options.alpha: alpha is not an option of this platform; options.zeta: zeta is not an option of this platform"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, i := range Validate(rules, Post{Text: "x", Options: c.options}) {
				got = append(got, i.Field+": "+i.Message)
			}
			if s := strings.Join(got, "; "); s != c.want {
				t.Errorf("got %q, want %q", s, c.want)
			}
		})
	}
}

// A platform that lists no options takes none.
func TestValidateRefusesOptionsWhereThereAreNone(t *testing.T) {
	issues := Validate(Rules{MaxTextLength: 100}, Post{Text: "x", Options: map[string]string{"title": "a"}})
	if len(issues) != 1 || issues[0].Field != "options.title" {
		t.Fatalf("issues %+v", issues)
	}
}
