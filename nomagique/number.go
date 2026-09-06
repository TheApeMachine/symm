package nomagique

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Number constructs a live composition. Construction does not tick a stage.
func Number(stages ...core.Primitive) core.Primitive { return transport.NewPipe(stages...) }
