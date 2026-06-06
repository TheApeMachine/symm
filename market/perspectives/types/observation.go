package types

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

type ObservationType uint8

const (
	ObservationNone ObservationType = iota
	ObservationHasStarted
	ObservationHasContinued
	ObservationHasEnded
	ObservationHasDoneBefore
	ObservationHolding
	ObservationNotHolding
)

var observationNames = map[ObservationType]string{
	ObservationNone:          "none",
	ObservationHasStarted:    "has_started",
	ObservationHasContinued:  "has_continued",
	ObservationHasEnded:      "has_ended",
	ObservationHasDoneBefore: "has_done_before",
	ObservationHolding:       "holding",
	ObservationNotHolding:    "not_holding",
}

func (observation ObservationType) String() string {
	name, ok := observationNames[observation]

	if ok {
		return name
	}

	return fmt.Sprintf("observation_%d", observation)
}

func (observation ObservationType) MarshalYAML() (any, error) {
	return MarshalEnum(observation, observationNames)
}

func (observation ObservationType) MarshalJSON() ([]byte, error) {
	return MarshalEnumJSON(observation, observationNames)
}

func (observation *ObservationType) UnmarshalYAML(value *yaml.Node) error {
	return UnmarshalEnum(value, observation, observationNames)
}

func (observation *ObservationType) UnmarshalJSON(data []byte) error {
	return UnmarshalEnumJSON(data, observation, observationNames)
}
