package config

import "reflect"

/*
CloneTunables returns a deep copy so hill-climb mutations do not alias cfg fields.
*/
func CloneTunables(source Tunables) Tunables {
	clone := Tunables{}
	sourceValue := reflect.ValueOf(source)
	cloneValue := reflect.ValueOf(&clone).Elem()

	for index := 0; index < sourceValue.NumField(); index++ {
		field := sourceValue.Field(index)

		if field.Kind() != reflect.Pointer || field.IsNil() {
			continue
		}

		copyValue := reflect.New(field.Type().Elem())
		copyValue.Elem().Set(field.Elem())
		cloneValue.Field(index).Set(copyValue)
	}

	return clone
}
