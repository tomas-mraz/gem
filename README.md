# gem

Zig port of the Go game engine at <https://github.com/tomas-mraz/gem>. Built on
Vulkan via [vulkan-ash](https://github.com/tomas-mraz/vulkan-ash); window and
input come from [glfw-zig](https://github.com/tomas-mraz/glfw-zig).

The standard game loop is intentionally sequential:
Inputs → Update data → Draw → Present → repeat.

Entity-Component-System data is stored in archetypes (parallel arrays per
component combination). Systems are pure logic operating on those archetypes.

## Layout

```
gem/
├── build.zig
├── build.zig.zon
└── src/
    ├── root.zig             — module surface (re-exports)
    ├── engine.zig           — Engine + Config; fixed-step game loop
    ├── scene.zig            — Scene and Renderer vtables
    ├── scene_manager.zig    — Scene registry, event routing, ash.Session driver
    ├── actions.zig          — ActionSet, ActionMap, digital + axis bindings
    ├── input.zig            — Input state (keys, mouse, scroll)
    ├── glfw_input.zig       — GLFW polling bridge into Input
    ├── component.zig        — Position, Velocity, Color, Angle, EntityID, …
    ├── archetype.zig        — Archetype with parallel component slices
    └── resource_manager.zig — File/JSON resource cache
```

## Build

```bash
export VULKAN_SDK=/usr   # or the path containing share/vulkan/registry/vk.xml
zig build                # libgem.a
zig build test           # run unit tests
```

## API surface (selected)

| Symbol                     | Description                                       |
|----------------------------|---------------------------------------------------|
| `gem.Engine.init(...)`     | Create engine, GLFW host and Vulkan session       |
| `engine.run(scene)`        | Register one scene and run it                     |
| `engine.scene_manager`     | Register multiple scenes + event-bound transitions|
| `engine.deltaTime()`       | Fixed simulation step (seconds)                   |
| `engine.frameTime()`       | Last render-frame duration (seconds)              |
| `engine.interpolationAlpha()` | [0..1] for renderer-side state interpolation   |

## Notes vs. the Go original

- Resource manager is single-threaded (Go used an `RwLock`); the engine never
  touches resources from background goroutines, so the lock added cost without
  buying anything in practice.
- Entity IDs are an atomic-counter `u64` instead of UUIDs.
- Input is polled from GLFW once per fixed update; gem-go used GLFW callbacks,
  but ash.DesktopHost already owns the window's user pointer.
