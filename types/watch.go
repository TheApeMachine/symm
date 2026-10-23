package types

import (
	"sync"

	"github.com/theapemachine/symm/data"
)

/*
uiWatch holds symbols the desk currently holds as open virtual lots. The
learning route publishes sequitur and cognition/match for these symbols as
well as the dashboard focus, so open position cards stay live.
*/
var uiWatch sync.Map

func SetVirtualOpen(symbol string, open bool) {
	if symbol == "" {
		return
	}

	if open {
		uiWatch.Store(symbol, struct{}{})
		return
	}

	uiWatch.Delete(symbol)
}

func IsVirtualOpen(symbol string) bool {
	if symbol == "" {
		return false
	}

	_, ok := uiWatch.Load(symbol)

	return ok
}

func isFocusOrVirtualOpen(value *data.Value) bool {
	if value == nil {
		return false
	}

	if isFocus(value) {
		return true
	}

	return IsVirtualOpen(value.Scope)
}
