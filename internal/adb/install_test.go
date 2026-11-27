package adb

import "testing"

// pm install-create's session comes out of its line, or its id alone.
func TestInstallSession(t *testing.T) {
	for _, c := range []struct{ out, want string }{
		{"Success: created install session [1234567]", "1234567"},
		{"Success: created install session [12]\n", "12"},
		{"1234", "1234"},
		{"Failure [INSTALL_FAILED_INVALID_APK]", ""},
	} {
		if got := installSession(c.out); got != c.want {
			t.Errorf("%q: %q, want %q", c.out, got, c.want)
		}
	}
}
