package main

import (
	"path/filepath"
	"time"

	"github.com/tomas-mraz/gem"
)

const (
	dataDir         = "shaders"
	fragmentShader1 = "default.frag.spv"
	vertexShader1   = "default.vert.spv"
)

func main() {
	engine := gem.New(gem.Config{
		Title:      "GEM Example",
		Width:      800,
		Height:     600,
		RayTracing: true,
	})
	defer engine.Destroy()

	err := engine.LoadShaders(filepath.Join(dataDir, fragmentShader1), filepath.Join(dataDir, vertexShader1))
	if err != nil {
		panic(err)
	}

	scene := gem.NewScene[exampleScene](gem.SceneTypeRasterization)

	// initialization scene data, Stop keep data (to go back to scene later), Destroy delete data
	scene.Init(engine)
	// after init all scenes delete engine's pointers to shaders (enable to GC it from memory when scenes do not need it)
	// for example, if only scene1 needs some shader and scene1 is destroyed, then GC can free data because there is no reference on it
	engine.CleanShadersBank()

	scene.Start(engine)
	engine.Run(scene)
	scene.Stop()

	// test výměny scény a vrácení
	time.Sleep(3 * time.Second)

	scene.Start(engine)
	engine.Run(scene)
	engine.Destroy()
}
