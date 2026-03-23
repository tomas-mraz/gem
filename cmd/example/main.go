package main

import (
	"math"

	"github.com/tomas-mraz/gem"
)

func main() {
	e := gem.New(gem.Config{
		Title:  "GEM Example",
		Width:  800,
		Height: 600,
	})
	defer e.Destroy()

	e.Run(func(e *gem.Engine) {
		t := float32(e.Elapsed())

		// 7 colored triangles arranged in a circle, slowly orbiting
		for i := 0; i < 7; i++ {
			a := float32(i) * 2 * math.Pi / 7
			radius := float32(0.5)
			x := radius * float32(math.Cos(float64(a+t*0.5)))
			y := radius * float32(math.Sin(float64(a+t*0.5)))

			// Each triangle gets a different hue
			r := float32(math.Sin(float64(a))*0.5 + 0.5)
			g := float32(math.Sin(float64(a+2*math.Pi/3))*0.5 + 0.5)
			b := float32(math.Sin(float64(a+4*math.Pi/3))*0.5 + 0.5)

			e.DrawTriangle(x, y, t*2+a, r, g, b)
		}
	})
}
