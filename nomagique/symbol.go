package nomagique

import (
	"fmt"
	"sync"
)

/*
Symbol is an interned numeric identifier. Primitive hot paths use Symbol as a
direct Frame offset and never hash or compare strings.
*/
type Symbol uint16

const (
	// MaxSlots bounds both the registry and the universal Frame representation.
	// The capacity leaves room for the fixed 128-sample histories used by Ignition
	// while retaining startup space for application-owned symbols.
	MaxSlots = 1024
)

var symbols = struct {
	sync.RWMutex
	byName map[string]Symbol
	names  [MaxSlots]string
	count  int
}{
	byName: make(map[string]Symbol, MaxSlots),
}

/*
Intern returns the stable slot assigned to name. Registry work is expected to
happen during package initialization, outside event-processing hot paths.
*/
func Intern(name string) (Symbol, error) {
	if name == "" {
		return 0, fmt.Errorf("nomagique: symbol name is blank")
	}

	symbols.RLock()
	symbol, found := symbols.byName[name]
	symbols.RUnlock()

	if found {
		return symbol, nil
	}

	symbols.Lock()
	defer symbols.Unlock()

	if symbol, found = symbols.byName[name]; found {
		return symbol, nil
	}

	if symbols.count >= MaxSlots {
		return 0, fmt.Errorf(
			"nomagique: symbol registry is full (%d slots)",
			MaxSlots,
		)
	}

	symbol = Symbol(symbols.count)
	symbols.count++
	symbols.byName[name] = symbol
	symbols.names[int(symbol)] = name

	return symbol, nil
}

/*
MustIntern returns the stable slot assigned to name and panics when the static
registry cannot accept it. It is intended for package-level symbol declarations.
*/
func MustIntern(name string) Symbol {
	symbol, err := Intern(name)

	if err != nil {
		panic(err)
	}

	return symbol
}

/*
SymbolName resolves a symbol for diagnostics and serialization. It is not part
of the numeric hot path.
*/
func SymbolName(symbol Symbol) (string, bool) {
	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		return "", false
	}

	symbols.RLock()
	defer symbols.RUnlock()

	name := symbols.names[index]

	return name, name != ""
}

/*
RegisteredSymbols returns the number of interned symbols.
*/
func RegisteredSymbols() int {
	symbols.RLock()
	defer symbols.RUnlock()

	return symbols.count
}
