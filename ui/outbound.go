package ui

func (hub *Hub) publishToFrontend(value any) {
	link := hub.frontend.Load()

	if link == nil {
		return
	}

	_ = link.publish(value)
}
