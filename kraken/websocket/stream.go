package websocket

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Stream struct {
	observers map[string][]chan []byte
	buffer    int
}

func NewStream(buffer int) *Stream {
	return &Stream{
		observers: map[string][]chan []byte{},
		buffer:    buffer,
	}
}

func (stream *Stream) Observe(channel string) chan []byte {
	out := make(chan []byte, stream.buffer)

	if stream.observers == nil {
		stream.observers = map[string][]chan []byte{}
	}

	stream.observers[channel] = append(stream.observers[channel], out)
	return out
}

func (stream *Stream) Receive(raw []byte) string {
	channel := stream.Channel(raw)
	method := false

	if channel == "" {
		channel = stream.Method(raw)
		method = channel != ""
	}

	if channel == "" {
		return ""
	}

	if len(stream.observers[channel]) == 0 {
		return channel
	}

	data := raw
	if !method && channel != "book" {
		data = stream.Data(raw)
		if len(data) == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: frame data required",
				nil,
			))
			return channel
		}
	}

	for _, observer := range stream.observers[channel] {
		select {
		case observer <- data:
		default:
		}
	}

	return channel
}

func (stream *Stream) Method(raw []byte) string {
	node, err := sonic.Get(raw, "method")
	if err != nil || !node.Exists() {
		return ""
	}

	method, err := node.String()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: method string required",
			err,
		))
		return ""
	}

	return strings.TrimSpace(method)
}

func (stream *Stream) Channel(raw []byte) string {
	node, err := sonic.Get(raw, "channel")
	if err != nil || !node.Exists() {
		return ""
	}

	channel, err := node.String()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: channel string required",
			err,
		))
		return ""
	}

	return strings.TrimSpace(channel)
}

func (stream *Stream) Data(raw []byte) []byte {
	node, err := sonic.Get(raw, "data")
	if err != nil || !node.Exists() {
		return nil
	}

	data, err := node.Raw()
	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: data payload required",
			err,
		))
		return nil
	}

	return []byte(data)
}
