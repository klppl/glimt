package ingest

import "testing"

func TestScreenBucket(t *testing.T) {
	cases := map[int]string{0: "", 320: "small", 800: "medium", 1280: "large", 1920: "xlarge"}
	for w, want := range cases {
		if got := screenBucket(w); got != want {
			t.Errorf("screenBucket(%d) = %q, want %q", w, got, want)
		}
	}
}

func TestNormLang(t *testing.T) {
	cases := map[string]string{"en-US": "en", "EN": "en", "sv": "sv", "": "", "pt-BR": "pt"}
	for in, want := range cases {
		if got := normLang(in); got != want {
			t.Errorf("normLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanPath(t *testing.T) {
	cases := map[string]string{"": "/", "/": "/", "/about/": "/about", "/blog/post": "/blog/post"}
	for in, want := range cases {
		if got := cleanPath(in); got != want {
			t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstLang(t *testing.T) {
	cases := map[string]string{"en-US,en;q=0.9": "en-US", "fr": "fr", "": ""}
	for in, want := range cases {
		if got := firstLang(in); got != want {
			t.Errorf("firstLang(%q) = %q, want %q", in, got, want)
		}
	}
}
