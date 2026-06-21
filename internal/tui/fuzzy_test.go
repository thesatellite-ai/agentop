package tui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		query, target string
		want          bool
	}{
		{"", "anything", true},            // empty matches all
		{"flm", "default-filemark", true}, // subsequence
		{"codex", "codex direct", true},   // exact substring is a subsequence
		{"cdx", "codex", true},            // gapped subsequence
		{"xyz", "codex", false},           // missing chars
		{"emdash", "cmux", false},         // no overlap
		{"go", "sync_go", true},           // trailing match
		{"zz", "codex", false},            // repeated char not present
	}
	for _, tc := range cases {
		if got := fuzzyMatch(tc.query, tc.target); got != tc.want {
			t.Fatalf("fuzzyMatch(%q,%q)=%v want %v", tc.query, tc.target, got, tc.want)
		}
	}
}
