package manifold

import (
	"crypto/sha256"
	"encoding/binary"
)

type orderIdentity struct{ symbol, orderID string }

// orderContentID uses length-delimited identities, so neither symbol separators
// nor order-ID suffixes can alias concatenations. Bit 62 reserves the order
// namespace away from the small symbol-token IDs used by probe particles.
func orderContentID(identity orderIdentity) int64 {
	payload := make([]byte, 16+len(identity.symbol)+len(identity.orderID))
	binary.LittleEndian.PutUint64(payload, uint64(len(identity.symbol)))
	copy(payload[8:], identity.symbol)
	offset := 8 + len(identity.symbol)
	binary.LittleEndian.PutUint64(payload[offset:], uint64(len(identity.orderID)))
	copy(payload[offset+8:], identity.orderID)
	digest := sha256.Sum256(payload)
	return int64(binary.LittleEndian.Uint64(digest[:8])&((1<<62)-1) | (1 << 62))
}
