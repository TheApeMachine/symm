package trader

/*
drainOrderEvents processes pending paper fills and acks. Tests call this after
manage/enter when PaperOrderLatency is zero so the simulated live pipeline
completes synchronously.
*/
func drainOrderEvents(crypto *Crypto) {
	if crypto == nil {
		return
	}

	for {
		progressed := false

		if crypto.paper != nil {
			select {
			case fill := <-crypto.paper.Fills():
				crypto.handleOrderFill(fill)
				progressed = true
			default:
			}

			select {
			case ack := <-crypto.paper.Acks():
				crypto.handleOrderAck(ack)
				progressed = true
			default:
			}
		}

		if !progressed {
			return
		}
	}
}
