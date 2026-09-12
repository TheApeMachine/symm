package data

type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionIdentify
	ActionRead
	ActionWrite
	ActionExecute
)

/*
Action makes any ActionType itself Actionable, so a query can carry its
intent as a plain value.
*/
func (actionType ActionType) Action() ActionType {
	return actionType
}

/*
Actionable is the capability of naming an operation against a store.
*/
type Actionable interface {
	Action() ActionType
}
