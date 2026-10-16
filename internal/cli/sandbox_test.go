package cli

import "testing"

// parseReadyTimeout is the one place --ready-timeout's raw flag text is
// told apart from "not given" at all (0, never a value `sandbox-ready-
// timeout-invalid` could also mean) — unit-tested directly here the same
// way decideHook (hook_test.go) is, rather than only through the built
// binary in e2e/sandbox_test.go.
func TestParseReadyTimeout(t *testing.T) {
	t.Run("empty means not given", func(t *testing.T) {
		seconds, err := parseReadyTimeout("")
		if err != nil || seconds != 0 {
			t.Errorf("parseReadyTimeout(\"\") = %d, %v, want 0, nil", seconds, err)
		}
	})

	t.Run("a positive integer is accepted", func(t *testing.T) {
		seconds, err := parseReadyTimeout("90")
		if err != nil || seconds != 90 {
			t.Errorf("parseReadyTimeout(\"90\") = %d, %v, want 90, nil", seconds, err)
		}
	})

	for _, invalid := range []string{"0", "-1", "1.5", "60s", "not-a-number"} {
		t.Run("rejected: "+invalid, func(t *testing.T) {
			if _, err := parseReadyTimeout(invalid); err == nil {
				t.Errorf("parseReadyTimeout(%q) accepted", invalid)
			}
		})
	}
}
