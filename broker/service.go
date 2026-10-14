package broker

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

/*
BalanceReport summarizes account cash, unrealized PnL, total equity, and asset holdings.
*/
type BalanceReport struct {
	Quote      string            `json:"quote,omitempty"`
	Cash       string            `json:"cash"`
	Unrealized string            `json:"unrealized"`
	Equity     string            `json:"equity"`
	Assets     map[string]string `json:"assets,omitempty"`
}

/*
Service exposes REST endpoints for broker operations and position lifecycle queries.
Positions and their lifecycles are owned directly by the Desk.
*/
type Service struct {
	desk *Desk
}

/*
NewService constructs a broker HTTP service bound to desk and optional balance.
*/
func NewService(desk *Desk) *Service {
	svc := &Service{
		desk: desk,
	}

	return svc
}

/*
LifeEvent records an order, fill, or risk adjustment event on a position.
*/
type LifeEvent struct {
	At      string         `json:"at"`
	VenueAt string         `json:"venue_at,omitempty"`
	Kind    string         `json:"kind"`
	Detail  string         `json:"detail"`
	Fields  map[string]any `json:"fields,omitempty"`
}

/*
LifeOutcome summarizes the economic results and duration of a closed or evaluated position.
*/
type LifeOutcome struct {
	VenueCost      float64  `json:"venue_cost"`
	VenueProceeds  float64  `json:"venue_proceeds"`
	VenueFees      float64  `json:"venue_fees"`
	VenueRealized  float64  `json:"venue_realized"`
	ShadowDefined  bool     `json:"shadow_defined"`
	ShadowCost     float64  `json:"shadow_cost"`
	ShadowProceeds float64  `json:"shadow_proceeds"`
	ShadowFees     float64  `json:"shadow_fees"`
	ShadowRealized float64  `json:"shadow_realized"`
	ShadowShort    float64  `json:"shadow_short"`
	ShadowUnpriced float64  `json:"shadow_unpriced"`
	HoldNs         int64    `json:"hold_ns"`
	ExpectedHoldNs int64    `json:"expected_hold_ns"`
	Triggers       []string `json:"triggers"`
}

/*
Lifecycle represents a complete or active position history.
*/
type Lifecycle struct {
	Id       string       `json:"id"`
	Symbol   string       `json:"symbol"`
	Status   string       `json:"status"`
	OpenedAt string       `json:"opened_at"`
	ClosedAt string       `json:"closed_at,omitempty"`
	Events   []LifeEvent  `json:"events"`
	Outcome  *LifeOutcome `json:"outcome,omitempty"`
}

/*
Performance summarizes aggregate trading performance across positions.
*/
type Performance struct {
	Closed           int      `json:"closed"`
	ShadowTrades     int      `json:"shadow_trades"`
	ShadowWins       int      `json:"shadow_wins"`
	ShadowWinRate    *float64 `json:"shadow_win_rate"`
	ShadowMeanReturn *float64 `json:"shadow_mean_return"`
	ShadowPnl        float64  `json:"shadow_pnl"`
	VenueWins        int      `json:"venue_wins"`
	VenueWinRate     *float64 `json:"venue_win_rate"`
	VenueMeanReturn  *float64 `json:"venue_mean_return"`
	VenuePnl         float64  `json:"venue_pnl"`
}

/*
LifecycleReport provides the unified position lifecycle and performance snapshot.
*/
type LifecycleReport struct {
	Lifecycles  []Lifecycle `json:"lifecycles"`
	Performance Performance `json:"performance"`
	OpenCount   int         `json:"open_count"`
}

/*
Register mounts the broker endpoints onto router.
*/
func (service *Service) Register(router fiber.Router) {
	if service == nil || service.desk == nil || router == nil {
		return
	}

	router.Get("/positions", service.positions)
	router.Get("/positions/lifecycle", service.lifecycle)
	router.Get("/balance", service.balanceReport)
	router.Get("/equity", service.balanceReport)
}

func (service *Service) positions(reqCtx fiber.Ctx) error {
	positions := make([]*Position, 0)

	if service.desk != nil && service.desk.positions != nil {
		service.desk.positions.Range(func(key, value any) bool {
			if list, ok := value.([]*Position); ok {
				for _, pos := range list {
					if !pos.Closed() {
						positions = append(positions, pos)
					}
				}
			}

			return true
		})
	}

	return reqCtx.JSON(positions)
}

