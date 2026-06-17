package signal

import (
	"time"

	"github.com/theapemachine/datura"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/buffer"
	"github.com/theapemachine/symm/signal/codec"
)

type (
	Trade           = buffer.Trade
	Book            = buffer.Book
	Ticker          = buffer.Ticker
	TradeRecord     = buffer.TradeRecord
	BookRecord      = buffer.BookRecord
	TickerRecord    = buffer.TickerRecord
	BookLevelRecord = buffer.BookLevelRecord
	SymbolWindow    = buffer.SymbolWindow
	TradeSnapshot   = buffer.TradeSnapshot
	TickerSnapshot  = buffer.TickerSnapshot
)

const FeatureFrameSize = codec.FeatureFrameSize

var (
	NewTrade               = buffer.NewTrade
	NewBook                = buffer.NewBook
	NewTicker              = buffer.NewTicker
	EncodePayload          = codec.EncodePayload
	ValidFloatPayload      = codec.ValidFloatPayload
	ValidExcitationPayload = codec.ValidExcitationPayload
	ValidFlowPayload       = codec.ValidFlowPayload
	ValidDecayPayload      = codec.ValidDecayPayload
	BookQualityMinFloats   = codec.BookQualityMinFloats
	VerticalityMinFloats   = codec.VerticalityMinFloats
	ConvictionMinFloats    = codec.ConvictionMinFloats
	FluidflowMinFloats     = codec.FluidflowMinFloats
	LagMinFloats           = codec.LagMinFloats
	ManifoldMinFloats      = codec.ManifoldMinFloats
	CohortMinFloats        = codec.CohortMinFloats
	ReadFeatureArtifact    = codec.ReadFeatureArtifact
	MaxFeatureFloats       = codec.MaxFeatureFloats
	TrimLargestFloats      = codec.TrimLargestFloats
	TrimHistoryTails       = codec.TrimHistoryTails
	ArtifactPayload        = codec.ArtifactPayload
)

/*
PeekElementOK reads one typed field from a JSON feed element.
*/
func PeekElementOK[T any](element []byte, path string) (T, bool) {
	return codec.PeekElementOK[T](element, path)
}

func ElementTime(element []byte, key string) (time.Time, bool) {
	return codec.ElementTime(element, key)
}

func EachBookLevelElement(
	element []byte,
	key string,
	visit func(price float64, qty float64),
) {
	codec.EachBookLevelElement(element, key, visit)
}

func UnmarshalElement(element []byte, dest any) error {
	return codec.UnmarshalElement(element, dest)
}

func TouchSpread(prices []float64) (float64, bool) {
	return codec.TouchSpread(prices)
}

func PayloadSymbols(artifact *datura.Artifact) []string {
	return krakenmarket.PayloadSymbols(artifact)
}

func VisitTickers(
	artifact *datura.Artifact,
	visit func(symbol string, last float64) bool,
) {
	krakenmarket.VisitTickers(artifact, visit)
}

func TickerFeedArtifact(updates krakenmarket.TickerUpdates) *datura.Artifact {
	return krakenmarket.TickerFeedArtifact(updates)
}

func TradeFeedArtifact(updates krakenmarket.TradeUpdates) *datura.Artifact {
	return krakenmarket.TradeFeedArtifact(updates)
}

func BookFeedArtifact(updates krakenmarket.BookUpdates) *datura.Artifact {
	return krakenmarket.BookFeedArtifact(updates)
}
