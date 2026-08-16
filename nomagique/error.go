package nomagique

import "errors"

func primitiveError(message string) error {
	return errors.New("nomagique: " + message)
}
