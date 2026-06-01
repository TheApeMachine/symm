package private

import "strings"

type EndpointType string

const (
	BaseURL EndpointType = "https://api.kraken.com/0/private"


)

func (endpoint EndpointType) signPath() string {
	return strings.TrimPrefix(string(endpoint), "https://api.kraken.com")
}
