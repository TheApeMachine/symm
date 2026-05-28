package market

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/core"
)

var ErrNotInstrument = errors.New("not an instrument event")

/*
ExecutionVenue is the execution venue for the instrument channel subscribe params.
*/
type ExecutionVenue string

const (
	ExecutionVenueInternational     ExecutionVenue = "international"
	ExecutionVenueBitnomialExchange ExecutionVenue = "bitnomial-exchange"
)

/*
AssetStatus is the status of an asset.
*/
type AssetStatus string

const (
	AssetStatusDepositOnly                AssetStatus = "depositonly"
	AssetStatusDisabled                   AssetStatus = "disabled"
	AssetStatusEnabled                    AssetStatus = "enabled"
	AssetStatusFundingTemporarilyDisabled AssetStatus = "fundingtemporarilydisabled"
	AssetStatusWithdrawalOnly             AssetStatus = "withdrawalonly"
	AssetStatusWorkInProgress             AssetStatus = "workinprogress"
)

/*
PairStatus is the status of a trading pair.
*/
type PairStatus string

const (
	PairStatusCancelOnly     PairStatus = "cancel_only"
	PairStatusDelisted       PairStatus = "delisted"
	PairStatusLimitOnly      PairStatus = "limit_only"
	PairStatusMaintenance    PairStatus = "maintenance"
	PairStatusOnline         PairStatus = "online"
	PairStatusPostOnly       PairStatus = "post_only"
	PairStatusReduceOnly     PairStatus = "reduce_only"
	PairStatusWorkInProgress PairStatus = "work_in_progress"
)

/*
InstrumentUpdateType distinguishes snapshot from incremental instrument channel updates.
*/
type InstrumentUpdateType string

const (
	InstrumentUpdateTypeSnapshot InstrumentUpdateType = "snapshot"
	InstrumentUpdateTypeUpdate   InstrumentUpdateType = "update"
)

type Asset struct {
	ID               string      `json:"id"`
	Status           AssetStatus `json:"status"`
	Precision        int         `json:"precision"`
	PrecisionDisplay int         `json:"precision_display"`
	Borrowable       bool        `json:"borrowable"`
	CollateralValue  float64     `json:"collateral_value"`
	MarginRate       float64     `json:"margin_rate"`
	Multiplier       float64     `json:"multiplier,omitempty"`
}

/*
Instrument is a tradeable pair from the Kraken WebSocket v2 instrument channel.
*/
type Instrument struct {
	Symbol             string     `json:"symbol"`
	Base               string     `json:"base"`
	Quote              string     `json:"quote"`
	Status             PairStatus `json:"status"`
	QtyPrecision       int        `json:"qty_precision"`
	QtyIncrement       float64    `json:"qty_increment"`
	PricePrecision     int        `json:"price_precision"`
	CostPrecision      int        `json:"cost_precision"`
	Marginable         bool       `json:"marginable"`
	HasIndex           bool       `json:"has_index"`
	CostMin            float64    `json:"cost_min"`
	MarginInitial      float64    `json:"margin_initial,omitempty"`
	PositionLimitLong  int        `json:"position_limit_long,omitempty"`
	PositionLimitShort int        `json:"position_limit_short,omitempty"`
	TickSize           float64    `json:"tick_size,omitempty"`
	PriceIncrement     float64    `json:"price_increment"`
	QtyMin             float64    `json:"qty_min"`
}

type InstrumentData struct {
	Assets []Asset      `json:"assets"`
	Pairs  []Instrument `json:"pairs"`
}

type InstrumentMessage struct {
	Channel string               `json:"channel"`
	Type    InstrumentUpdateType `json:"type"`
	Data    InstrumentData       `json:"data"`
}

/*
AssetPair maps a websocket instrument record into the shared asset pair shape.
*/
func (instrument Instrument) AssetPair() asset.Pair {
	return asset.Pair{
		Wsname:       instrument.Symbol,
		Altname:      instrument.Symbol,
		Base:         instrument.Base,
		Quote:        instrument.Quote,
		Costmin:      strconv.FormatFloat(instrument.CostMin, 'f', -1, 64),
		CostDecimals: instrument.CostPrecision,
		PairDecimals: instrument.PricePrecision,
		LotDecimals:  instrument.QtyPrecision,
		Status:       string(instrument.Status),
	}
}

/*
Parse decodes an instrument channel snapshot or update frame from payload.
*/
func (instrumentMessage *InstrumentMessage) Parse(payload []byte) error {
	channel, err := ChannelName(payload)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotInstrument, err)
	}

	if channel != core.ChannelInstrument {
		return fmt.Errorf("%w: channel=%q", ErrNotInstrument, channel)
	}

	if err = json.Unmarshal(payload, instrumentMessage); err != nil {
		return fmt.Errorf("parse instrument frame: %w", err)
	}

	return nil
}
