package ui

/*
isManifoldBinary reports whether payload is an SMF1 lattice frame so Publish
can fan it out as a WebSocket BinaryMessage without retaining it for replay.
*/
func isManifoldBinary(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}

	if payload[0] != 'S' || payload[1] != 'M' || payload[2] != 'F' || payload[3] != '1' {
		return false
	}

	kind := payload[4]
	return kind >= 1 && kind <= 5
}
