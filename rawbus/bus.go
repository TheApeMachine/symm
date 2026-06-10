package rawbus

import (
	"fmt"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

/*
Send publishes one typed frame on the raw broadcast bus.
*/
func Send(bus *internal.Bus, messageType Type, value any) error {
	return bus.Send(internal.ChannelRaw, messageType.String(), value)
}

/*
TypeOf returns the typed raw discriminator for one bus row.
*/
func TypeOf(row *qpool.QValue[any]) Type {
	if row == nil {
		return ""
	}

	return TypeFrom(row.Type)
}

/*
DecodeAction extracts a playbook action from a raw actions or order frame.
*/
func DecodeAction(row *qpool.QValue[any]) (*logic.Action, error) {
	if row == nil {
		return nil, fmt.Errorf("rawbus: nil message")
	}

	messageType := TypeFrom(row.Type)

	if messageType != TypeActions && messageType != TypeOrder {
		return nil, fmt.Errorf("rawbus: expected action message, got %q", row.Type)
	}

	action, ok := row.Value.(*logic.Action)

	if !ok || action == nil {
		return nil, fmt.Errorf("rawbus: invalid action payload for %q", row.Type)
	}

	return action, nil
}

/*
DecodeExecutions extracts execution rows from a raw executions frame.
*/
func DecodeExecutions(row *qpool.QValue[any]) ([]user.Execution, error) {
	if row == nil {
		return nil, fmt.Errorf("rawbus: nil message")
	}

	if TypeFrom(row.Type) != TypeExecutions {
		return nil, fmt.Errorf("rawbus: expected executions message, got %q", row.Type)
	}

	executions, ok := row.Value.([]user.Execution)

	if !ok {
		return nil, fmt.Errorf("rawbus: invalid executions payload")
	}

	return executions, nil
}

/*
DecodeOrderUpdates extracts private order updates from a raw orders frame.
*/
func DecodeOrderUpdates(row *qpool.QValue[any]) ([]trading.OrderUpdate, error) {
	if row == nil {
		return nil, fmt.Errorf("rawbus: nil message")
	}

	if TypeFrom(row.Type) != TypeOrders {
		return nil, fmt.Errorf("rawbus: expected orders message, got %q", row.Type)
	}

	updates, ok := row.Value.([]trading.OrderUpdate)

	if !ok {
		return nil, fmt.Errorf("rawbus: invalid orders payload")
	}

	return updates, nil
}
