package ui

import (
	"errors"
	"net"
	"syscall"
	"testing"
)

func TestIsBenignWriteError(t *testing.T) {
	if isBenignWriteError(nil) {
		t.Fatal("nil error is not benign")
	}

	if !isBenignWriteError(net.ErrClosed) {
		t.Fatal("net.ErrClosed should be benign")
	}

	if !isBenignWriteError(errors.New("write tcp: broken pipe")) {
		t.Fatal("broken pipe message should be benign")
	}

	if !isBenignWriteError(&net.OpError{
		Op:  "write",
		Err: syscall.EPIPE,
	}) {
		t.Fatal("EPIPE should be benign")
	}

	if isBenignWriteError(errors.New("write: no space left on device")) {
		t.Fatal("unexpected error should not be benign")
	}
}
