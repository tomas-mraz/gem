const std = @import("std");

pub const EntityID = u64;

var next_entity_id: std.atomic.Value(u64) = .init(1);

pub fn newEntityID() EntityID {
    return next_entity_id.fetchAdd(1, .monotonic);
}

pub const Position = extern struct {
    x: f32 = 0,
    y: f32 = 0,
    z: f32 = 0,
};

pub const Velocity = extern struct {
    dx: f32 = 0,
    dy: f32 = 0,
    dz: f32 = 0,
};

pub const Color = extern struct {
    r: f32 = 0,
    g: f32 = 0,
    b: f32 = 0,
    brightness: f32 = 1,
};

pub const TransparentAlpha = f32;
pub const Angle = f32;
pub const RotationVelocity = f32;

test "fresh entity ids are unique and monotonic" {
    const a = newEntityID();
    const b = newEntityID();
    try std.testing.expect(b > a);
}
