package utils

import "reflect"

/*
Compact removes all nil values from a slice.
*/
func Compact[T comparable](data []T) []T {
	var result []T

	for _, item := range data {
		if !reflect.ValueOf(item).IsZero() {
			result = append(result, item)
		}
	}

	return result
}
