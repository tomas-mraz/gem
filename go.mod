module github.com/tomas-mraz/gem

go 1.25.7

require (
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20250301202403-da16c1255728
	github.com/google/uuid v1.6.0
	github.com/tomas-mraz/input v0.0.0
	github.com/tomas-mraz/vulkan v0.0.0-20260323112817-4f29e510696c
	github.com/tomas-mraz/vulkan-ash v0.0.0-20260323122016-b1e454795046
)

replace github.com/tomas-mraz/vulkan-ash => /home/tomas/git-osobni-github/vulkan-ash

replace github.com/tomas-mraz/vulkan => /home/tomas/git-osobni-github/vulkan-goki_fork

replace github.com/tomas-mraz/input => /home/tomas/git-osobni-github/input
