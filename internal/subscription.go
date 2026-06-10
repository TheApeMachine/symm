package internal

/*
Subscription binds one consumer to a broadcast channel under a unique name.
*/
type Subscription struct {
	Channel string
	Name    string
}

func Subscribe(channel string, name string) Subscription {
	return Subscription{
		Channel: channel,
		Name:    name,
	}
}
