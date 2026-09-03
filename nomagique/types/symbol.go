package types

import (
	"fmt"
	"strings"
	"sync"
)

/*
Symbol is an interned numeric identifier. Primitive hot paths use Symbol as a
direct Frame offset and never hash or compare strings.
*/
type Symbol uint16

const (
	frameMaskWordBits  = 64
	fullStackMaskWords = 128
	// MaxSlots bounds both the registry and the universal Frame representation.
	// It must cover every distinct interned symbol across the whole program:
	// each namespaced estimator series reserves its own sample-slot block.
	MaxSlots = frameMaskWordBits * fullStackMaskWords
)

type symbolRegistry struct {
	sync.RWMutex
	byName   map[string]Symbol
	names    []string
	capacity int
}

func newSymbolRegistry(capacity int) *symbolRegistry {
	if capacity < 0 || capacity > MaxSlots {
		capacity = MaxSlots
	}

	return &symbolRegistry{
		byName:   make(map[string]Symbol, capacity),
		names:    make([]string, capacity),
		capacity: capacity,
	}
}

func (registry *symbolRegistry) intern(name string) (Symbol, error) {
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("nomagique: symbol name is blank")
	}

	registry.RLock()
	symbol, found := registry.byName[name]
	registry.RUnlock()

	if found {
		return symbol, nil
	}

	registry.Lock()
	defer registry.Unlock()

	if symbol, found = registry.byName[name]; found {
		return symbol, nil
	}

	if len(registry.byName) >= registry.capacity {
		return 0, fmt.Errorf(
			"nomagique: symbol registry is full (%d slots)",
			registry.capacity,
		)
	}

	symbol = Symbol(len(registry.byName))
	registry.byName[name] = symbol
	registry.names[int(symbol)] = name

	return symbol, nil
}

func (registry *symbolRegistry) name(symbol Symbol) (string, bool) {
	index := int(symbol)

	if index < 0 || index >= registry.capacity {
		return "", false
	}

	registry.RLock()
	defer registry.RUnlock()

	name := registry.names[index]

	return name, name != ""
}

func (registry *symbolRegistry) registered() int {
	registry.RLock()
	defer registry.RUnlock()

	return len(registry.byName)
}

var symbols = newSymbolRegistry(MaxSlots)

/*
Intern returns the stable slot assigned to name. Registry work is expected to
happen during package initialization, outside event-processing hot paths.
*/
func Intern(name string) (Symbol, error) {
	return symbols.intern(name)
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
	return symbols.name(symbol)
}

/*
String returns the human-readable string representation of this Symbol.
*/
func (symbol Symbol) String() string {
	name, _ := SymbolName(symbol)
	return name
}

/*
RegisteredSymbols returns the number of interned symbols.
*/
func RegisteredSymbols() int {
	return symbols.registered()
}

