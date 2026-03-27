package main

import (
	"github.com/tomas-mraz/gem/component"
)

type ArchetypeTriangle struct {
	EntityID []component.EntityID
	Position []component.Position
	Angle    []component.Angle
	Color    []component.Color
}

func NewArchetypeTriangle() ArchetypeTriangle {
	return ArchetypeTriangle{
		EntityID: make([]component.EntityID, 0),
		Position: make([]component.Position, 0),
		Color:    make([]component.Color, 0),
		Angle:    make([]component.Angle, 0),
	}
}
