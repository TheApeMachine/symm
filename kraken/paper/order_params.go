package paper

import (
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func paramsMap(params any) map[string]any {
	if params == nil {
		return nil
	}

	if frame, ok := params.(map[string]any); ok {
		return frame
	}

	if frame, ok := params.(map[string]interface{}); ok {
		out := make(map[string]any, len(frame))

		for key, value := range frame {
			out[key] = value
		}

		return out
	}

	return nil
}

func orderIDs(params map[string]any) []string {
	return stringList(params["order_id"])
}

func stringList(value any) []string {
	switch ids := value.(type) {
	case string:
		if ids == "" {
			return nil
		}

		return []string{ids}
	case []string:
		return ids
	case []any:
		out := make([]string, 0, len(ids))

		for _, id := range ids {
			if text, ok := id.(string); ok && text != "" {
				out = append(out, text)
			}
		}

		return out
	}

	return nil
}

func batchOrders(params any) (string, []trading.AddParams, bool) {
	frame := paramsMap(params)

	if frame == nil {
		return "", nil, false
	}

	symbol, _ := frame["symbol"].(string)

	if symbol == "" {
		return "", nil, false
	}

	if items, ok := frame["orders"].([]trading.AddParams); ok && len(items) >= 2 {
		return symbol, items, true
	}

	return "", nil, false
}

func (orders *Orders) publishMessages(messages []public.SocketMessage) public.SocketMessage {
	if len(messages) == 0 {
		return public.SocketMessage{}
	}

	for index := 1; index < len(messages); index++ {
		if messages[index].Channel == "" {
			continue
		}

		orders.socket.broadcasts["kraken:private"].Send(&qpool.QValue[any]{
			Type:  public.OrdersChannel,
			Value: messages[index],
		})
	}

	return messages[0]
}
