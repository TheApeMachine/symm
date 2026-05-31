package replay

import (
	"context"
)

/*
StreamRows replays channel frames from Hub and emits decoded rows on out.
*/
func StreamRows[T any](
	ctx context.Context, hub *Hub, channel string,
) <-chan *T {
	inbound := hub.SubscribeWS(channel)
	outbound := make(chan *T, 256)

	go func() {
		defer close(outbound)

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-inbound:
				if !ok {
					return
				}

				rows, _, err := DecodeWSRows[T](payload)

				if err != nil {
					continue
				}

				for index := range rows {
					row := rows[index]

					select {
					case <-ctx.Done():
						return
					case outbound <- &row:
					}
				}
			}
		}
	}()

	return outbound
}

/*
StreamSnapshot replays object-shaped channel frames from Hub.
*/
func StreamSnapshot[T any](
	ctx context.Context, hub *Hub, channel string,
) <-chan *T {
	inbound := hub.SubscribeWS(channel)
	outbound := make(chan *T, 8)

	go func() {
		defer close(outbound)

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-inbound:
				if !ok {
					return
				}

				row, err := DecodeWSSnapshot[T](payload)

				if err != nil || row == nil {
					continue
				}

				select {
				case <-ctx.Done():
					return
				case outbound <- row:
				}
			}
		}
	}()

	return outbound
}
