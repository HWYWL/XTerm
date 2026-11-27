package app

import "testing"

// The apps list reads pm's "package:<apk>=<package>" lines, and the ones
// after the system marker are the device's own.
func TestParseADBPackages(t *testing.T) {
	out := `__USER__
package:/data/app/~~x/com.foo-y/base.apk=com.foo
package:/data/app/~~y/com.bar-z/base.apk=com.bar
__SYSTEM__
package:/system/priv-app/Settings/Settings.apk=com.android.settings
`
	list := parseADBPackages(out)
	if len(list) != 3 {
		t.Fatalf("%d apps: %+v", len(list), list)
	}
	by := map[string]adbApp{}
	for _, a := range list {
		by[a.Package] = a
	}
	if a := by["com.foo"]; a.System || a.Path != "/data/app/~~x/com.foo-y/base.apk" {
		t.Errorf("com.foo is %+v", a)
	}
	if a := by["com.android.settings"]; !a.System {
		t.Errorf("settings is %+v", a)
	}
	if list[0].Package != "com.android.settings" {
		t.Errorf("the list is not in package order: %+v", list)
	}
}
