package main

import (
	"github.com/tomas-mraz/gem"
)

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

	s := gem.NewScene[exampleScene](gem.SceneTypeRasterization)
	s.Init(e)
	e.Run(s)
}
