package main

import (
	"math"

	"github.com/tomas-mraz/gem"
	"github.com/tomas-mraz/gem/component"
)

// exampleScene renders 7 colored triangles orbiting in a circle.
type exampleScene struct{}

func (s *exampleScene) Init(e *gem.Engine)        {}
func (s *exampleScene) Update(e *gem.Engine) bool { return false }

func (s *exampleScene) Draw(e *gem.Engine) {
	t := float32(e.Elapsed())

	for i := 0; i < 7; i++ {
		a := float32(i) * 2 * math.Pi / 7
		radius := float32(0.5)
		x := radius * float32(math.Cos(float64(a+t*0.5)))
		y := radius * float32(math.Sin(float64(a+t*0.5)))

		r := float32(math.Sin(float64(a))*0.5 + 0.5)
		g := float32(math.Sin(float64(a+2*math.Pi/3))*0.5 + 0.5)
		b := float32(math.Sin(float64(a+4*math.Pi/3))*0.5 + 0.5)

		e.DrawTriangle(
			component.Position{X: x, Y: y},
			component.Angle(t*2+a),
			component.Color{ColorR: r, ColorG: g, ColorB: b, Brightness: 1},
		)
	}
}

func main() {
	e := gem.New(gem.Config{
		Title:  "GEM Example",
		Width:  800,
		Height: 600,
	})
	defer e.Destroy()
	if err := e.SetShaders(defaultVertShader, defaultFragShader); err != nil {
		panic(err)
	}

	s := &exampleScene{}
	s.Init(e)
	e.Run(s)
}
