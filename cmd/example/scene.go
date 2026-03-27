package main

import (
	"math"

	"github.com/tomas-mraz/gem"
	"github.com/tomas-mraz/gem/component"
)

// exampleScene renders 7 colored triangles orbiting in a circle.
type exampleScene struct {
	gem.SceneBasic
	triangles ArchetypeTriangle

	vertexShader   *[]byte
	fragmentShader *[]byte
}

func (s exampleScene) Init(e *gem.Engine) {
	s.SceneBasic.Init(e)

	s.vertexShader = e.GetVertexShader(vertexShader1)
	s.fragmentShader = e.GetFragmentShader(fragmentShader1)

	// init data
	s.triangles = NewArchetypeTriangle()
	s.triangles.Add(component.Position{X: 0, Y: 0}, component.Angle(0), component.Color{ColorR: 1, ColorG: 1, ColorB: 1, Brightness: 1})
	s.triangles.Add(component.Position{X: 1, Y: 0}, component.Angle(0), component.Color{ColorR: 1, ColorG: 1, ColorB: 1, Brightness: 1})
}

func (s exampleScene) Update(e *gem.Engine) bool {
	for i := range s.triangles.Entity {
		a := float32(i) * 2 * math.Pi / 7
		s.triangles.Position[i].X = s.triangles.Position[i].X + 1
		s.triangles.Position[i].Y = s.triangles.Position[i].Y + 1

		s.triangles.Color[i].ColorR = float32(math.Sin(float64(a))*0.5 + 0.5)
		s.triangles.Color[i].ColorG = float32(math.Sin(float64(a+2*math.Pi/3))*0.5 + 0.5)
		s.triangles.Color[i].ColorB = float32(math.Sin(float64(a+4*math.Pi/3))*0.5 + 0.5)
	}
	return true
}

func (s exampleScene) Draw(e *gem.Engine) {
	for i, _ := range s.triangles.Entity {
		e.DrawTriangle(s.triangles.Position[i], s.triangles.Angle[i], s.triangles.Color[i])
	}
}
