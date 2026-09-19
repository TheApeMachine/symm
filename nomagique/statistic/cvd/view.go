package cvd

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Field pickers projecting Reading into float64 metrics.
No structs, pure Value functions.
*/

type TradeCount types.Value[Reading, float64]
func NewTradeCount() TradeCount { return func(r Reading) float64 { return r.TradeCount } }

type BuyCount types.Value[Reading, float64]
func NewBuyCount() BuyCount { return func(r Reading) float64 { return r.BuyCount } }

type SellCount types.Value[Reading, float64]
func NewSellCount() SellCount { return func(r Reading) float64 { return r.SellCount } }

type BuyQty types.Value[Reading, float64]
func NewBuyQty() BuyQty { return func(r Reading) float64 { return r.BuyQty } }

type SellQty types.Value[Reading, float64]
func NewSellQty() SellQty { return func(r Reading) float64 { return r.SellQty } }

type GrossQty types.Value[Reading, float64]
func NewGrossQty() GrossQty { return func(r Reading) float64 { return r.GrossQty } }

type NetQty types.Value[Reading, float64]
func NewNetQty() NetQty { return func(r Reading) float64 { return r.NetQty } }

type BuyNotional types.Value[Reading, float64]
func NewBuyNotional() BuyNotional { return func(r Reading) float64 { return r.BuyNotional } }

type SellNotional types.Value[Reading, float64]
func NewSellNotional() SellNotional { return func(r Reading) float64 { return r.SellNotional } }

type GrossNotional types.Value[Reading, float64]
func NewGrossNotional() GrossNotional { return func(r Reading) float64 { return r.GrossNotional } }

type NetNotional types.Value[Reading, float64]
func NewNetNotional() NetNotional { return func(r Reading) float64 { return r.NetNotional } }

type MeanNotional types.Value[Reading, float64]
func NewMeanNotional() MeanNotional { return func(r Reading) float64 { return r.MeanNotional } }

type CVD types.Value[Reading, float64]
func NewCVD() CVD { return func(r Reading) float64 { return r.CVD } }

type CND types.Value[Reading, float64]
func NewCND() CND { return func(r Reading) float64 { return r.CND } }

type Epoch types.Value[Reading, float64]
func NewEpoch() Epoch { return func(r Reading) float64 { return r.Epoch } }

type SignedCount types.Value[Reading, float64]
func NewSignedCount() SignedCount { return func(r Reading) float64 { return r.SignedCount } }

type SignedNet types.Value[Reading, float64]
func NewSignedNet() SignedNet { return func(r Reading) float64 { return r.SignedNet } }
