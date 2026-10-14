package broker

/*
Transport is the execution transport abstraction satisfied by
network.WebsocketClient in live trading and broker.Paper in simulation.
*/
type Transport interface {
	Read(...string) ([]byte, error)
	Write([]byte) error
}
