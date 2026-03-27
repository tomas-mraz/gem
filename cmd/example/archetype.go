package main

import (
	"github.com/tomas-mraz/gem"
	"github.com/tomas-mraz/gem/component"
)

type ArchetypeTriangle struct {
	gem.Archetype
	Angle []component.Angle
	Color []component.Color
}

func NewArchetypeTriangle() ArchetypeTriangle {
	return ArchetypeTriangle{
		Archetype: gem.NewArchetype(),
		Color:     make([]component.Color, 0),
		Angle:     make([]component.Angle, 0),
	}
}

func (t ArchetypeTriangle) Add(position component.Position, angle component.Angle, color component.Color) {
	t.Entity = append(t.Entity, component.NewEntityID())
	t.Position = append(t.Position, position)

	t.Angle = append(t.Angle, angle)
	t.Color = append(t.Color, color)
}
