package main

import (
	"path/filepath"
	"time"

	"github.com/tomas-mraz/gem"
)

const (
	dataDir = "shaders"
	aaa     = "default.frag.spv"
	bbb     = "default.vert.spv"
)

func main() {
	e := gem.New(gem.Config{
		Title:      "GEM Example",
		Width:      800,
		Height:     600,
		RayTracing: true,
	})
	defer e.Destroy()

	err := e.LoadShaders(filepath.Join(dataDir, aaa), filepath.Join(dataDir, bbb))
	if err != nil {
		panic(err)
	}

	s := gem.NewScene[exampleScene](gem.SceneTypeRasterization)
	s.Init(e)

	s.Start(e)
	e.Run(s)
	s.Stop()

	// test výměny scény a vrácení
	time.Sleep(3 * time.Second)

	s.Start(e)
	e.Run(s)
	e.Destroy()
}
