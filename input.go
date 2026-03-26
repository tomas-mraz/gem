package gem

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/tomas-mraz/input"
)

func hookInput(w *glfw.Window, in *input.Input) {
	w.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
		k := mapKey(key)
		switch action {
		case glfw.Press:
			in.KeyDown(k)
		case glfw.Release:
			in.KeyUp(k)
		}
	})

	w.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		in.MouseMove(x, y)
	})

	w.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
		mb, ok := mapMouseButton(button)
		if !ok {
			return
		}
		switch action {
		case glfw.Press:
			in.MouseButtonDown(mb)
		case glfw.Release:
			in.MouseButtonUp(mb)
		}
	})

	w.SetScrollCallback(func(_ *glfw.Window, dx, dy float64) {
		in.MouseScroll(dx, dy)
	})
}

func mapKey(k glfw.Key) input.Key {
	switch k {
	case glfw.KeyW:
		return input.KeyW
	case glfw.KeyA:
		return input.KeyA
	case glfw.KeyS:
		return input.KeyS
	case glfw.KeyD:
		return input.KeyD
	case glfw.KeySpace:
		return input.KeySpace
	case glfw.KeyLeftShift:
		return input.KeyShift
	case glfw.KeyLeftControl:
		return input.KeyCtrl
	case glfw.KeyEscape:
		return input.KeyEscape
	case glfw.KeyLeft:
		return input.KeyLeft
	case glfw.KeyRight:
		return input.KeyRight
	case glfw.KeyUp:
		return input.KeyUp
	case glfw.KeyDown:
		return input.KeyDown
	default:
		if k == glfw.KeyUnknown {
			return input.KeyUnknown
		}
		return input.Key(k)
	}
}

func mapMouseButton(b glfw.MouseButton) (input.MouseButton, bool) {
	switch b {
	case glfw.MouseButtonLeft:
		return input.MouseLeft, true
	case glfw.MouseButtonRight:
		return input.MouseRight, true
	case glfw.MouseButtonMiddle:
		return input.MouseMiddle, true
	default:
		return 0, false
	}
}
