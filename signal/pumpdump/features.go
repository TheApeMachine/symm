package pumpdump

import (
	"context"
	"io"

	"github.com/theapemachine/datura"
	feed "github.com/theapemachine/symm/signal"
)

type Features struct {
	ctx          context.Context
	cancel       context.CancelFunc
	entity       string
	scope        string
	crossSection *CrossSection
	trade        *feed.Trade
	book         *feed.Book
}

func NewFeatures(
	ctx context.Context,
	crossSection *CrossSection,
	trade *feed.Trade,
	book *feed.Book,
) *Features {
	ctx, cancel := context.WithCancel(ctx)

	return &Features{
		ctx:          ctx,
		cancel:       cancel,
		entity:       "features",
		crossSection: crossSection,
		trade:        trade,
		book:         book,
	}
}

func (features *Features) Entity() string {
	return features.entity
}

func (features *Features) Snapshot() ScopeSnapshot {
	snapshot := TradeScopeSnapshot(features.trade, features.scope)

	if snapshot.Price > 0 {
		return snapshot
	}

	return BookScopeSnapshot(features.book, features.scope)
}

func (features *Features) Read(p []byte) (int, error) {
	if !features.crossSection.Ready(features.scope) {
		return 0, io.EOF
	}

	snapshot := features.Snapshot()

	if snapshot.Price <= 0 || snapshot.Spread <= 0 {
		return 0, io.EOF
	}

	payload, ok := features.crossSection.verticalityPayload(
		features.scope,
		snapshot.Move,
		snapshot.Precursor,
	)

	if !ok || len(payload) == 0 {
		return 0, io.EOF
	}

	artifact := datura.Acquire("verticality-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(features.scope)
	artifact.WithPayload(feed.EncodePayload(payload...))

	return artifact.Read(p)
}

func (features *Features) Close() error {
	features.cancel()

	return nil
}
