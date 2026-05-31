package trader

import "sync"

/*
orderSession tracks in-flight client order ids for both paper and live desks.
*/
type orderSession struct {
	intents      sync.Map
	pendingEntry sync.Map
	pendingExit  sync.Map
}

func (session *orderSession) trackEntry(clOrdID, symbol string, intent orderIntent) {
	if clOrdID == "" || symbol == "" {
		return
	}

	session.intents.Store(clOrdID, intent)
	session.pendingEntry.Store(symbol, clOrdID)
}

func (session *orderSession) trackExit(clOrdID, symbol string, intent orderIntent) bool {
	if clOrdID == "" || symbol == "" {
		return false
	}

	if _, loaded := session.pendingExit.LoadOrStore(symbol, clOrdID); loaded {
		return false
	}

	session.intents.Store(clOrdID, intent)

	return true
}

func (session *orderSession) dropIntent(clOrdID, symbol string) {
	if clOrdID != "" {
		session.intents.Delete(clOrdID)
	}

	if symbol != "" {
		session.dropPending(&session.pendingEntry, clOrdID, symbol)
		session.dropPending(&session.pendingExit, clOrdID, symbol)
	}
}

func (session *orderSession) dropPending(pending *sync.Map, clOrdID, symbol string) {
	value, ok := pending.Load(symbol)

	if !ok {
		return
	}

	pendingID, idOK := value.(string)

	if clOrdID != "" && idOK && pendingID != clOrdID {
		return
	}

	pending.Delete(symbol)
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

func (session *orderSession) HasPendingExit(symbol string) bool {
	_, ok := session.pendingExit.Load(symbol)

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
