package tests

import "github.com/spf13/viper"

/*
EnsureActorBuffer sets system.actor.buffer to the production default when unset.
*/
func EnsureActorBuffer() {
	if viper.GetInt("system.actor.buffer") < 1 {
		viper.Set("system.actor.buffer", 64)
	}
}
