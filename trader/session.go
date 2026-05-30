package trader

import "sync"

/*
orderSession tracks in-flight client order ids for both paper and live desks.
*/
type orderSession struct {
	intents      sync.Map
	pendingEntry sync.Map
}

func (session *orderSession) trackEntry(clOrdID, symbol string, intent orderIntent) {
	if clOrdID == "" {
		return
	}

	session.intents.Store(clOrdID, intent)
	session.pendingEntry.Store(symbol, struct{}{})
}

func (session *orderSession) trackExit(clOrdID string, intent orderIntent) {
	if clOrdID == "" {
		return
	}

	session.intents.Store(clOrdID, intent)
}

func (session *orderSession) dropIntent(clOrdID, symbol string) {
	if clOrdID != "" {
		session.intents.Delete(clOrdID)
	}

	if symbol != "" {
		session.pendingEntry.Delete(symbol)
	}
}

func (session *orderSession) intentFor(clOrdID string) (orderIntent, bool) {
	value, ok := session.intents.Load(clOrdID)

	if !ok {
		return orderIntent{}, false
	}

	intent, ok := value.(orderIntent)

	return intent, ok
}

func (session *orderSession) HasPendingEntry(symbol string) bool {
	_, ok := session.pendingEntry.Load(symbol)

	return ok
}

func (session *orderSession) entryBound(clOrdID string) bool {
	intent, ok := session.intentFor(clOrdID)

	return ok && intent.entryBound
}

func (session *orderSession) markEntryBound(clOrdID string) {
	value, ok := session.intents.Load(clOrdID)

	if !ok {
		return
	}

	intent, intentOK := value.(orderIntent)

	if !intentOK {
		return
	}

	intent.entryBound = true
	session.intents.Store(clOrdID, intent)
}
