package hawkes

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Field pickers projecting Reading into float64 metrics.
No structs, pure Value functions.
*/

type EventCount types.Value[Reading, float64]
func NewEventCount() EventCount { return func(r Reading) float64 { return r.EventCount } }

type BuyCount types.Value[Reading, float64]
func NewBuyCount() BuyCount { return func(r Reading) float64 { return r.BuyCount } }

type SellCount types.Value[Reading, float64]
func NewSellCount() SellCount { return func(r Reading) float64 { return r.SellCount } }

type BuyFraction types.Value[Reading, float64]
func NewBuyFraction() BuyFraction { return func(r Reading) float64 { return r.BuyFraction } }

type SellFraction types.Value[Reading, float64]
func NewSellFraction() SellFraction { return func(r Reading) float64 { return r.SellFraction } }

type ArrivalRate types.Value[Reading, float64]
func NewArrivalRate() ArrivalRate { return func(r Reading) float64 { return r.ArrivalRate } }

type BuyRate types.Value[Reading, float64]
func NewBuyRate() BuyRate { return func(r Reading) float64 { return r.BuyRate } }

type SellRate types.Value[Reading, float64]
func NewSellRate() SellRate { return func(r Reading) float64 { return r.SellRate } }

type ConditionalIntensity types.Value[Reading, float64]
func NewConditionalIntensity() ConditionalIntensity { return func(r Reading) float64 { return r.Lambda } }

type BuyIntensity types.Value[Reading, float64]
func NewBuyIntensity() BuyIntensity { return func(r Reading) float64 { return r.LambdaBuy } }

type SellIntensity types.Value[Reading, float64]
func NewSellIntensity() SellIntensity { return func(r Reading) float64 { return r.LambdaSell } }

type SpectralRadius types.Value[Reading, float64]
func NewSpectralRadius() SpectralRadius { return func(r Reading) float64 { return r.SpectralRadius } }
