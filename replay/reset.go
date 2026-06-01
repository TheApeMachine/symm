package replay

/*
ResetShared drops the cached replay hub so the next Open starts a fresh playback.
Eval and tune batches call this between replays in the same process.
*/
func ResetShared() {
	sharedHub.mu.Lock()
	defer sharedHub.mu.Unlock()

	sharedHub.path = ""
	sharedHub.hub = nil
}
