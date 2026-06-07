package referrer

import (
	"testing"

	"github.com/klppl/glimt/internal/model"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		ref, own  string
		wantClass int
		wantSrc   string
	}{
		{"", "", model.RefDirect, ""},
		{"https://www.google.com/search?q=x", "", model.RefSearch, "Google"},
		{"https://t.co/abc", "", model.RefSocial, "Twitter"},
		{"https://news.ycombinator.com/", "", model.RefSocial, "Hacker News"},
		{"https://example.com/blog", "example.com", model.RefInternal, ""},
		{"https://sub.example.com/x", "example.com", model.RefInternal, ""},
		{"https://www.someblog.io/post", "", model.RefReferral, "someblog.io"},
	}
	for _, c := range cases {
		class, src := Classify(c.ref, c.own)
		if class != c.wantClass || src != c.wantSrc {
			t.Errorf("Classify(%q,%q) = (%d,%q), want (%d,%q)",
				c.ref, c.own, class, src, c.wantClass, c.wantSrc)
		}
	}
}
