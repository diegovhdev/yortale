package auth

import "testing"

func TestSafeNext(t *testing.T) {
	tests := map[string]string{
		"":                    "/",
		"/stories/new":        "/stories/new",
		"/stories?q=a&page=2": "/stories?q=a&page=2",
		"https://malo.com":    "/",
		"//malo.com":          "/",
		`/\malo.com`:          "/",
		"javascript:alert(1)": "/",
		"stories":             "/",
	}
	for in, want := range tests {
		if got := SafeNext(in); got != want {
			t.Errorf("SafeNext(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}
