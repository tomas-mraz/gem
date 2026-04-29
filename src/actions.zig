const std = @import("std");
const Input = @import("input.zig").Input;
const Key = @import("input.zig").Key;

pub const ActionID = []const u8;

pub const ActionState = struct {
    value: f32 = 0,
    down: bool = false,
    just_pressed: bool = false,
    just_released: bool = false,
};

pub const DigitalBinding = struct {
    action: ActionID,
    key: Key,
};

pub const AxisBinding = struct {
    action: ActionID,
    negative: Key,
    positive: Key,
};

pub const ActionMap = struct {
    digital: []const DigitalBinding = &.{},
    axes: []const AxisBinding = &.{},
};

pub const ActionSet = struct {
    states: std.StringHashMapUnmanaged(ActionState) = .empty,
    allocator: std.mem.Allocator,

    pub fn init(allocator: std.mem.Allocator) ActionSet {
        return .{ .allocator = allocator };
    }

    pub fn deinit(self: *ActionSet) void {
        self.states.deinit(self.allocator);
    }

    pub fn update(self: *ActionSet, input: *const Input, bindings: ActionMap) !void {
        self.states.clearRetainingCapacity();

        for (bindings.digital) |binding| {
            const gop = try self.states.getOrPut(self.allocator, binding.action);
            if (!gop.found_existing) gop.value_ptr.* = .{};
            const state = gop.value_ptr;

            if (input.keyDownQ(binding.key)) {
                state.down = true;
                if (state.value < 1) state.value = 1;
            }
            state.just_pressed = state.just_pressed or input.keyJustPressed(binding.key);
            state.just_released = state.just_released or input.keyJustReleased(binding.key);
        }

        for (bindings.axes) |binding| {
            const gop = try self.states.getOrPut(self.allocator, binding.action);
            if (!gop.found_existing) gop.value_ptr.* = .{};
            const state = gop.value_ptr;

            var value: f32 = 0;
            if (input.keyDownQ(binding.negative)) value -= 1;
            if (input.keyDownQ(binding.positive)) value += 1;
            state.value = maxAbs(state.value, value);
            state.down = state.down or value != 0;
            state.just_pressed = state.just_pressed or
                input.keyJustPressed(binding.negative) or
                input.keyJustPressed(binding.positive);
            state.just_released = state.just_released or
                input.keyJustReleased(binding.negative) or
                input.keyJustReleased(binding.positive);
        }
    }

    pub fn clear(self: *ActionSet) void {
        self.states.clearRetainingCapacity();
    }

    pub fn state(self: *const ActionSet, action: ActionID) ActionState {
        return self.states.get(action) orelse .{};
    }

    pub fn value(self: *const ActionSet, action: ActionID) f32 {
        return self.state(action).value;
    }

    pub fn down(self: *const ActionSet, action: ActionID) bool {
        return self.state(action).down;
    }

    pub fn justPressed(self: *const ActionSet, action: ActionID) bool {
        return self.state(action).just_pressed;
    }

    pub fn justReleased(self: *const ActionSet, action: ActionID) bool {
        return self.state(action).just_released;
    }
};

fn maxAbs(current: f32, candidate: f32) f32 {
    return if (@abs(candidate) > @abs(current)) candidate else current;
}

test "axis binding resolves to negative on its own" {
    const allocator = std.testing.allocator;
    var input = Input.init(allocator);
    defer input.deinit();
    var actions = ActionSet.init(allocator);
    defer actions.deinit();

    input.keyDown(.a);

    const map: ActionMap = .{
        .axes = &.{
            .{ .action = "horizontal", .negative = .a, .positive = .d },
        },
    };
    try actions.update(&input, map);

    try std.testing.expectEqual(@as(f32, -1), actions.value("horizontal"));
    try std.testing.expect(actions.down("horizontal"));
}
