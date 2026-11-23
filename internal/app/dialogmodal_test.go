package app

import (
	"testing"
	"time"
)

// A dialog starts unseen, comes in over dialogIn, and goes out over
// dialogOut once it closes.
func TestDialogMotion(t *testing.T) {
	now := time.Now()
	m := &dialogMotion{}
	if p := m.fade(now); p != 0 {
		t.Errorf("it starts at %v, want 0: unseen", p)
	}
	if p := m.fade(now.Add(dialogIn / 2)); p < 0.4 || p > 0.6 {
		t.Errorf("half way in it is %v, want about 0.5", p)
	}
	if p := m.fade(now.Add(dialogIn)); p != 1 {
		t.Errorf("once in it is %v, want 1", p)
	}
	if m.goingOut() || m.gone(now.Add(time.Hour)) {
		t.Error("it goes out before it closed")
	}

	m.begin(now)
	if !m.goingOut() {
		t.Error("it does not go out once closed")
	}
	if m.gone(now.Add(dialogOut / 2)) {
		t.Error("it is gone before its way out is done")
	}
	if p := m.fade(now.Add(dialogOut / 2)); p < 0.4 || p > 0.6 {
		t.Errorf("half way out it is %v, want about 0.5", p)
	}
	if !m.gone(now.Add(dialogOut)) {
		t.Error("it is not gone once its way out is done")
	}
	if p := m.fade(now.Add(dialogOut)); p != 0 {
		t.Errorf("once out it is %v, want 0", p)
	}
}
