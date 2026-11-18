package publisher_test

import (
	"strings"
	"testing"

	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/publisher/discord"
	"github.com/FACorreiaa/postitheka/internal/publisher/facebook"
	"github.com/FACorreiaa/postitheka/internal/publisher/instagram"
	"github.com/FACorreiaa/postitheka/internal/publisher/linkedin"
	"github.com/FACorreiaa/postitheka/internal/publisher/threads"
	"github.com/FACorreiaa/postitheka/internal/publisher/tiktok"
	"github.com/FACorreiaa/postitheka/internal/publisher/x"
)

func TestValidateChecksTheFirstComment(t *testing.T) {
	rules := publisher.Rules{MaxTextLength: 100, MaxCommentLength: 5}
	issues := publisher.Validate(rules, publisher.Post{Text: "hi", Comment: "#toolong"})
	if len(issues) != 1 || issues[0].Field != "first_comment" || issues[0].Message != "8 characters, limit is 5" {
		t.Errorf("issues %+v; want one on first_comment", issues)
	}

	// A platform that takes no comment publishes the post alone, so a long
	// comment is no reason to refuse it.
	rules.MaxCommentLength = 0
	if issues := publisher.Validate(rules, publisher.Post{Text: "hi", Comment: strings.Repeat("#", 500)}); len(issues) != 0 {
		t.Errorf("issues %+v; want none where comments are not taken", issues)
	}
}

// The service offers a comment wherever Rules give a length, and the job
// sends it through Commenter: the two must agree on every platform.
func TestEveryPlatformThatTakesACommentCanSendOne(t *testing.T) {
	for _, p := range []publisher.Publisher{
		discord.New(nil),
		linkedin.New("", "", nil),
		threads.New("", "", nil),
		instagram.New("", "", nil),
		facebook.New("", "", nil),
		x.New("", "", nil),
		tiktok.New("", "", nil),
	} {
		_, commenter := p.(publisher.Commenter)
		if takes := p.Rules().MaxCommentLength > 0; takes != commenter {
			t.Errorf("%s: MaxCommentLength %d but Commenter %v", p.Platform(), p.Rules().MaxCommentLength, commenter)
		}
	}
}
