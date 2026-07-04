package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
dispatch forwards allocated actions into the broker execution path.
*/
func (crypto *Crypto) dispatch(actions []*datura.Artifact) error {
	if len(actions) == 0 {
		return nil
	}

	if crypto == nil || crypto.desk == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: broker desk unavailable",
			nil,
		))
	}

	return crypto.desk.Update(actions)
}
