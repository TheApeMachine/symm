package disconnect

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

func TestExpected(t *testing.T) {
	t.Parallel()

	if !Expected(&websocket.CloseError{Code: websocket.CloseAbnormalClosure}) {
		t.Fatal("abnormal closure should be expected")
	}

	if Expected(errors.New("json: invalid")) {
		t.Fatal("unrelated errors should not be expected")
	}
}
