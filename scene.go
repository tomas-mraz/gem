package gem

import "github.com/tomas-mraz/gem/component"

// SceneInterface Scene defines the interface for a game scene.
// Each scene manages its own entities, archetypes, and rendering logic.
// Update returns true when the scene is finished.
type Scene interface {
	Init(e *Engine)
	Update(e *Engine) bool
	Draw(e *Engine)
	setType(SceneType)
}

type SceneType uint8

const (
	SceneTypeRasterization SceneType = iota
	SceneTypeRaytracing
)

type SceneBasic struct {
	Camera component.Position
	Type   SceneType
}

func NewScene[T Scene](t SceneType) T {
	var a T
	a.setType(t)
	return a
}

func (s SceneBasic) Init(e *Engine) {
	s.Camera = component.Position{X: 0, Y: 0, Z: 0}
}

func (s SceneBasic) Update(e *Engine) bool {
	return false
}

func (s SceneBasic) Draw(e *Engine) {
	//TODO implement me
}

func (s SceneBasic) setType(sceneType SceneType) {
	s.Type = sceneType
}

// CustomDrawer is implemented by scenes that handle their own frame rendering,
// bypassing the default graphics pipeline and render pass.
type CustomDrawer interface {
	DrawCustom(e *Engine) bool
}

// System defines the interface for an ECS system.
// Systems operate on slices of component data each frame.
type System interface {
	Update(e *Engine)
}
