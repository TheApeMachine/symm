package types

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

var socketMessagePool = sync.Pool{
	New: func() any {
		return &SocketMessage{}
	},
}

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Method  string          `json:"method"`
	Errors  []string        `json:"errors"`
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
	TimeIn  *time.Time      `json:"time_in"`
	TimeOut *time.Time      `json:"time_out"`
}

func NewSocketMessage() *SocketMessage {
	return socketMessagePool.Get().(*SocketMessage)
}

func (sm *SocketMessage) Unmarshal(model any) error {
	return sonic.Unmarshal(sm.Data, model)
}

func (sm *SocketMessage) Release() {
	socketMessagePool.Put(sm)
}
