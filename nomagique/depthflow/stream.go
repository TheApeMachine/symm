package depthflow

import (
	"errors"
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
BookFlow measures displayed-depth mutation across book levels, tracking
additions, removals, turnover, net displayed flow, and resolution gaps.
*/
type BookFlow struct {
	err          error
	books        broker.BookSource
	prevBids     map[float64]float64
	prevAsks     map[float64]float64
	prevNotional float64
	prevTime     time.Time
}

func NewBookFlow(books broker.BookSource) core.Primitive {
	return &BookFlow{
		books:    books,
		prevBids: make(map[float64]float64),
		prevAsks: make(map[float64]float64),
	}
}

func (op *BookFlow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if measurement == nil || measurement.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			var obsBid, obsAsk float64
			var addedBid, removedBid float64
			var addedAsk, removedAsk float64
			var touchBidNotional, touchAskNotional float64

			if op.books != nil {
				op.books.Book(measurement.Label, func(b *book.Book) {
					currBids := make(map[float64]float64)
					currAsks := make(map[float64]float64)

					cursor := b.BestBid()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := kraken.Float64(cursor.Price)
						qty := kraken.Float64(cursor.Quantity)
						levelNotional := price * qty
						obsBid += levelNotional

						if count == 0 {
							touchBidNotional = levelNotional
						}

						currBids[price] = qty

						prevQty := op.prevBids[price]
						if qty > prevQty {
							addedBid += price * (qty - prevQty)
						}

						if qty < prevQty {
							removedBid += price * (prevQty - qty)
						}

						cursor = cursor.Lower
					}

					for price, prevQty := range op.prevBids {
						if _, exists := currBids[price]; !exists {
							removedBid += price * prevQty
						}
					}

					cursor = b.BestAsk()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := kraken.Float64(cursor.Price)
						qty := kraken.Float64(cursor.Quantity)
						levelNotional := price * qty
						obsAsk += levelNotional

						if count == 0 {
							touchAskNotional = levelNotional
						}

						currAsks[price] = qty

						prevQty := op.prevAsks[price]
						if qty > prevQty {
							addedAsk += price * (qty - prevQty)
						}

						if qty < prevQty {
							removedAsk += price * (prevQty - qty)
						}

						cursor = cursor.Higher
					}

					for price, prevQty := range op.prevAsks {
						if _, exists := currAsks[price]; !exists {
							removedAsk += price * prevQty
						}
					}

					op.prevBids = currBids
					op.prevAsks = currAsks
				})
			}

			if obsBid > 0 || obsAsk > 0 {
				if !op.prevTime.IsZero() && !op.prevTime.After(measurement.At) {
					measurement.From = op.prevTime
				}

				totalNotional := obsBid + obsAsk
				notionalScale := math.Max(totalNotional, 1.0)

				measurement.SetMetric("book_notional:bid", data.NewMetric[float64](
					"book_notional:bid",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(obsBid))
				measurement.SetMetric("book_notional:ask", data.NewMetric[float64](
					"book_notional:ask",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(obsAsk))
				measurement.SetMetric("book_notional", data.NewMetric[float64](
					"book_notional",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(totalNotional))
				measurement.SetMetric("observed_notional:bid", data.NewMetric[float64](
					"observed_notional:bid",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(obsBid))
				measurement.SetMetric("observed_notional:ask", data.NewMetric[float64](
					"observed_notional:ask",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(obsAsk))
				measurement.SetMetric("observed_notional", data.NewMetric[float64](
					"observed_notional",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					notionalScale,
				).Write(totalNotional))

				measurement.SetMetric("added_notional:bid", data.NewMetric[float64](
					"added_notional:bid",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(addedBid, 1.0),
				).Write(addedBid))
				measurement.SetMetric("removed_notional:bid", data.NewMetric[float64](
					"removed_notional:bid",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(removedBid, 1.0),
				).Write(removedBid))
				measurement.SetMetric("net_displayed_flow:bid", data.NewMetric[float64](
					"net_displayed_flow:bid",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(math.Abs(addedBid-removedBid), 1.0),
				).Write(addedBid-removedBid))
				measurement.SetMetric("added_notional:ask", data.NewMetric[float64](
					"added_notional:ask",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(addedAsk, 1.0),
				).Write(addedAsk))
				measurement.SetMetric("removed_notional:ask", data.NewMetric[float64](
					"removed_notional:ask",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(removedAsk, 1.0),
				).Write(removedAsk))
				measurement.SetMetric("net_displayed_flow:ask", data.NewMetric[float64](
					"net_displayed_flow:ask",
					data.UnitNotional,
					data.TimescaleInstantaneous,
					0,
					math.Max(math.Abs(addedAsk-removedAsk), 1.0),
				).Write(addedAsk-removedAsk))

				bookImb := 0.0
				if totalNotional > 0 {
					bookImb = (obsBid - obsAsk) / totalNotional
					measurement.SetMetric("book_imbalance", data.NewMetric[float64](
						"book_imbalance",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(bookImb))
					measurement.SetMetric("observed_notional_imbalance", data.NewMetric[float64](
						"observed_notional_imbalance",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(bookImb))
				}

				if touchBidNotional > 0 || touchAskNotional > 0 {
					touchImb := (touchBidNotional - touchAskNotional) / (touchBidNotional + touchAskNotional)
					measurement.SetMetric("touch_imbalance", data.NewMetric[float64](
						"touch_imbalance",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(touchImb))

					gap := touchImb - bookImb
					measurement.SetMetric("imbalance_resolution_gap", data.NewMetric[float64](
						"imbalance_resolution_gap",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(gap))
					measurement.SetMetric("imbalance_resolution_distance", data.NewMetric[float64](
						"imbalance_resolution_distance",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(math.Abs(gap)))
				}

				netFlowBid := addedBid - removedBid
				netFlowAsk := addedAsk - removedAsk
				grossFlow := math.Abs(netFlowBid) + math.Abs(netFlowAsk)
				if grossFlow > 0 {
					measurement.SetMetric("flow_activity_imbalance", data.NewMetric[float64](
						"flow_activity_imbalance",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write((netFlowBid-netFlowAsk)/grossFlow))
				}

				if !op.prevTime.IsZero() {
					elapsedSeconds := measurement.At.Sub(op.prevTime).Seconds()

					if elapsedSeconds > 0 {
						measurement.SetMetric("added_notional_rate:bid", data.NewMetric[float64](
							"added_notional_rate:bid",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(addedBid/elapsedSeconds, 1.0),
						).Write(addedBid/elapsedSeconds))
						measurement.SetMetric("added_notional_rate:ask", data.NewMetric[float64](
							"added_notional_rate:ask",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(addedAsk/elapsedSeconds, 1.0),
						).Write(addedAsk/elapsedSeconds))
						measurement.SetMetric("removed_notional_rate:bid", data.NewMetric[float64](
							"removed_notional_rate:bid",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(removedBid/elapsedSeconds, 1.0),
						).Write(removedBid/elapsedSeconds))
						measurement.SetMetric("removed_notional_rate:ask", data.NewMetric[float64](
							"removed_notional_rate:ask",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(removedAsk/elapsedSeconds, 1.0),
						).Write(removedAsk/elapsedSeconds))
						measurement.SetMetric("net_displayed_flow_rate:bid", data.NewMetric[float64](
							"net_displayed_flow_rate:bid",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(math.Abs(netFlowBid/elapsedSeconds), 1.0),
						).Write(netFlowBid/elapsedSeconds))
						measurement.SetMetric("net_displayed_flow_rate:ask", data.NewMetric[float64](
							"net_displayed_flow_rate:ask",
							data.UnitNotionalRate,
							data.TimescalePerSecond,
							0.0,
							math.Max(math.Abs(netFlowAsk/elapsedSeconds), 1.0),
						).Write(netFlowAsk/elapsedSeconds))

						referenceNotional := (op.prevNotional + totalNotional) / 2.0
						if referenceNotional > 0 {
							turnover := (addedBid + removedBid + addedAsk + removedAsk) / (referenceNotional * elapsedSeconds)
							measurement.SetMetric("book_turnover_rate", data.NewMetric[float64](
								"book_turnover_rate",
								data.UnitRate,
								data.TimescalePerSecond,
								0.0,
								math.Max(turnover, 1.0),
							).Write(turnover))

							netBookChange := (totalNotional - op.prevNotional) / (referenceNotional * elapsedSeconds)
							measurement.SetMetric("net_book_change_rate", data.NewMetric[float64](
								"net_book_change_rate",
								data.UnitRate,
								data.TimescalePerSecond,
								0.0,
								math.Max(math.Abs(netBookChange), 1.0),
							).Write(netBookChange))

							signedFlowRate := (netFlowBid - netFlowAsk) / (referenceNotional * elapsedSeconds)
							measurement.SetMetric("signed_net_displayed_flow_rate", data.NewMetric[float64](
								"signed_net_displayed_flow_rate",
								data.UnitRate,
								data.TimescalePerSecond,
								0.0,
								math.Max(math.Abs(signedFlowRate), 1.0),
							).Write(signedFlowRate))
						}
					}
				}

				op.prevNotional = totalNotional
				op.prevTime = measurement.At
			}

			measurement.EnsureMetadata()

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *BookFlow) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
