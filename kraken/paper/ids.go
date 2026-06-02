package paper

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

/*
Identifier mints Kraken-shaped identifiers for simulated private events.
*/
type Identifier struct{}

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
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return hex.EncodeToString(buffer)
}
