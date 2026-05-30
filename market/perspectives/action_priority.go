package perspectives

/*
IsEntryBlocked reports whether a playbook verdict forbids opening a position.
*/
func IsEntryBlocked(action ActionType) bool {
	return action == ActionDeny || action == ActionWait
}

/*
IsExitAction reports whether the verdict closes or flips an open position.
*/
func IsExitAction(action ActionType) bool {
	switch action {
	case ActionStopLoss, ActionTakeProfit, ActionShort:
		return true
	default:
		return false
	}
}

/*
ExitUrgency ranks exit actions so the trader can merge parallel playbooks.
Higher wins.
*/
func ExitUrgency(action ActionType) int {
	switch action {
	case ActionStopLoss:
		return 3
	case ActionShort:
		return 2
	case ActionTakeProfit:
		return 1
	default:
		return 0
	}
}

