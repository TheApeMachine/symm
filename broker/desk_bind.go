package broker

/*
bind is retained for Initialize/Buy ordering. Channel handlers live on each
Position so inventory mark updates stay on Holding and closed lots Unsubscribe.
*/
func (desk *Desk) bind() {
	if desk == nil || !desk.bound.CompareAndSwap(false, true) {
		return
	}
}

/*
evict removes a terminal lot from the desk maps, then closes channel handlers.
LoadAndDelete runs first so Position.Close cannot re-enter through onTerminal.
*/
func (desk *Desk) evict(symbol string) {
	if symbol == "" {
		return
	}

	value, ok := desk.positions.LoadAndDelete(symbol)

	if ok {
		position := value.(*Position)
		position.onTerminal = nil
		position.Close()
	}

	if desk.balance != nil {
		desk.balance.closeHolding(symbol)
	}
}
