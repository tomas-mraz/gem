# gem

Go game engine built on Vulkan via [vulkan-ash](https://github.com/tomas-mraz/vulkan-ash).

## Architecture

Typy objektů:
- scene
- nodes
- pods
- events

Precess game loop
- process "action" on each "node"
- process inputs 
- process events

Engine umožňuje vytvořit objekt "scene" který určuje způsob vykreslovací pipeline [rasterization / raytracing].
Pod objekt scene jde přiřadit "objects": 3D primitiva (krychle, koule, jehlan, ...), model, světlo, aktivitu ...

Příklad:
 - scene (rasterization)
   - primitive (triangle)
     - attributes: position, dimensions, color
     - aktivita (function describing behavior in the time of the game loop)
     - events 


## Structure

```
gem/
├── engine.go              # Engine core — window, Vulkan init, game loop, rendering
├── shaders/
│   ├── default.vert       # Vertex shader (push constants: position, rotation, aspect)
│   ├── default.frag       # Fragment shader (push constants: color, brightness)
│   ├── default.vert.spv   # Compiled SPIR-V (embedded via go:embed)
│   └── default.frag.spv   # Compiled SPIR-V (embedded via go:embed)
└── cmd/
    └── example/
        └── main.go        # Example — 7 colored triangles orbiting in a circle
```

## Usage

```go
package main

import "github.com/tomas-mraz/gem"

func main() {
    e := gem.New(gem.Config{
        Title:  "My Game",
        Width:  800,
        Height: 600,
    })
    defer e.Destroy()

    e.Run(func(e *gem.Engine) {
        t := float32(e.Elapsed())
        e.DrawTriangle(0, 0, t, 0, 1, 0) // spinning green triangle
    })
}
```

## API

| Method | Description |
|---|---|
| `gem.New(cfg)` | Create engine, open window, initialize Vulkan |
| `e.Run(func)` | Start game loop, calls provided function every frame |
| `e.DrawTriangle(x, y, angle, r, g, b)` | Draw a colored triangle at position with rotation |
| `e.Elapsed()` | Total time since start (seconds) |
| `e.DeltaTime()` | Last frame duration (seconds) |
| `e.Destroy()` | Release all resources |

## Build

```bash
go build -tags wayland ./cmd/example/
```

## Dependencies

- [vulkan-ash](https://github.com/tomas-mraz/vulkan-ash) — Vulkan abstraction layer
- [vulkan](https://github.com/tomas-mraz/vulkan) — Go Vulkan bindings
- [glfw](https://github.com/go-gl/glfw) — Window management
