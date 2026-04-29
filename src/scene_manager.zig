const std = @import("std");
const ash = @import("ash");

const Allocator = std.mem.Allocator;

const Engine = @import("engine.zig").Engine;
const Scene = @import("scene.zig").Scene;
const Renderer = @import("scene.zig").Renderer;
const ActionMap = @import("actions.zig").ActionMap;
const max_sub_steps = @import("engine.zig").max_sub_steps;
const glfw_input = @import("glfw_input.zig");

const SceneRegistration = struct {
    id: []const u8,
    scene: Scene,
    renderer: Renderer,
    routes: std.StringHashMapUnmanaged([]const u8) = .empty,
    loaded: bool = false,
    renderer_once_ready: bool = false,
    renderer_sized_ready: bool = false,
};

/// SceneManager owns scene registration, lifecycle and event-based routing.
/// It also doubles as the renderer that ash.Session drives — the createOnce/
/// createSized/destroySized/destroyOnce/draw methods are the contract Session
/// expects via duck typing.
pub const SceneManager = struct {
    allocator: Allocator,
    engine: *Engine,
    scenes: std.StringHashMapUnmanaged(*SceneRegistration) = .empty,
    current: ?*SceneRegistration = null,
    pending_event: ?[]u8 = null,
    enter_pending: bool = false,

    pub fn init(allocator: Allocator, engine: *Engine) SceneManager {
        return .{ .allocator = allocator, .engine = engine };
    }

    pub fn deinit(self: *SceneManager) void {
        if (self.current) |reg| {
            reg.scene.exit(self.engine);
            self.current = null;
        }

        var it = self.scenes.iterator();
        while (it.next()) |entry| {
            const reg = entry.value_ptr.*;
            self.destroyRenderer(reg);
            if (reg.loaded) {
                reg.scene.unload(self.engine);
                reg.loaded = false;
            }

            var rit = reg.routes.iterator();
            while (rit.next()) |route| {
                self.allocator.free(route.key_ptr.*);
                self.allocator.free(route.value_ptr.*);
            }
            reg.routes.deinit(self.allocator);

            self.allocator.free(reg.id);
            self.allocator.destroy(reg);
        }
        self.scenes.deinit(self.allocator);

        if (self.pending_event) |ev| {
            self.allocator.free(ev);
            self.pending_event = null;
        }
    }

    pub fn register(self: *SceneManager, id: []const u8, scene: Scene) !void {
        if (id.len == 0) return error.EmptySceneID;
        if (self.scenes.contains(id)) return error.SceneAlreadyRegistered;

        const reg = try self.allocator.create(SceneRegistration);
        errdefer self.allocator.destroy(reg);

        const id_copy = try self.allocator.dupe(u8, id);
        errdefer self.allocator.free(id_copy);

        reg.* = .{
            .id = id_copy,
            .scene = scene,
            .renderer = scene.renderer(),
        };
        try self.scenes.put(self.allocator, id_copy, reg);
    }

    pub fn bind(self: *SceneManager, from_scene_id: []const u8, event: []const u8, to_scene_id: []const u8) !void {
        if (from_scene_id.len == 0 or event.len == 0 or to_scene_id.len == 0) {
            return error.InvalidBindArguments;
        }
        const from_reg = self.scenes.get(from_scene_id) orelse return error.UnknownFromScene;
        if (!self.scenes.contains(to_scene_id)) return error.UnknownToScene;

        const event_copy = try self.allocator.dupe(u8, event);
        errdefer self.allocator.free(event_copy);
        const target_copy = try self.allocator.dupe(u8, to_scene_id);
        errdefer self.allocator.free(target_copy);

        try from_reg.routes.put(self.allocator, event_copy, target_copy);
    }

    pub fn emit(self: *SceneManager, event: []const u8) void {
        if (event.len == 0) return;
        if (self.pending_event) |existing| self.allocator.free(existing);
        self.pending_event = self.allocator.dupe(u8, event) catch return;
    }

    pub fn run(self: *SceneManager, start_scene_id: []const u8) !void {
        try self.prepareStart(start_scene_id);
        try self.engine.session.run(self);
    }

    fn prepareStart(self: *SceneManager, id: []const u8) !void {
        const reg = self.scenes.get(id) orelse return error.UnknownScene;
        try self.ensureSceneLoaded(reg);
        self.current = reg;
        self.enter_pending = true;
    }

    fn ensureSceneLoaded(self: *SceneManager, reg: *SceneRegistration) !void {
        if (reg.loaded) return;
        try reg.scene.load(self.engine);
        reg.loaded = true;
    }

    fn ensureRendererOnce(self: *SceneManager, reg: *SceneRegistration, session: *ash.Session) !void {
        if (reg.renderer_once_ready) return;
        try reg.renderer.createOnce(session);
        reg.renderer_once_ready = true;
    }

    fn ensureRendererSized(self: *SceneManager, reg: *SceneRegistration, session: *ash.Session, extent: ash.vk.Extent2D) !void {
        if (reg.renderer_sized_ready) return;
        try reg.renderer.createSized(session, extent);
        reg.renderer_sized_ready = true;
    }

    fn destroyRendererSized(self: *SceneManager, reg: *SceneRegistration) void {
        _ = self;
        if (!reg.renderer_sized_ready) return;
        reg.renderer.destroySized();
        reg.renderer_sized_ready = false;
    }

    fn destroyRenderer(self: *SceneManager, reg: *SceneRegistration) void {
        self.destroyRendererSized(reg);
        if (reg.renderer_once_ready) {
            reg.renderer.destroyOnce();
            reg.renderer_once_ready = false;
        }
    }

    fn ensureCurrentEntered(self: *SceneManager) !void {
        if (self.current == null or !self.enter_pending) return;
        self.engine.resetSceneClock();
        try self.current.?.scene.enter(self.engine);
        self.enter_pending = false;
    }

    fn updateActions(self: *SceneManager) !void {
        if (self.current == null) {
            self.engine.actions.clear();
            return;
        }
        // Poll GLFW into Input first so the action update sees fresh state.
        if (self.engine.host.window) |window| {
            glfw_input.poll(window, &self.engine.input, self.current.?.scene.actions());
        }
        try self.engine.actions.update(&self.engine.input, self.current.?.scene.actions());
    }

    fn consumePendingEvent(self: *SceneManager) ?[]u8 {
        const ev = self.pending_event orelse return null;
        self.pending_event = null;
        return ev;
    }

    fn transitionFromCurrent(self: *SceneManager, session: *ash.Session) !void {
        const from = self.current orelse return;

        const ev = self.consumePendingEvent();
        if (ev == null) {
            self.engine.host.shutdown(); // request close: matches gem-go RequestClose
            return;
        }
        defer self.allocator.free(ev.?);

        const target = from.routes.get(ev.?) orelse return error.NoRouteBound;

        from.scene.exit(self.engine);
        self.destroyRendererSized(from);

        const next = self.scenes.get(target) orelse return error.UnknownScene;
        try self.ensureSceneLoaded(next);
        try self.ensureRendererOnce(next, session);
        if (session.swapchain) |sc| {
            try self.ensureRendererSized(next, session, sc.extent);
        }

        self.current = next;
        self.enter_pending = true;
        self.engine.actions.clear();
        try self.ensureCurrentEntered();
    }

    // -- Renderer interface (called by ash.Session via duck typing) -----------

    pub fn createOnce(self: *SceneManager, session: *ash.Session) !void {
        const reg = self.current orelse return;
        try self.ensureRendererOnce(reg, session);
    }

    pub fn createSized(self: *SceneManager, session: *ash.Session, extent: ash.vk.Extent2D) !void {
        const reg = self.current orelse return;
        try self.ensureRendererSized(reg, session, extent);
    }

    pub fn destroySized(self: *SceneManager) void {
        if (self.current) |reg| self.destroyRendererSized(reg);
    }

    pub fn destroyOnce(self: *SceneManager) void {
        if (self.current) |reg| {
            reg.scene.exit(self.engine);
            self.current = null;
            _ = reg;
        }
        var it = self.scenes.iterator();
        while (it.next()) |entry| self.destroyRenderer(entry.value_ptr.*);
    }

    pub fn draw(self: *SceneManager, session: *ash.Session, frame: *const ash.Frame) !void {
        const reg = self.current orelse return error.NoActiveScene;

        self.engine.tick();

        try self.ensureCurrentEntered();
        try self.updateActions();

        var steps: u32 = 0;
        while (steps < max_sub_steps and self.engine.advanceFixedStep()) {
            if (reg.scene.update(self.engine)) {
                try self.transitionFromCurrent(session);
                if (self.current == null) return error.NoActiveScene;
                try self.ensureCurrentEntered();
                self.engine.discardAccumulator();
                try self.updateActions();
                break;
            }
            steps += 1;
        }
        if (steps == max_sub_steps) self.engine.discardAccumulator();

        self.engine.computeInterpAlpha();

        self.current.?.scene.preRender(self.engine);
        try self.current.?.renderer.draw(session, frame);
    }
};
