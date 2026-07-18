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

func (desk *Desk) evict(symbol string) {
	if symbol == "" {
		return
	}

	if value, ok := desk.positions.Load(symbol); ok {
		value.(*Position).Close()
	}

	desk.positions.Delete(symbol)

	if desk.balance != nil && desk.balance.holdings != nil {
		desk.balance.holdings.Delete(symbol)
	}
}
