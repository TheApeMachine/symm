package broker

import "fmt"

/*
Router publishes live exchange requests.
*/
type Router struct {
	publish func(any)
}

/*
NewRouter wires one order publisher.
*/
func NewRouter(publish func(any)) *Router {
	return &Router{publish: publish}
}

/*
Publish sends one live order frame.
*/
func (router *Router) Publish(value any) error {
	if router == nil || router.publish == nil {
		return fmt.Errorf("order router is required")
	}

	router.publish(value)

	return nil
}
