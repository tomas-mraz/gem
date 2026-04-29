//! gem — Zig port of the Go game engine at github.com/tomas-mraz/gem.
//!
//! Entity-Component-System data with parallel-array archetypes, fixed-step
//! simulation, scene-event routing, GLFW-polled input, and a Vulkan rendering
//! pipeline driven through vulkan-ash.

const std = @import("std");

pub const ash = @import("ash");

pub const component = @import("component.zig");
pub const Archetype = @import("archetype.zig").Archetype;
pub const ArchetypeHolder = @import("archetype.zig").ArchetypeHolder;

pub const input = @import("input.zig");
pub const Input = input.Input;
pub const Key = input.Key;
pub const MouseButton = input.MouseButton;

pub const actions_mod = @import("actions.zig");
pub const ActionID = actions_mod.ActionID;
pub const ActionState = actions_mod.ActionState;
pub const ActionMap = actions_mod.ActionMap;
pub const ActionSet = actions_mod.ActionSet;
pub const DigitalBinding = actions_mod.DigitalBinding;
pub const AxisBinding = actions_mod.AxisBinding;

pub const ResourceManager = @import("resource_manager.zig").ResourceManager;

pub const scene = @import("scene.zig");
pub const Scene = scene.Scene;
pub const Renderer = scene.Renderer;

pub const Engine = @import("engine.zig").Engine;
pub const Config = @import("engine.zig").Config;
pub const SceneManager = @import("scene_manager.zig").SceneManager;

pub const glfw_input = @import("glfw_input.zig");

test {
    std.testing.refAllDecls(@This());
}
