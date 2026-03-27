package gem

import (
	"github.com/tomas-mraz/gem/component"
)

type Archetype struct {
	Entity   []component.EntityID // stejný index = stejná entita
	Position []component.Position // hustě zabalené slicy
}

type ArchetypeHolder struct {
	Entity   component.EntityID
	Position component.Position
	Velocity component.Velocity
}

func NewArchetype() Archetype {
	return Archetype{
		Entity:   make([]component.EntityID, 0),
		Position: make([]component.Position, 0),
	}
}
