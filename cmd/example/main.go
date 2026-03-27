package main

import (
	"path/filepath"

	"github.com/tomas-mraz/gem"
)

const (
	dataDir = "shaders"
	aaa     = "default.frag.spv"
	bbb     = "default.vert.spv"
)

func main() {
	e := gem.New(gem.Config{
		Title:  "GEM Example",
		Width:  800,
		Height: 600,
	})
	defer e.Destroy()

	err := e.LoadShaders(filepath.Join(dataDir, aaa), filepath.Join(dataDir, bbb))
	if err != nil {
		panic(err)
	}

	s := gem.NewScene[exampleScene](gem.SceneTypeRasterization)
	s.Init(e)
	e.Run(s)
}
