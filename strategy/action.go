package strategy

/*
Action is one decision the learner can take. The vocabulary is minimal and
semantically opaque to the cognition layer: the learner sees region-token
context and chooses one of the currently legal actions. It does not know why
any action is correct.

When flat:  {Enter, Wait}
When holding: {Exit, Wait}
*/
type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

/*
LegalActions returns the actions available to the learner given its current
position state. A flat learner may enter or wait. A holding learner may exit
or wait.
*/
func LegalActions(holding bool) []Action {
	if holding {
		return []Action{ActionExit, ActionWait}
	}

	return []Action{ActionEnter, ActionWait}
}
