//! Bridges polled SDL keyboard/mouse state into a gem.Input.
//!
//! ash.DesktopHost owns the SDL event loop (it drains the queue for window and
//! surface events), so we read SDL's keyboard and mouse state, which the host's
//! pump keeps up to date, instead of consuming input events. Polling is
//! sufficient at the engine's fixed update rate.

const ash = @import("ash");
const sdl = ash.sdl;
const Input = @import("input.zig").Input;
const Key = @import("input.zig").Key;
const MouseButton = @import("input.zig").MouseButton;
const ActionMap = @import("actions.zig").ActionMap;

pub fn poll(input: *Input, map: ActionMap) void {
    var num_keys: c_int = 0;
    const keyboard = sdl.SDL_GetKeyboardState(&num_keys);
    const keys = keyboard[0..@intCast(num_keys)];

    for (map.digital) |binding| syncKey(keys, input, binding.key);
    for (map.axes) |binding| {
        syncKey(keys, input, binding.negative);
        syncKey(keys, input, binding.positive);
    }

    var x: f32 = 0;
    var y: f32 = 0;
    const buttons = sdl.SDL_GetMouseState(&x, &y);

    syncMouseButton(buttons, input, .left, sdl.SDL_BUTTON_LEFT);
    syncMouseButton(buttons, input, .right, sdl.SDL_BUTTON_RIGHT);
    syncMouseButton(buttons, input, .middle, sdl.SDL_BUTTON_MIDDLE);

    input.mouseMove(x, y);
}

fn syncKey(keys: []const bool, input: *Input, key: Key) void {
    const scancode: usize = @intCast(toSdlScancode(key) orelse return);
    const is_down = scancode < keys.len and keys[scancode];
    if (is_down) input.keyDown(key) else input.keyUp(key);
}

fn syncMouseButton(
    buttons: sdl.SDL_MouseButtonFlags,
    input: *Input,
    btn: MouseButton,
    sdl_btn: c_int,
) void {
    // SDL_BUTTON_MASK(X) is (1u << ((X) - 1)).
    const mask = @as(sdl.SDL_MouseButtonFlags, 1) << @intCast(sdl_btn - 1);
    const is_down = buttons & mask != 0;
    if (is_down) input.mouseButtonDown(btn) else input.mouseButtonUp(btn);
}

// Scancodes name the physical key position (US layout), like GLFW key tokens.
fn toSdlScancode(k: Key) ?c_uint {
    return switch (k) {
        .a => sdl.SDL_SCANCODE_A,
        .b => sdl.SDL_SCANCODE_B,
        .c => sdl.SDL_SCANCODE_C,
        .d => sdl.SDL_SCANCODE_D,
        .e => sdl.SDL_SCANCODE_E,
        .f => sdl.SDL_SCANCODE_F,
        .g => sdl.SDL_SCANCODE_G,
        .h => sdl.SDL_SCANCODE_H,
        .i => sdl.SDL_SCANCODE_I,
        .j => sdl.SDL_SCANCODE_J,
        .k => sdl.SDL_SCANCODE_K,
        .l => sdl.SDL_SCANCODE_L,
        .m => sdl.SDL_SCANCODE_M,
        .n => sdl.SDL_SCANCODE_N,
        .o => sdl.SDL_SCANCODE_O,
        .p => sdl.SDL_SCANCODE_P,
        .q => sdl.SDL_SCANCODE_Q,
        .r => sdl.SDL_SCANCODE_R,
        .s => sdl.SDL_SCANCODE_S,
        .t => sdl.SDL_SCANCODE_T,
        .u => sdl.SDL_SCANCODE_U,
        .v => sdl.SDL_SCANCODE_V,
        .w => sdl.SDL_SCANCODE_W,
        .x => sdl.SDL_SCANCODE_X,
        .y => sdl.SDL_SCANCODE_Y,
        .z => sdl.SDL_SCANCODE_Z,
        .space => sdl.SDL_SCANCODE_SPACE,
        .enter => sdl.SDL_SCANCODE_RETURN,
        .shift => sdl.SDL_SCANCODE_LSHIFT,
        .ctrl => sdl.SDL_SCANCODE_LCTRL,
        .alt => sdl.SDL_SCANCODE_LALT,
        .escape => sdl.SDL_SCANCODE_ESCAPE,
        .tab => sdl.SDL_SCANCODE_TAB,
        .backspace => sdl.SDL_SCANCODE_BACKSPACE,
        .left => sdl.SDL_SCANCODE_LEFT,
        .right => sdl.SDL_SCANCODE_RIGHT,
        .up => sdl.SDL_SCANCODE_UP,
        .down => sdl.SDL_SCANCODE_DOWN,
        .unknown => null,
        _ => null,
    };
}
