package pumpdump

import (
	"io"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Book struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewBook() *Book {
	book := &Book{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	book.algo = nomagique.Number(
		algorithm.NewBookflowSample(datura.Acquire("pumpdump", datura.APPJSON)),
		equation.NewBookflow(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.BookflowInputKeys,
		})),
		probability.NewClassifier(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"loadedScore",
				"spoofScore",
				"thinScore",
				"neutralScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryLoadedImbalance)),
				float64(logic.CategoryIndex(logic.CategorySpoofTrap)),
				float64(logic.CategoryIndex(logic.CategoryBookThinning)),
				float64(logic.CategoryIndex(logic.CategoryDenseNeutrality)),
			},
		})),
	)

	return book
}

func (book *Book) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if err := transport.NewFlipFlop(
		datura.NewRWCStream(frame), book.algo,
	); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return frame
}
