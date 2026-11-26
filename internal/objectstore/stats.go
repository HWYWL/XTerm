package objectstore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// StatusError is what a storage answered when it refused a request: the
// status the server gave, which the monitor counts by code.
type StatusError struct {
	Op     string
	Key    string
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		return fmt.Sprintf("%s %s: %d", e.Op, e.Key, e.Status)
	}
	return fmt.Sprintf("%s %s: %d: %s", e.Op, e.Key, e.Status, msg)
}

// HTTPStatus is the status of a request that failed, false for a failure
// that never reached a server.
func HTTPStatus(err error) (int, bool) {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status, true
	}
	if errors.Is(err, ErrNotFound) {
		return 404, true
	}
	return 0, false
}

// IsTimeout tells whether err is a request that ran out of time: this
// app's timeout, or the network's.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
