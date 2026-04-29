const std = @import("std");
const component = @import("component.zig");

const Allocator = std.mem.Allocator;

pub const Archetype = struct {
    entity: std.ArrayList(component.EntityID),
    position: std.ArrayList(component.Position),

    pub fn init() Archetype {
        return .{
            .entity = .empty,
            .position = .empty,
        };
    }

    pub fn deinit(self: *Archetype, allocator: Allocator) void {
        self.entity.deinit(allocator);
        self.position.deinit(allocator);
    }

    pub fn append(
        self: *Archetype,
        allocator: Allocator,
        id: component.EntityID,
        pos: component.Position,
    ) !void {
        try self.entity.append(allocator, id);
        try self.position.append(allocator, pos);
    }

    pub fn len(self: *const Archetype) usize {
        return self.entity.items.len;
    }
};

pub const ArchetypeHolder = struct {
    entity: component.EntityID,
    position: component.Position,
    velocity: component.Velocity,
};

test "archetype keeps parallel slices in sync" {
    const allocator = std.testing.allocator;
    var arch = Archetype.init();
    defer arch.deinit(allocator);

    try arch.append(allocator, component.newEntityID(), .{ .x = 1, .y = 2, .z = 3 });
    try arch.append(allocator, component.newEntityID(), .{ .x = 4, .y = 5, .z = 6 });

    try std.testing.expectEqual(@as(usize, 2), arch.len());
    try std.testing.expectEqual(@as(f32, 4), arch.position.items[1].x);
}
