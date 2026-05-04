const std = @import("std");
const ash = @import("ash");

const Allocator = std.mem.Allocator;

const Input = @import("input.zig").Input;
const ActionSet = @import("actions.zig").ActionSet;
const ResourceManager = @import("resource_manager.zig").ResourceManager;
const SceneManager = @import("scene_manager.zig").SceneManager;
const Scene = @import("scene.zig").Scene;

pub const Config = struct {
    title: [:0]const u8 = "gem",
    width: u32 = 800,
    height: u32 = 600,
    resource_root: []const u8 = "resources",
    ray_tracing: bool = false,
    /// Fixed simulation rate. 0 means use the default (120 Hz).
    update_hz: u32 = 120,
};

pub const HostImpl = if (ash.is_android) ash.AndroidHost else ash.DesktopHost;

const max_frame_time_s: f64 = 1.0 / 30.0;
const dt_spike_log_above_s: f64 = 0.020;
pub const max_sub_steps: u32 = 5;

/// Engine wires the host, Vulkan session, scene manager, input and resources
/// together. The engine owns the host and session for their full lifetime; the
/// scene manager calls back into Engine.tick once per render frame to advance
/// timing and drain accumulated time into fixed-step updates.
pub const Engine = struct {
    allocator: Allocator,
    config: Config,

    input: Input,
    actions: ActionSet,
    resources: ResourceManager,

    host: HostImpl,
    session: ash.Session,
    scene_manager: SceneManager,

    total_elapsed: f64 = 0,
    scene_elapsed: f64 = 0,
    frame_time: f64 = 0,
    fixed_dt: f64,
    accumulator: f64 = 0,
    interp_alpha: f64 = 0,

    last_time: u64,

    pub fn init(allocator: Allocator, cfg: Config) !*Engine {
        if (ash.is_android) {
            @compileError("On Android use Engine.initAndroid(allocator, cfg, android_app).");
        }
        var config = cfg;
        if (config.update_hz == 0) config.update_hz = 120;

        const engine = try allocator.create(Engine);
        errdefer allocator.destroy(engine);

        ash.setDebug(true);
        ash.setValidations(false);

        engine.* = .{
            .allocator = allocator,
            .config = config,
            .input = Input.init(allocator),
            .actions = ActionSet.init(allocator),
            .resources = try ResourceManager.init(allocator, config.resource_root),
            .host = ash.DesktopHost.init(allocator, config.width, config.height, config.title),
            .session = undefined,
            .scene_manager = undefined,
            .fixed_dt = 1.0 / @as(f64, @floatFromInt(config.update_hz)),
            .last_time = nowNs(),
        };

        var session_opts: ash.SessionOptions = .{};
        if (config.ray_tracing) {
            session_opts.device_options = rayTracingDeviceOptions();
        }
        engine.session = ash.Session.init(
            allocator,
            engine.host.asHost(),
            config.title,
            session_opts,
        );

        engine.scene_manager = SceneManager.init(allocator, engine);
        return engine;
    }

    pub fn initAndroid(
        allocator: Allocator,
        cfg: Config,
        android_app: *ash.native_app_glue.android_app,
    ) !*Engine {
        if (!ash.is_android) {
            @compileError("Engine.initAndroid is only available on Android targets.");
        }
        var config = cfg;
        if (config.update_hz == 0) config.update_hz = 120;

        const engine = try allocator.create(Engine);
        errdefer allocator.destroy(engine);

        ash.setDebug(false);
        ash.setValidations(false);

        engine.* = .{
            .allocator = allocator,
            .config = config,
            .input = Input.init(allocator),
            .actions = ActionSet.init(allocator),
            .resources = try ResourceManager.init(allocator, config.resource_root),
            .host = ash.AndroidHost.init(allocator, android_app),
            .session = undefined,
            .scene_manager = undefined,
            .fixed_dt = 1.0 / @as(f64, @floatFromInt(config.update_hz)),
            .last_time = nowNs(),
        };

        var session_opts: ash.SessionOptions = .{};
        if (config.ray_tracing) {
            session_opts.device_options = rayTracingDeviceOptions();
        }
        engine.session = ash.Session.init(
            allocator,
            engine.host.asHost(),
            config.title,
            session_opts,
        );

        engine.scene_manager = SceneManager.init(allocator, engine);
        return engine;
    }

    pub fn deinit(self: *Engine) void {
        self.scene_manager.deinit();
        self.resources.deinit();
        self.actions.deinit();
        self.input.deinit();
        self.allocator.destroy(self);
    }

    /// Run a single scene without registering routes. Mirrors gem-go Engine.Run.
    pub fn run(self: *Engine, scene: Scene) !void {
        try self.scene_manager.register("__single__", scene);
        try self.scene_manager.run("__single__");
    }

    /// Called by SceneManager once per render frame, before fixed updates.
    pub fn tick(self: *Engine) void {
        const now = nowNs();
        const elapsed_ns = now -% self.last_time;
        const raw = @as(f64, @floatFromInt(elapsed_ns)) / @as(f64, std.time.ns_per_s);
        self.last_time = now;

        var clamped = raw;
        if (clamped > max_frame_time_s) clamped = max_frame_time_s;
        if (raw > dt_spike_log_above_s) {
            std.log.debug("frame time spike: raw={d:.2}ms clamped={d:.2}ms", .{ raw * 1000.0, clamped * 1000.0 });
        }

        self.frame_time = clamped;
        self.total_elapsed += clamped;
        self.scene_elapsed += clamped;
        self.accumulator += clamped;
        self.input.tick(self.total_elapsed);
    }

    pub fn advanceFixedStep(self: *Engine) bool {
        if (self.accumulator < self.fixed_dt) return false;
        self.accumulator -= self.fixed_dt;
        return true;
    }

    pub fn computeInterpAlpha(self: *Engine) void {
        var a = self.accumulator / self.fixed_dt;
        if (a < 0) a = 0 else if (a > 1) a = 1;
        self.interp_alpha = a;
    }

    pub fn discardAccumulator(self: *Engine) void {
        self.accumulator = 0;
        self.interp_alpha = 0;
    }

    pub fn resetSceneClock(self: *Engine) void {
        self.scene_elapsed = 0;
        self.last_time = nowNs();
        self.discardAccumulator();
    }

    pub fn elapsed(self: *const Engine) f64 {
        return self.total_elapsed;
    }
    pub fn sceneElapsed(self: *const Engine) f64 {
        return self.scene_elapsed;
    }
    pub fn deltaTime(self: *const Engine) f64 {
        return self.fixed_dt;
    }
    pub fn frameTime(self: *const Engine) f64 {
        return self.frame_time;
    }
    pub fn interpolationAlpha(self: *const Engine) f64 {
        return self.interp_alpha;
    }
};

fn nowNs() u64 {
    return switch (@import("builtin").os.tag) {
        .windows => blk: {
            var counter: std.os.windows.LARGE_INTEGER = undefined;
            var freq: std.os.windows.LARGE_INTEGER = undefined;
            _ = std.os.windows.ntdll.RtlQueryPerformanceCounter(&counter);
            _ = std.os.windows.ntdll.RtlQueryPerformanceFrequency(&freq);
            const c: u64 = @intCast(counter);
            const f: u64 = @intCast(freq);
            break :blk @divFloor(c, f) * std.time.ns_per_s +
                @divFloor((c % f) * std.time.ns_per_s, f);
        },
        else => blk: {
            var ts: std.c.timespec = undefined;
            _ = std.c.clock_gettime(.MONOTONIC, &ts);
            break :blk @as(u64, @intCast(ts.sec)) * std.time.ns_per_s +
                @as(u64, @intCast(ts.nsec));
        },
    };
}

fn rayTracingDeviceOptions() ash.DeviceOptions {
    return .{
        .api_version = ash.vk.API_VERSION_1_2.toU32(),
    };
}

test {
    _ = @import("input.zig");
    _ = @import("actions.zig");
}
