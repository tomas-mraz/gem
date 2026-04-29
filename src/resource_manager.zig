const std = @import("std");

const Allocator = std.mem.Allocator;

pub const Error = error{
    EmptyPath,
    AbsolutePath,
    PathEscapesRoot,
};

/// Caches resource data by path. Not thread-safe — load resources from the
/// engine thread (the Go original used an RwLock; gem-zig uses single-threaded
/// access, which matches typical engine resource loading).
pub const ResourceManager = struct {
    allocator: Allocator,
    root: []const u8,

    bytes_cache: std.StringHashMapUnmanaged([]u8) = .empty,
    float32_cache: std.StringHashMapUnmanaged([]f32) = .empty,

    pub fn init(allocator: Allocator, root: []const u8) !ResourceManager {
        const root_copy = try allocator.dupe(u8, if (root.len == 0) "resources" else root);
        return .{ .allocator = allocator, .root = root_copy };
    }

    pub fn deinit(self: *ResourceManager) void {
        var bit = self.bytes_cache.iterator();
        while (bit.next()) |entry| {
            self.allocator.free(entry.key_ptr.*);
            self.allocator.free(entry.value_ptr.*);
        }
        self.bytes_cache.deinit(self.allocator);

        var fit = self.float32_cache.iterator();
        while (fit.next()) |entry| {
            self.allocator.free(entry.key_ptr.*);
            self.allocator.free(entry.value_ptr.*);
        }
        self.float32_cache.deinit(self.allocator);

        self.allocator.free(self.root);
    }

    /// Caller owns the returned slice.
    pub fn loadBytes(self: *ResourceManager, path: []const u8) ![]u8 {
        if (self.bytes_cache.get(path)) |cached| {
            return self.allocator.dupe(u8, cached);
        }

        const full_path = try self.resolve(path);
        defer self.allocator.free(full_path);

        const data = try readWholeFile(self.allocator, full_path);

        const key_copy = try self.allocator.dupe(u8, path);
        const value_copy = try self.allocator.dupe(u8, data);
        try self.bytes_cache.put(self.allocator, key_copy, value_copy);
        return data;
    }

    fn readWholeFile(allocator: Allocator, path: []const u8) ![]u8 {
        const path_z = try allocator.dupeZ(u8, path);
        defer allocator.free(path_z);

        const fd = try std.posix.openatZ(std.posix.AT.FDCWD, path_z, .{ .ACCMODE = .RDONLY }, 0);
        defer _ = std.os.linux.close(fd);

        var buf: std.ArrayList(u8) = .empty;
        errdefer buf.deinit(allocator);
        var chunk: [4096]u8 = undefined;
        while (true) {
            const n = try std.posix.read(fd, &chunk);
            if (n == 0) break;
            try buf.appendSlice(allocator, chunk[0..n]);
        }
        return try buf.toOwnedSlice(allocator);
    }

    /// Caller owns the returned slice. Decodes a JSON array of numbers.
    pub fn loadFloat32Slice(self: *ResourceManager, path: []const u8) ![]f32 {
        if (self.float32_cache.get(path)) |cached| {
            return self.allocator.dupe(f32, cached);
        }

        const data = try self.loadBytes(path);
        defer self.allocator.free(data);

        const parsed = try std.json.parseFromSlice([]f32, self.allocator, data, .{});
        defer parsed.deinit();
        const owned = try self.allocator.dupe(f32, parsed.value);

        const key_copy = try self.allocator.dupe(u8, path);
        const value_copy = try self.allocator.dupe(f32, owned);
        try self.float32_cache.put(self.allocator, key_copy, value_copy);
        return owned;
    }

    /// Caller owns the returned slice.
    fn resolve(self: *ResourceManager, path: []const u8) ![]u8 {
        if (path.len == 0) return Error.EmptyPath;
        if (std.fs.path.isAbsolute(path)) return Error.AbsolutePath;
        if (std.mem.indexOf(u8, path, "..") != null) return Error.PathEscapesRoot;

        return std.fs.path.join(self.allocator, &.{ self.root, path });
    }
};

test "resolve rejects path traversal" {
    const allocator = std.testing.allocator;
    var rm = try ResourceManager.init(allocator, "resources");
    defer rm.deinit();

    try std.testing.expectError(error.PathEscapesRoot, rm.loadBytes("../etc/passwd"));
    try std.testing.expectError(error.AbsolutePath, rm.loadBytes("/etc/passwd"));
    try std.testing.expectError(error.EmptyPath, rm.loadBytes(""));
}
