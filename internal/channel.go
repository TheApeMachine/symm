package internal

/*
Channel names the broadcast groups on the shared qpool bus.
Use these constants so misspelled routes fail at compile time.
*/
type Channel string

const (
	ChannelRaw           Channel = "raw"
	ChannelUI            Channel = "ui"
	ChannelMeasurements  Channel = "measurements"
	ChannelKrakenPublic  Channel = "kraken:public"
	ChannelKrakenPrivate Channel = "kraken:private"
	ChannelKrakenFutures Channel = "kraken:futures"
	ChannelLevel3        Channel = "level3"
	ChannelAudit         Channel = "audit"
)

func (channel Channel) String() string {
	return string(channel)
}
