//! Bridges polled GLFW state into a gem.Input.
//!
//! ash.DesktopHost owns the GLFW window's user pointer to receive iconify and
//! framebuffer-size events, so we cannot install our own key/mouse callbacks
//! without conflicting. Polling instead is sufficient at the engine's fixed
//! update rate and keeps gem decoupled from GLFW's callback registration.

const ash = @import("ash");
const Input = @import("input.zig").Input;
const Key = @import("input.zig").Key;
const MouseButton = @import("input.zig").MouseButton;
const ActionMap = @import("actions.zig").ActionMap;

pub fn poll(window: ash.glfw.Window, input: *Input, map: ActionMap) void {
    for (map.digital) |binding| syncKey(window, input, binding.key);
    for (map.axes) |binding| {
        syncKey(window, input, binding.negative);
        syncKey(window, input, binding.positive);
    }

    syncMouseButton(window, input, .left, .left);
    syncMouseButton(window, input, .right, .right);
    syncMouseButton(window, input, .middle, .middle);

    const cursor = window.getCursorPos();
    input.mouseMove(cursor.xpos, cursor.ypos);
}

fn syncKey(window: ash.glfw.Window, input: *Input, key: Key) void {
    const gk = toGlfwKey(key) orelse return;
    const action = window.getKey(gk);
    const is_down = action == .press or action == .repeat;
    if (is_down) input.keyDown(key) else input.keyUp(key);
}

fn syncMouseButton(
    window: ash.glfw.Window,
    input: *Input,
    btn: MouseButton,
    glfw_btn: ash.glfw.MouseButton,
) void {
    const action = window.getMouseButton(glfw_btn);
    const is_down = action == .press or action == .repeat;
    if (is_down) input.mouseButtonDown(btn) else input.mouseButtonUp(btn);
}

fn toGlfwKey(k: Key) ?ash.glfw.Key {
    return switch (k) {
        .a => .a,
        .b => .b,
        .c => .c,
        .d => .d,
        .e => .e,
        .f => .f,
        .g => .g,
        .h => .h,
        .i => .i,
        .j => .j,
        .k => .k,
        .l => .l,
        .m => .m,
        .n => .n,
        .o => .o,
        .p => .p,
        .q => .q,
        .r => .r,
        .s => .s,
        .t => .t,
        .u => .u,
        .v => .v,
        .w => .w,
        .x => .x,
        .y => .y,
        .z => .z,
        .space => .space,
        .enter => .enter,
        .shift => .left_shift,
        .ctrl => .left_control,
        .alt => .left_alt,
        .escape => .escape,
        .tab => .tab,
        .backspace => .backspace,
        .left => .left,
        .right => .right,
        .up => .up,
        .down => .down,
        .unknown => null,
        _ => null,
    };
}
