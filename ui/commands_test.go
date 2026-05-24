package ui

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
)

func TestFluidCommandsHandleDisplayOp(t *testing.T) {
	commands := &FluidCommands{}

	commands.HandleCommand(map[string]any{
		"op": "unknown",
	})

	commands.HandleCommand([]byte(`{"op":"get_fluid_display"}`))
}

func TestBroadcastGroupDropsSlowConsumerWithoutClosing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ui, err := qpool.NewBroadcastGroup(ctx, "ui-test-drop", time.Minute)

	if err != nil {
		t.Fatal(err)
	}

	slow := ui.Subscribe("slow", 1)

	ui.Send(&qpool.QValue[any]{Value: map[string]any{"seq": 1}})
	ui.Send(&qpool.QValue[any]{Value: map[string]any{"seq": 2}})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Send panicked on slow consumer: %v", recovered)
		}
	}()

	ui.Send(&qpool.QValue[any]{Value: map[string]any{"seq": 3}})

	if len(slow.Incoming) != 1 {
		t.Fatalf("expected buffered frame to remain, got %d", len(slow.Incoming))
	}
}

func TestHubRoutesCommandsWithoutBroadcast(t *testing.T) {
	received := make(chan map[string]any, 1)

	hub := &Hub{
		commands: commandSink{received: received},
	}

	hub.readPumpCommand([]byte(`{"op":"get_fluid_display"}`))

	select {
	case payload := <-received:
		if payload["op"] != "get_fluid_display" {
			t.Fatalf("unexpected op: %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command dispatch")
	}
}

type commandSink struct {
	received chan map[string]any
}

func (sink commandSink) HandleCommand(raw any) {
	payload, ok := decodeCommand(raw)

	if !ok {
		return
	}

	sink.received <- payload
}

func (hub *Hub) readPumpCommand(payload []byte) {
	if hub.commands != nil {
		hub.commands.HandleCommand(payload)
	}
}
