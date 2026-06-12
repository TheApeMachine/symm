package rawbus

import (
	"errors"

	"github.com/theapemachine/errnie"
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
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: nil message",
			nil,
		))
	}

	messageType := TypeFrom(row.Type)

	if messageType != TypeActions && messageType != TypeOrder {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: expected action message, got %q",
			errors.New(row.Type),
		))
	}

	action, ok := row.Value.(*logic.Action)

	if !ok || action == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: invalid action payload for %q",
			errors.New(row.Type),
		))
	}

	return action, nil
}

/*
DecodeExecutions extracts execution rows from a raw executions frame.
*/
func DecodeExecutions(row *qpool.QValue[any]) ([]user.Execution, error) {
	if row == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: nil message",
			nil,
		))
	}

	if TypeFrom(row.Type) != TypeExecutions {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: expected executions message, got %q",
			errors.New(row.Type),
		))
	}

	executions, ok := row.Value.([]user.Execution)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: invalid executions payload",
			nil,
		))
	}

	return executions, nil
}

/*
DecodeOrderUpdates extracts private order updates from a raw orders frame.
*/
func DecodeOrderUpdates(row *qpool.QValue[any]) ([]trading.OrderUpdate, error) {
	if row == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: nil message",
			nil,
		))
	}

	if TypeFrom(row.Type) != TypeOrders {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: expected orders message, got %q",
			errors.New(row.Type),
		))
	}

	updates, ok := row.Value.([]trading.OrderUpdate)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"rawbus: invalid orders payload",
			nil,
		))
	}

	return updates, nil
}
