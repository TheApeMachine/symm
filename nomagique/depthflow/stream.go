package depthflow

import (
	"errors"
	"iter"
	"math"
	"strconv"
	"time"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
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
	prevBids     map[string]float64
	prevAsks     map[string]float64
	prevNotional float64
	prevTime     time.Time
}

func NewBookFlow(books broker.BookSource) core.Primitive {
	return &BookFlow{
		books:    books,
		prevBids: make(map[string]float64),
		prevAsks: make(map[string]float64),
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
					currBids := make(map[string]float64)
					currAsks := make(map[string]float64)

					cursor := b.BestBid()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := cursor.Price.Float64()
						qty := cursor.Quantity.Float64()
						levelNotional := price * qty
						obsBid += levelNotional

						if count == 0 {
							touchBidNotional = levelNotional
						}

						priceStr := cursor.Price.String()
						currBids[priceStr] = qty

						prevQty := op.prevBids[priceStr]
						if qty > prevQty {
							addedBid += price * (qty - prevQty)
						}

						if qty < prevQty {
							removedBid += price * (prevQty - qty)
						}

						cursor = cursor.Lower
					}

					for priceStr, prevQty := range op.prevBids {
						if _, exists := currBids[priceStr]; !exists {
							price, _ := strconv.ParseFloat(priceStr, 64)
							removedBid += price * prevQty
						}
					}

					cursor = b.BestAsk()
					for count := 0; count < 100 && cursor != nil; count++ {
						price := cursor.Price.Float64()
						qty := cursor.Quantity.Float64()
						levelNotional := price * qty
						obsAsk += levelNotional

						if count == 0 {
							touchAskNotional = levelNotional
						}

						priceStr := cursor.Price.String()
						currAsks[priceStr] = qty

						prevQty := op.prevAsks[priceStr]
						if qty > prevQty {
							addedAsk += price * (qty - prevQty)
						}

						if qty < prevQty {
							removedAsk += price * (prevQty - qty)
						}

						cursor = cursor.Higher
					}

					for priceStr, prevQty := range op.prevAsks {
						if _, exists := currAsks[priceStr]; !exists {
							price, _ := strconv.ParseFloat(priceStr, 64)
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

				measurement.WriteMetric("book_notional:bid", obsBid)
				measurement.WriteMetric("book_notional:ask", obsAsk)
				measurement.WriteMetric("book_notional", totalNotional)
				measurement.WriteMetric("observed_notional:bid", obsBid)
				measurement.WriteMetric("observed_notional:ask", obsAsk)
				measurement.WriteMetric("observed_notional", totalNotional)

				measurement.WriteMetric("added_notional:bid", addedBid)
				measurement.WriteMetric("removed_notional:bid", removedBid)
				measurement.WriteMetric("net_displayed_flow:bid", addedBid-removedBid)
				measurement.WriteMetric("added_notional:ask", addedAsk)
				measurement.WriteMetric("removed_notional:ask", removedAsk)
				measurement.WriteMetric("net_displayed_flow:ask", addedAsk-removedAsk)

				bookImb := 0.0
				if totalNotional > 0 {
					bookImb = (obsBid - obsAsk) / totalNotional
					measurement.WriteNormalized("book_imbalance", bookImb)
					measurement.WriteNormalized("observed_notional_imbalance", bookImb)
				}

				if touchBidNotional > 0 || touchAskNotional > 0 {
					touchImb := (touchBidNotional - touchAskNotional) / (touchBidNotional + touchAskNotional)
					measurement.WriteNormalized("touch_imbalance", touchImb)

					gap := touchImb - bookImb
					measurement.WriteMetric("imbalance_resolution_gap", gap)
					measurement.WriteMetric("imbalance_resolution_distance", math.Abs(gap))
				}

				netFlowBid := addedBid - removedBid
				netFlowAsk := addedAsk - removedAsk
				grossFlow := math.Abs(netFlowBid) + math.Abs(netFlowAsk)
				if grossFlow > 0 {
					measurement.WriteNormalized("flow_activity_imbalance", (netFlowBid-netFlowAsk)/grossFlow)
				}

				if !op.prevTime.IsZero() {
					elapsedSeconds := measurement.At.Sub(op.prevTime).Seconds()

					if elapsedSeconds > 0 {
						measurement.WriteMetric("added_notional_rate:bid", addedBid/elapsedSeconds)
						measurement.WriteMetric("added_notional_rate:ask", addedAsk/elapsedSeconds)
						measurement.WriteMetric("removed_notional_rate:bid", removedBid/elapsedSeconds)
						measurement.WriteMetric("removed_notional_rate:ask", removedAsk/elapsedSeconds)
						measurement.WriteMetric("net_displayed_flow_rate:bid", netFlowBid/elapsedSeconds)
						measurement.WriteMetric("net_displayed_flow_rate:ask", netFlowAsk/elapsedSeconds)

						referenceNotional := (op.prevNotional + totalNotional) / 2.0
						if referenceNotional > 0 {
							turnover := (addedBid + removedBid + addedAsk + removedAsk) / (referenceNotional * elapsedSeconds)
							measurement.WriteMetric("book_turnover_rate", turnover)

							netBookChange := (totalNotional - op.prevNotional) / (referenceNotional * elapsedSeconds)
							measurement.WriteMetric("net_book_change_rate", netBookChange)

							signedFlowRate := (netFlowBid - netFlowAsk) / (referenceNotional * elapsedSeconds)
							measurement.WriteMetric("signed_net_displayed_flow_rate", signedFlowRate)
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
