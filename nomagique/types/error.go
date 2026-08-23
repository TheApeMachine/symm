package types

import (
	"errors"
	"fmt"
)

func primitiveError(message string) error {
	return errors.New("nomagique: " + message)
}

func symbolLabel(symbol Symbol) string {
	if name, found := SymbolName(symbol); found {
		return fmt.Sprintf("%q", name)
	}

	return fmt.Sprintf("symbol/%d", symbol)
}
