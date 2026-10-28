package procs

import "testing"

func TestDeepestPrefersLongestProjectPath(t *testing.T) {
	sorted := []string{"/w/a/web", "/w/a", "/w/b"}
	cases := map[string]string{
		"/w/a/web/src": "/w/a/web",
		"/w/a/other":   "/w/a",
		"/w/a":         "/w/a",
		"/w/ab":        "", // sibling sharing a name prefix must not match
		"/elsewhere":   "",
	}
	for cwd, want := range cases {
		if got := deepest(sorted, cwd); got != want {
			t.Errorf("deepest(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestDevTokensAreExact(t *testing.T) {
	for _, bad := range []string{"serve", "codegraph", "/x/devtools/index.js", "--mcp"} {
		if devTokens[bad] {
			t.Errorf("%q must not be a dev token", bad)
		}
	}
}
