package types

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
	Error   string          `json:"errors" `
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	TimeIn  time.Time       `json:"time_in"`
	TimeOut time.Time       `json:"time_out"`
}

func Acquire() *SocketMessage {
	return socketMessagePool.Get().(*SocketMessage)
}

func (socketMessage *SocketMessage) Marshal() []byte {
	return errnie.Does(func() ([]byte, error) {
		return sonic.Marshal(socketMessage)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"unable to marshal message",
			err,
		))
	}).Value()
}

func (socketMessage *SocketMessage) Decode(payload []byte) error {
	return sonic.Unmarshal(payload, socketMessage)
}

func (socketMessage *SocketMessage) Unmarshal(model any) error {
	return sonic.Unmarshal(socketMessage.Data, model)
}

func (sm *SocketMessage) Release() {
	sm.Channel = ""
	sm.Type = ""
	sm.Method = ""
	sm.Error = ""
	sm.Success = false
	sm.Data = nil
	sm.TimeIn = time.Time{}
	sm.TimeOut = time.Time{}
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
