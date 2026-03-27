package main

import _ "embed"

var (
	//go:embed shaders/default.vert.spv
	defaultVertShader []byte

	//go:embed shaders/default.frag.spv
	defaultFragShader []byte
)