func (service *Service) lifecycle(reqCtx fiber.Ctx) error {
	lifecycles := make([]Lifecycle, 0)
	var closedCount int
	var venueWins int
	var totalVenuePnl float64
	var totalVenueReturn float64
	var shadowTrades int
	var shadowWins int
	var totalShadowPnl float64
	var totalShadowReturn float64
	var openCount int

	if service.desk != nil {
		closedPositions := service.desk.Closed()

		for _, pos := range closedPositions {
			lc := pos.Lifecycle()

			if lc.Status != "closed" {
				continue
			}

			lifecycles = append(lifecycles, lc)

			if lc.Outcome != nil {
				closedCount++
				totalVenuePnl += lc.Outcome.VenueRealized

				if lc.Outcome.VenueRealized > 0 {
					venueWins++
				}

				if lc.Outcome.VenueCost > 0 {
					totalVenueReturn += lc.Outcome.VenueRealized / lc.Outcome.VenueCost
				}

				if lc.Outcome.ShadowDefined {
					shadowTrades++
					totalShadowPnl += lc.Outcome.ShadowRealized

					if lc.Outcome.ShadowRealized > 0 {
						shadowWins++
					}

					if lc.Outcome.ShadowCost > 0 {
						totalShadowReturn += lc.Outcome.ShadowRealized / lc.Outcome.ShadowCost
					}
				}
			}
		}

		if service.desk.positions != nil {
			service.desk.positions.Range(func(key, value any) bool {
				list, ok := value.([]*Position)

				if !ok {
					return true
				}

				for _, pos := range list {
					if !pos.Closed() {
						openCount++
					}
				}

				return true
			})
		}
	}

	var venueWinRate *float64
	var venueMeanReturn *float64

	if closedCount > 0 {
		rate := float64(venueWins) / float64(closedCount)
		venueWinRate = &rate

		mean := totalVenueReturn / float64(closedCount)
		venueMeanReturn = &mean
	}

	var shadowWinRate *float64
	var shadowMeanReturn *float64

	if shadowTrades > 0 {
		rate := float64(shadowWins) / float64(shadowTrades)
		shadowWinRate = &rate

		mean := totalShadowReturn / float64(shadowTrades)
		shadowMeanReturn = &mean
	}

	if shadowTrades == 0 && closedCount > 0 {
		shadowWinRate = venueWinRate
		shadowMeanReturn = venueMeanReturn
		shadowWins = venueWins
		shadowTrades = closedCount
		totalShadowPnl = totalVenuePnl
	}

	return reqCtx.JSON(LifecycleReport{
		Lifecycles: lifecycles,
		Performance: Performance{
			Closed:           closedCount,
			ShadowTrades:     shadowTrades,
			ShadowWins:       shadowWins,
			ShadowWinRate:    shadowWinRate,
			ShadowMeanReturn: shadowMeanReturn,
			ShadowPnl:        totalShadowPnl,
			VenueWins:        venueWins,
			VenueWinRate:     venueWinRate,
			VenueMeanReturn:  venueMeanReturn,
			VenuePnl:         totalVenuePnl,
		},
		OpenCount: openCount,
	})
}

func (service *Service) balanceReport(reqCtx fiber.Ctx) error {
	balance := service.desk.balance

	if balance == nil {
		return reqCtx.JSON(BalanceReport{
			Cash:       "0",
			Unrealized: "0",
			Equity:     "0",
		})
	}

	cash := "0"
	unrealized := "0"
	equity := "0"

	if balance.measurement != nil {
		for entry := range balance.measurement.Read("cash") {
			if entry.Metric != nil {
				cash = strconv.FormatFloat(entry.Metric.Raw(), 'f', -1, 64)
				break
			}
		}

		for entry := range balance.measurement.Read("unrealized") {
			if entry.Metric != nil {
				unrealized = strconv.FormatFloat(entry.Metric.Raw(), 'f', -1, 64)
				break
			}
		}

		for entry := range balance.measurement.Read("equity") {
			if entry.Metric != nil {
				equity = strconv.FormatFloat(entry.Metric.Raw(), 'f', -1, 64)
				break
			}
		}
	}

	return reqCtx.JSON(BalanceReport{
		Quote:      balance.Quote,
		Cash:       cash,
		Unrealized: unrealized,
		Equity:     equity,
	})
}
