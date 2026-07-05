package ui

import (
	"sync"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/theapemachine/errnie"
)

/*
Snapshot keeps the latest backend-owned UI messages for new websocket clients.
*/
type Snapshot struct {
	messages sync.Map
}

func NewSnapshot() *Snapshot {
	return &Snapshot{}
}

func (snapshot *Snapshot) Observe(message Message) error {
	if snapshot == nil {
		return errnie.Err(errnie.Validation, "ui: snapshot is nil", nil)
	}

	if message.Empty() {
		return errnie.Err(errnie.Validation, "ui: snapshot message is empty", nil)
	}

	if message.Decision != nil {
		return nil
	}

	snapshot.messages.Store(message.Key(), message)
	return nil
}

func (snapshot *Snapshot) Replay(conn *websocket.Conn) error {
	if snapshot == nil {
		return errnie.Err(errnie.Validation, "ui: snapshot is nil", nil)
	}

	if conn == nil {
		return errnie.Err(errnie.Validation, "ui: websocket connection is nil", nil)
	}

	var err error

	snapshot.messages.Range(func(_, value any) bool {
		message, ok := value.(Message)

		if !ok || message.Empty() {
			err = errnie.Err(errnie.Validation, "ui: snapshot contains invalid message", nil)
			return false
		}

		if writeErr := writeMessage(conn, message); writeErr != nil {
			err = errnie.Err(errnie.IO, "ui: replay latest message failed", writeErr)
			return false
		}

		return true
	})

	return err
}
