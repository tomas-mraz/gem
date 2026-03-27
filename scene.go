package gem

// Scene defines the interface for a game scene.
// Each scene manages its own entities, archetypes, and rendering logic.
// Update returns true when the scene is finished.
type Scene interface {
	Init(e *Engine)
	Update(e *Engine) bool
	Draw(e *Engine)
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
