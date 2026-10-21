package version

import "testing"

// TestShortCommit checks ShortCommit cuts Commit to its first 12 characters and returns a
// shorter one whole.
func TestShortCommit(t *testing.T) {
	saved := Commit
	t.Cleanup(func() { Commit = saved })

	for _, tc := range []struct {
		name, commit, want string
	}{
		{"full SHA", "0123456789abcdef0123456789abcdef01234567", "0123456789ab"},
		{"12 characters", "0123456789ab", "0123456789ab"},
		{"shorter, as the Makefile's fallback", "unknown", "unknown"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Commit = tc.commit
			if got := ShortCommit(); got != tc.want {
				t.Errorf("ShortCommit() with Commit %q = %q, want %q", tc.commit, got, tc.want)
			}
		})
	}
}
