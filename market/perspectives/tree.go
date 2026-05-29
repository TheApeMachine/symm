package perspectives

type Tree struct {
	Branch      *map[CategoryType]*Tree
	Observation *Observation
	Action      *Action
}
