package gem

import (
	"fmt"
	"reflect"

	"github.com/tomas-mraz/gem/component"
	vk "github.com/tomas-mraz/vulkan"
	ash "github.com/tomas-mraz/vulkan-ash"
)

// SceneInterface Scene defines the interface for a game scene.
// Each scene manages its own entities, archetypes, and rendering logic.
// Update returns true when the scene is finished.
type Scene interface {
	Init(e *Engine)
	Update(e *Engine) bool
	Draw(e *Engine)
	setType(SceneType)
}

type rasterScene interface {
	rasterState() *rasterState
}

type SceneType uint8

const (
	SceneTypeRasterization SceneType = iota
	SceneTypeRaytracing
)

type SceneBasic struct {
	Camera component.Position
	Type   SceneType
	raster *rasterState
}

func NewScene[T Scene](t SceneType) T {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ == nil {
		panic("gem: scene type cannot be nil")
	}

	var sceneValue reflect.Value
	if typ.Kind() == reflect.Pointer {
		sceneValue = reflect.New(typ.Elem())
	} else {
		sceneValue = reflect.New(typ).Elem()
	}

	if err := initSceneBasic(sceneValue, t); err != nil {
		scene := sceneValue.Interface().(T)
		scene.setType(t)
		return scene
	}

	return sceneValue.Interface().(T)
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
	s.raster = newRasterState()
}

func (s SceneBasic) rasterState() *rasterState {
	return s.raster
}

// SetShaders creates or replaces the raster pipeline owned by this scene.
func (s SceneBasic) SetShaders(e *Engine, vertShaderData, fragShaderData []byte) error {
	rs := s.rasterState()
	if rs == nil {
		return fmt.Errorf("gem: scene raster state is not initialized; create the scene with gem.NewScene or pass a pointer scene embedding SceneBasic")
	}
	return e.setRasterShaders(rs, vertShaderData, fragShaderData)
}

// DestroyRaster releases raster resources owned by this scene.
func (s SceneBasic) DestroyRaster(e *Engine) {
	e.destroyRaster(s.rasterState())
}

func (s SceneBasic) hasGraphicsPipeline() bool {
	rs := s.rasterState()
	return rs != nil && rs.gfx.GetPipeline() != vk.NullPipeline
}

type rasterState struct {
	buffer ash.VulkanBufferInfo
	gfx    ash.VulkanGfxPipelineInfo
}

func newRasterState() *rasterState {
	return &rasterState{}
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
