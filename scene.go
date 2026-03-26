package gem

// Scene defines the interface for a game scene.
// Each scene manages its own entities, archetypes, and rendering logic.
// Update returns true when the scene is finished.
type Scene interface {
	Init(e *Engine)
	Update(e *Engine) bool
	Draw(e *Engine)
}
