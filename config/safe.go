package config

import "github.com/spf13/viper"

type Validatable interface {
	Validate() error
}

func NewSafeConfig[T Validatable](config T) (T, error) {
	if err := config.Validate(); err != nil {
		return config, err
	}

	return config, nil
}

func Get[T any](key string, fallback T) T {
	value := viper.Get(key)

	if value, ok := value.(T); ok {
		return value
	}

	return fallback
}
