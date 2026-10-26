package types

import "sync"

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
