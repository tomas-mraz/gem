const std = @import("std");
const ash = @import("ash");
const ActionMap = @import("actions.zig").ActionMap;

pub const Engine = @import("engine.zig").Engine;

pub const Renderer = struct {
    ptr: *anyopaque,
    vtable: *const VTable,

    pub const VTable = struct {
        createOnce: *const fn (ptr: *anyopaque, session: *ash.Session) anyerror!void,
        createSized: *const fn (ptr: *anyopaque, session: *ash.Session, extent: ash.vk.Extent2D) anyerror!void,
        destroySized: *const fn (ptr: *anyopaque) void,
        destroyOnce: *const fn (ptr: *anyopaque) void,
        draw: *const fn (ptr: *anyopaque, session: *ash.Session, frame: *const ash.Frame) anyerror!void,
    };

    pub fn createOnce(self: Renderer, session: *ash.Session) !void {
        return self.vtable.createOnce(self.ptr, session);
    }
    pub fn createSized(self: Renderer, session: *ash.Session, extent: ash.vk.Extent2D) !void {
        return self.vtable.createSized(self.ptr, session, extent);
    }
    pub fn destroySized(self: Renderer) void {
        self.vtable.destroySized(self.ptr);
    }
    pub fn destroyOnce(self: Renderer) void {
        self.vtable.destroyOnce(self.ptr);
    }
    pub fn draw(self: Renderer, session: *ash.Session, frame: *const ash.Frame) !void {
        return self.vtable.draw(self.ptr, session, frame);
    }
};

/// Scene is the lifecycle interface implemented by every gameplay scene.
///
/// load runs once when the scene is first registered and should allocate
/// long-lived resources. enter runs every time the scene becomes active;
/// reset transient state there. update returns true to ask the manager to
/// transition out of the scene (the scene typically also emits an event).
/// exit runs before deactivation, unload before unregistration. actions
/// returns the scene's action bindings. renderer returns the GPU-side
/// renderer the scene owns.
pub const Scene = struct {
    ptr: *anyopaque,
    vtable: *const VTable,

    pub const VTable = struct {
        load: *const fn (ptr: *anyopaque, e: *Engine) anyerror!void,
        enter: *const fn (ptr: *anyopaque, e: *Engine) anyerror!void,
        update: *const fn (ptr: *anyopaque, e: *Engine) bool,
        exit: *const fn (ptr: *anyopaque, e: *Engine) void,
        unload: *const fn (ptr: *anyopaque, e: *Engine) void,
        actions: *const fn (ptr: *anyopaque) ActionMap,
        renderer: *const fn (ptr: *anyopaque) Renderer,
        /// Optional. If non-null the engine calls it once per render frame after
        /// fixed updates and before draw.
        preRender: ?*const fn (ptr: *anyopaque, e: *Engine) void = null,
    };

    pub fn load(self: Scene, e: *Engine) !void {
        return self.vtable.load(self.ptr, e);
    }
    pub fn enter(self: Scene, e: *Engine) !void {
        return self.vtable.enter(self.ptr, e);
    }
    pub fn update(self: Scene, e: *Engine) bool {
        return self.vtable.update(self.ptr, e);
    }
    pub fn exit(self: Scene, e: *Engine) void {
        self.vtable.exit(self.ptr, e);
    }
    pub fn unload(self: Scene, e: *Engine) void {
        self.vtable.unload(self.ptr, e);
    }
    pub fn actions(self: Scene) ActionMap {
        return self.vtable.actions(self.ptr);
    }
    pub fn renderer(self: Scene) Renderer {
        return self.vtable.renderer(self.ptr);
    }
    pub fn preRender(self: Scene, e: *Engine) void {
        if (self.vtable.preRender) |f| f(self.ptr, e);
    }
};
