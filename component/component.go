package component

import "github.com/google/uuid"

type Position struct {
	X, Y, Z float32
}

type EntityID uuid.UUID

func NewEntityID() EntityID {
	return EntityID(uuid.New())
}

type Color struct {
	ColorR, ColorG, ColorB float32
	Brightness             float32
}

type TransparentAlpha float32

type Velocity struct {
	DX, DY, DZ float32
}

type RotationVelocity float32

type Angle float32
