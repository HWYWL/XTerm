//go:build windows

package serialstat

import (
	"testing"

	"golang.org/x/sys/windows"
)

// The handle the library keeps is read from its field; a port without one
// falls back to no counters.
type handleHolder struct{ handle windows.Handle }

func TestHandleOf(t *testing.T) {
	h, ok := handleOf(&handleHolder{handle: 7})
	if !ok || h != windows.Handle(7) {
		t.Errorf("the handle %v %v", h, ok)
	}
	if _, ok := handleOf(&fakePort{}); ok {
		t.Error("a port without a handle")
	}
	if _, ok := handleOf(nil); ok {
		t.Error("a nil port")
	}
	if _, ok := handleOf(handleHolder{}); ok {
		t.Error("a port that is not a pointer")
	}
}
