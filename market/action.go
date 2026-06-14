package market

import (
	"context"

	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/logic"
)

type Action struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	actions structure.Ring[logic.Action]
}
