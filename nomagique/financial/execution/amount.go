package execution

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
)

/* amountArgument reads exact decimal text through the canonical monetary parser. */
func amountArgument(read func() (string, error), name string) (*decimal.Decimal, error) {
	text, err := read()
	if err != nil {
		return nil, errnie.Error(err)
	}
	return core.ReadDecimal([]byte(text), name)
}

/* oneDecision admits only the one model classification for this inventory state. */
func oneDecision(choices capnp.DataList) ([]byte, error) {
	var action []byte
	for index := range choices.Len() {
		value, err := choices.At(index)
		if err != nil {
			return nil, errnie.Error(err)
		}
		if len(value) == 0 {
			continue
		}
		if action != nil {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "execution account: conflicting decisions for one inventory state", nil))
		}
		action = value
	}
	return action, nil
}
