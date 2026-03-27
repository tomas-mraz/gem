# gem

Go game engine built on Vulkan via [vulkan-ash](https://github.com/tomas-mraz/vulkan-ash).

The standard game loop is intentionally sequential: Inputs → Update data → Draw → Present → repeat

Engine using ECP (Entity-Component-System) for storing data about objects.
Using interfaces to access objects with a defined pattern of components.
Archetypes are static on the application side.

Tři části
- Entity — Identification like index into an array.
- Component — pure data without business logic. For example, Position{X, Y}, Velocity{DX, DY}, Sprite{TextureID}, Health{HP}.
- System — pure business logic without data. Operate on entities which a particular set of component (accessing by interface).
For example, `MovementSystem` process all entities with component `Position` and `Velocity`.
  
Another types of data architecture are: Interface-driven, Scene Graph

# Architecture

## Data Organization

Archetypy — jak to dělají moderní ECS (arche, Bevy, Unity DOTS)

// Archetyp = skupina entit se stejnou sadou komponent
type Archetype struct {                                                                                                                   
positions  []Position      // hustě zabalené slicy                                                                                    
velocities []Velocity      // stejný index = stejná entita                                                                            
healths    []Health                                                                                                                   
entities   []EntityID                                                                                                                 
}

Příklad:

Archetype A (Position + Velocity + Sprite):                                                                                               
positions:  [p0, p1, p2, p3, ...]    ← souvislý blok paměti
velocities: [v0, v1, v2, v3, ...]                                                                                                       
entities:   [42, 44, 71, 99, ...]

Archetype B (Position + Health):
positions:  [p0, p1, ...]                                                                                                               
healths:    [h0, h1, ...]                                                                                                               
entities:   [43, 50, ...]

Dotaz "má entita komponentu?" se řeší na úrovni archetypu:

func (w *World) Query(required uint64) []*Archetype {                                                                                     
var result []*Archetype
for _, arch := range w.archetypes {
if arch.mask & required == required {                                                                                             
result = append(result, arch)
}                                                                                                                                 
}
return result
}

// MovementSystem
for _, arch := range world.Query(CPosition | CVelocity) {
// ŽÁDNÉ if/ok kontroly — všechny entity v archetypu
// mají zaručeně obě komponenty
for i := range arch.entities {
arch.positions[i].X += arch.velocities[i].DX
arch.positions[i].Y += arch.velocities[i].DY
}
}

Proč je to rychlé: vnitřní smyčka iteruje přes souvislé slicy bez jakéhokoliv větvení. CPU prefetcher si s tím poradí, cache line se plně
využije. Žádné map lookupy, žádné if !ok.

Cena: když entitě přidáš/odebereš komponentu, musí se přesunout do jiného archetypu (kopie dat). To je drahé, ale děje se zřídka.




Object types:
- scene [rasterization / raytracing]
- node [2D tvar, 3D tvar, model, světlo, aktivita, ...]
- instance of node
- group containing nodes
- event
- activity
- input

Hierarchy:
- scene
  - binding inputs to events
  - node
    - instance

### Scene

It is top-level object který určuje způsob vykreslovací pipeline [rasterization / raytracing].
Scéna obsahuje nastavení kamery.

### Node

Základní společné atributy nezávisle na typu. Má atributy podle svého typu.

Common attributes (primary attributes):
- 3D position
- type (it determines secondary attributes)
- secondary attributes
- `event` listener
  - registered `activity` ("roztočit, zežloutnout a točit se", "zastavit se a zezelenat", "zastavit se a zčervenat)
- `event` generator (not implemented yet)

#### Type triangle

Node's Secondary attributes:
- length of sides (A,B,C)
- angle
- color

### Instance

Instance je "potomek" node a sdílí s ním `type`, `registered actions`
Má vlastní pozici.

Sdílí typ

### Group

has event listener and distribute event to all nodes in it  
position shift is applied to all nodes in it


#### Activity

Attributes:
- name
- type
- function
- event listener

Types:
- node activity (function is evaluated every loop)
- loop activity (function is evaluated every loop)
- background activity (function run in a separate goroutine)
  - started by starting a scene
  - started by event

#### Event

Attributes:
- name
- from node UUID
- to node [type, UUID]

Představuje událost / akci



### Example #1:
 - scene (rasterization)
   - activity (type: background activity (started by scene))
     - funkce
       - pošle event `ramp-up` na `trojuhelnik1`
       - zavolá funkci `check1()`
       - když se vrátí true ( pošle event `ramp-down-green` na `trojuhelnik1` & pošle event `ramp-up` na `trojuhelnik2` )
       - když se vrátí false ( pošle event `ramp-down-red` na `trojuhelnik1` & zastav vykonávání )
       - zavolá funkci `check2()`
       - a tak dále
   - activity (type: loop activity)
     - funkce která se vyhodnotí každý loop
       - posune 7 instancí trojúhelníků (má na ně odkaz a změní jim souřadnice)
   - node (type: triangle)
     - registered `loop activity` for `ramp-up`
     - registered `loop activity` for `ramp-down-green`
     - registered `loop activity` for `ramp-down-red`
     - 7 instances

## Structure

```
gem/
├── engine.go              # Engine core — window, Vulkan init, game loop, rendering
└── cmd/
    └── example/
        ├── main.go        # Example — 7 colored triangles orbiting in a circle
        └── shaders/
            ├── default.vert       # Vertex shader (push constants: position, rotation, aspect)
            ├── default.frag       # Fragment shader (push constants: color, brightness)
            ├── default.vert.spv   # Compiled SPIR-V (embedded via go:embed)
            └── default.frag.spv   # Compiled SPIR-V (embedded via go:embed)
```


## API

| Method                                  | Description                                           |
|-----------------------------------------|-------------------------------------------------------|
| `gem.New(cfg)`                          | Create engine, open window, initialize Vulkan         |
| `e.Run(func)`                           | Start game loop, calls provided function every frame  |
| `e.DrawTriangle(x, y, angle, r, g, b)`  | Draw a colored triangle at position with rotation     |
| `e.Elapsed()`                           | Total time since start (seconds)                      |
| `e.DeltaTime()`                         | Last frame duration (seconds)                         |
| `e.Destroy()`                           | Release all resources                                 |

## Build

```bash
go build -tags wayland ./cmd/example/
```

## Dependencies

- [vulkan-ash](https://github.com/tomas-mraz/vulkan-ash) — Vulkan abstraction layer
- [vulkan](https://github.com/tomas-mraz/vulkan) — Go Vulkan bindings
- [glfw](https://github.com/go-gl/glfw) — Window management


# other projects

https://github.com/mlange-42/ark
https://github.com/mlange-42/ark-tools
https://github.com/gopxl/pixel
