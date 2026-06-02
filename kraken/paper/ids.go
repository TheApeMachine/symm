package paper

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

/*
Identifier mints Kraken-shaped identifiers for simulated private events.
*/
type Identifier struct{}

var fallbackCounter uint64

func NewIdentifier() *Identifier {
	return &Identifier{}
}

func (identifier *Identifier) OrderID() string {
	return "PAPER-" + identifier.hex(8)
}

func (identifier *Identifier) ExecID() string {
	return identifier.hex(16)
}

func (identifier *Identifier) ClOrdID() string {
	return "p" + identifier.hex(8)
}

func (identifier *Identifier) LedgerID() string {
	return identifier.hex(6)
}

func (identifier *Identifier) hex(byteCount int) string {
	buffer := make([]byte, byteCount)

	if _, err := rand.Read(buffer); err != nil {
		counter := atomic.AddUint64(&fallbackCounter, 1)
		seed := uint64(time.Now().UnixNano()) ^ counter

		for index := range buffer {
			if index%8 == 0 {
				seed = uint64(time.Now().UnixNano()) ^ atomic.LoadUint64(&fallbackCounter)
			}

			buffer[index] = byte(seed >> ((index % 8) * 8))
		}

		return hex.EncodeToString(buffer)
	}

	return hex.EncodeToString(buffer)
}
