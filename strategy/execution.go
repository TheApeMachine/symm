package strategy

import (
 "time"

 "github.com/krakenfx/api-go/v2/pkg/decimal"
 "github.com/krakenfx/api-go/v2/pkg/spot"
 "github.com/theapemachine/errnie"
 "github.com/theapemachine/symm/broker"
 "github.com/theapemachine/symm/kraken"
)

/* Execution replaces only order submission during learning. Price supplies
book walks and fees; Balance and Regulator retain their normal accounting. */
type Execution struct {
 Price *broker.Price
 Balance *broker.Balance
 At time.Time
 Fill kraken.ExecutionData
}

/* AddOrder fills against the resident book without sending anything to a venue. */
func (execution *Execution) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
 quantity, err := decimal.NewFromString(order.Volume)

 if err != nil {
  return spot.AddOrderResult{}, errnie.Error(err)
 }
 fill := kraken.ExecutionData{ClientOrderID: order.ClOrdId, OrderID: order.ClOrdId, Symbol: order.Pair, Side: order.Type, OrderStatus: "filled", Timestamp: execution.At, CumQty: quantity}

 switch order.Type {
 case "buy":
  cost, err := execution.Price.EntryCost(order.Pair, quantity)

  if err != nil { return spot.AddOrderResult{}, err }
  fill.CumCost, fill.FeeUsdEquiv, fill.AvgPrice = cost.GrossNotional, cost.EntryFee, cost.EntryPrice
 case "sell":
  surface, err := execution.Price.Surface(order.Pair, quantity, execution.At)

  if err != nil { return spot.AddOrderResult{}, err }
  fill.CumCost = surface.Gross
  fill.FeeUsdEquiv = fill.CumCost.Sub(surface.ExecutableValue)
  fill.AvgPrice = surface.ExecutableVWAP
 default:
  return spot.AddOrderResult{}, errnie.Error(errnie.Err(errnie.Validation, "execution: buy or sell required", nil))
 }

 if err := execution.Balance.Settle(fill); err != nil { return spot.AddOrderResult{}, err }
 execution.Fill = fill
 result := spot.AddOrderResult{}
 result.ID = []string{order.ClOrdId}
 return result, nil
}

/* CancelOrder rejects cancellation because simulated orders finish synchronously. */
func (execution *Execution) CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error) {
 return spot.CancelResult{}, errnie.Error(errnie.Err(errnie.Conflict, "execution: no simulated order remains pending", nil))
}
