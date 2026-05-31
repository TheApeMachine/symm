package market

/*
RequestBookFeedRestart asks the shared live book websocket to reconnect so Kraken
re-sends snapshots after a checksum divergence.
*/
func RequestBookFeedRestart() {
	if replayActive() {
		return
	}

	bookFeed.requestRestart()
}

func (sharedFeed *sharedFeed[T]) requestRestart() {
	sharedFeed.mu.Lock()
	defer sharedFeed.mu.Unlock()

	if !sharedFeed.running {
		return
	}

	sharedFeed.restartUpstreamLocked()
}
