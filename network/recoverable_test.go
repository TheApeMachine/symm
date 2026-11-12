package network

import (
	"errors"
	"testing"

	"github.com/theapemachine/errnie"
)

func TestRecoverable(t *testing.T) {
	if Recoverable(nil) {
		t.Fatal("nil is not recoverable")
	}

	dial := errnie.Err(errnie.IO, "[network.websocket] error dialing url", errors.New("dial"))
	if !Recoverable(dial) {
		t.Fatalf("dial failure must be recoverable: %v", dial)
	}

	hook := errnie.Err(errnie.IO, "[network.websocket] reconnect hook failed", errors.New("token"))
	if !Recoverable(hook) {
		t.Fatalf("hook failure must be recoverable: %v", hook)
	}

	bug := errnie.Err(errnie.Validation, "[broker] corrupt frame", nil)
	if Recoverable(bug) {
		t.Fatalf("validation errors are not recoverable: %v", bug)
	}
}
