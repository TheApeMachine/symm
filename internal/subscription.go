package internal

/*
Subscription binds one consumer to a broadcast channel under a unique name.
*/
type Subscription struct {
	Channel Channel
	Name    string
}

func Subscribe(channel Channel, name string) Subscription {
	return Subscription{
		Channel: channel,
		Name:    name,
	}
}
