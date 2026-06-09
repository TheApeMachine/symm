package types

import (
	"encoding/json"
	"fmt"
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
	sm.Channel = ""
	sm.Type = ""
	sm.Method = ""
	sm.Errors = nil
	sm.Success = nil
	sm.Data = nil
	sm.TimeIn = nil
	sm.TimeOut = nil
	socketMessagePool.Put(sm)
}

type KrakenMessage struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ReqID  int64  `json:"req_id,omitempty"`
}

/*
NewKrakenMessage marshals params for the wire.
*/
func NewKrakenMessage(method string, params any, reqID int64) (KrakenMessage, error) {
	raw, err := sonic.Marshal(params)

	if err != nil {
		return KrakenMessage{}, fmt.Errorf("types: marshal %s params: %w", method, err)
	}

	return KrakenMessage{
		Method: method,
		Params: json.RawMessage(raw),
		ReqID:  reqID,
	}, nil
}
