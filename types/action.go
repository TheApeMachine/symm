package types

type Action string

const (
	ActionEnter   Action = "enter"
	ActionExit    Action = "exit"
	ActionReduce  Action = "reduce"
	ActionHold    Action = "hold"
	ActionNothing Action = "nothing"
)

func (action Action) String() string {
	return string(action)
}
