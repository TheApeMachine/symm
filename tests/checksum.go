package tests

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
)

const bookChecksumDepth = 10

/*
touch returns the best reconstructed price and its aggregate quantity.
*/
func (validator *Validator) touch(
	symbol string,
	side string,
	highest bool,
) (float64, float64) {
	price := 0.0

	for candidate := range validator.books[symbol][side] {
		if price == 0 || highest && candidate > price || !highest && candidate < price {
			price = candidate
		}
	}

	return price, validator.books[symbol][side][price]
}

/*
level3Checksum derives Kraken's CRC from exact resting-order decimals.
*/
func (validator *Validator) level3Checksum(symbol string) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		orders := make([]orderState, 0, len(validator.orders[symbol][side]))

		for _, order := range validator.orders[symbol][side] {
			orders = append(orders, order)
		}

		sort.Slice(orders, func(left, right int) bool {
			if orders[left].priceValue != orders[right].priceValue {
				if side == "bids" {
					return orders[left].priceValue > orders[right].priceValue
				}

				return orders[left].priceValue < orders[right].priceValue
			}

			if orders[left].priority != orders[right].priority {
				return orders[left].priority < orders[right].priority
			}

			return orders[left].id < orders[right].id
		})

		levels := 0
		lastPrice := ""

		for _, order := range orders {
			if order.price != lastPrice {
				levels++
				lastPrice = order.price
			}

			if levels > 10 {
				break
			}

			for _, value := range []string{order.price, order.qty} {
				normalized := strings.TrimLeft(strings.ReplaceAll(value, ".", ""), "0")
				checksum = crc32.Update(checksum, crc32.IEEETable, []byte(normalized))
			}
		}
	}

	return checksum
}

/*
bookChecksum derives Kraken's CRC from the reconstructed L2 state.
*/
func (validator *Validator) bookChecksum(symbol string) uint32 {
	checksum := uint32(0)

	for _, side := range []string{"asks", "bids"} {
		prices := make([]float64, 0, len(validator.books[symbol][side]))

		for price := range validator.books[symbol][side] {
			prices = append(prices, price)
		}

		sort.Float64s(prices)

		if side == "bids" {
			sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
		}

		for index, price := range prices {
			if index == bookChecksumDepth {
				break
			}

			for _, value := range []float64{price, validator.books[symbol][side][price]} {
				text := strconv.FormatFloat(value, 'f', -1, 64)
				normalized := strings.TrimLeft(strings.ReplaceAll(text, ".", ""), "0")
				checksum = crc32.Update(checksum, crc32.IEEETable, []byte(normalized))
			}
		}
	}

	return checksum
}
