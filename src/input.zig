const std = @import("std");

pub const Key = enum(u16) {
    unknown,
    a,
    b,
    c,
    d,
    e,
    f,
    g,
    h,
    i,
    j,
    k,
    l,
    m,
    n,
    o,
    p,
    q,
    r,
    s,
    t,
    u,
    v,
    w,
    x,
    y,
    z,
    space,
    enter,
    shift,
    ctrl,
    alt,
    escape,
    tab,
    backspace,
    left,
    right,
    up,
    down,
    _,
};

pub const MouseButton = enum(u8) {
    left,
    right,
    middle,
};

const KeyState = packed struct {
    down: bool = false,
    just_pressed: bool = false,
    just_released: bool = false,
};

pub const Input = struct {
    keys: std.AutoHashMapUnmanaged(Key, KeyState) = .empty,
    mouse_buttons: [3]KeyState = .{ .{}, .{}, .{} },
    mouse_x: f64 = 0,
    mouse_y: f64 = 0,
    scroll_dx: f64 = 0,
    scroll_dy: f64 = 0,
    last_tick_elapsed: f64 = 0,
    allocator: std.mem.Allocator,

    pub fn init(allocator: std.mem.Allocator) Input {
        return .{ .allocator = allocator };
    }

    pub fn deinit(self: *Input) void {
        self.keys.deinit(self.allocator);
    }

    /// Tick clears one-shot flags so JustPressed/JustReleased only fire for one
    /// frame. Per-key down state stays sticky until KeyUp is called.
    pub fn tick(self: *Input, total_elapsed: f64) void {
        self.last_tick_elapsed = total_elapsed;
        self.clearTransients();
    }

    /// Clear one-shot flags without advancing the clock. Used by SceneManager
    /// after a scene transition so a key still held from the previous scene
    /// does not appear as just-pressed to the new scene in the same frame.
    pub fn clearTransients(self: *Input) void {
        var it = self.keys.iterator();
        while (it.next()) |entry| {
            entry.value_ptr.just_pressed = false;
            entry.value_ptr.just_released = false;
        }
        for (&self.mouse_buttons) |*state| {
            state.just_pressed = false;
            state.just_released = false;
        }
        self.scroll_dx = 0;
        self.scroll_dy = 0;
    }

    pub fn keyDown(self: *Input, key: Key) void {
        const gop = self.keys.getOrPut(self.allocator, key) catch return;
        if (!gop.found_existing) gop.value_ptr.* = .{};
        if (!gop.value_ptr.down) gop.value_ptr.just_pressed = true;
        gop.value_ptr.down = true;
    }

    pub fn keyUp(self: *Input, key: Key) void {
        const gop = self.keys.getOrPut(self.allocator, key) catch return;
        if (!gop.found_existing) gop.value_ptr.* = .{};
        if (gop.value_ptr.down) gop.value_ptr.just_released = true;
        gop.value_ptr.down = false;
    }

    pub fn keyDownQ(self: *const Input, key: Key) bool {
        const state = self.keys.get(key) orelse return false;
        return state.down;
    }

    pub fn keyJustPressed(self: *const Input, key: Key) bool {
        const state = self.keys.get(key) orelse return false;
        return state.just_pressed;
    }

    pub fn keyJustReleased(self: *const Input, key: Key) bool {
        const state = self.keys.get(key) orelse return false;
        return state.just_released;
    }

    pub fn mouseMove(self: *Input, x: f64, y: f64) void {
        self.mouse_x = x;
        self.mouse_y = y;
    }

    pub fn mouseButtonDown(self: *Input, button: MouseButton) void {
        const idx = @intFromEnum(button);
        if (!self.mouse_buttons[idx].down) self.mouse_buttons[idx].just_pressed = true;
        self.mouse_buttons[idx].down = true;
    }

    pub fn mouseButtonUp(self: *Input, button: MouseButton) void {
        const idx = @intFromEnum(button);
        if (self.mouse_buttons[idx].down) self.mouse_buttons[idx].just_released = true;
        self.mouse_buttons[idx].down = false;
    }

    pub fn mouseScroll(self: *Input, dx: f64, dy: f64) void {
        self.scroll_dx += dx;
        self.scroll_dy += dy;
    }
};

test "key just_pressed fires once and decays after tick" {
    var input = Input.init(std.testing.allocator);
    defer input.deinit();

    input.keyDown(.w);
    try std.testing.expect(input.keyDownQ(.w));
    try std.testing.expect(input.keyJustPressed(.w));

    input.tick(0.016);
    try std.testing.expect(input.keyDownQ(.w));
    try std.testing.expect(!input.keyJustPressed(.w));

    input.keyUp(.w);
    try std.testing.expect(!input.keyDownQ(.w));
    try std.testing.expect(input.keyJustReleased(.w));
}
