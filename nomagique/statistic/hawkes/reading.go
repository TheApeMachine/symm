package hawkes

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/bytedance/sonic"
)

func extractReading(payloadPtr capnp.Ptr) (*Reading, error) {
	if !payloadPtr.IsValid() {
		return nil, nil
	}
	data := payloadPtr.Data()
	if len(data) == 0 {
		return nil, nil
	}
	var reading Reading
	if err := sonic.Unmarshal(data, &reading); err != nil {
		return nil, err
	}
	return &reading, nil
}
