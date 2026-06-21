package tui

import "testing"

func TestMem(t *testing.T) {
	const mb = 1024 * 1024
	cases := []struct {
		b    uint64
		want string
	}{
		{0, "0MB"},
		{512 * mb, "512MB"},
		{1024 * mb, "1.0GB"},
		{1536 * mb, "1.5GB"},
	}
	for _, tc := range cases {
		if got := mem(tc.b); got != tc.want {
			t.Errorf("mem(%d) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestTrunc(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 8, "this is…"},
		{"abc", 1, "a"},
	}
	for _, tc := range cases {
		if got := trunc(tc.s, tc.n); got != tc.want {
			t.Errorf("trunc(%q,%d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestBar(t *testing.T) {
	cases := []struct {
		frac float64
		w    int
		want string
	}{
		{0, 4, "░░░░"},
		{1, 4, "████"},
		{0.5, 4, "██░░"},
		{-1, 4, "░░░░"}, // clamps low
		{2, 4, "████"},  // clamps high
	}
	for _, tc := range cases {
		if got := bar(tc.frac, tc.w); got != tc.want {
			t.Errorf("bar(%v,%d) = %q, want %q", tc.frac, tc.w, got, tc.want)
		}
	}
}

func TestHomePath(t *testing.T) {
	if got := homePath("/Users/x/code/proj", "/Users/x"); got != "~/code/proj" {
		t.Errorf("homePath = %q, want ~/code/proj", got)
	}
	if got := homePath("/opt/other", "/Users/x"); got != "/opt/other" {
		t.Errorf("homePath should pass through non-home, got %q", got)
	}
	if got := homePath("/a/b", ""); got != "/a/b" {
		t.Errorf("empty home should pass through, got %q", got)
	}
}
