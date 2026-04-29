const std = @import("std");

const Allocator = std.mem.Allocator;

pub const Error = error{
    EmptyPath,
    AbsolutePath,
    PathEscapesRoot,
    NilManager,
} || std.fs.File.OpenError || std.fs.File.ReadError || std.mem.Allocator.Error || std.json.ParseError(std.json.Scanner);

pub const ResourceManager = struct {
    allocator: Allocator,
    root: []const u8,

    mutex: std.Thread.RwLock = .{},
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
        self.mutex.lockShared();
        if (self.bytes_cache.get(path)) |cached| {
            self.mutex.unlockShared();
            return self.allocator.dupe(u8, cached);
        }
        self.mutex.unlockShared();

        const full_path = try self.resolve(path);
        defer self.allocator.free(full_path);

        const data = try std.fs.cwd().readFileAlloc(self.allocator, full_path, std.math.maxInt(usize));

        self.mutex.lock();
        defer self.mutex.unlock();

        if (!self.bytes_cache.contains(path)) {
            const key_copy = try self.allocator.dupe(u8, path);
            const value_copy = try self.allocator.dupe(u8, data);
            try self.bytes_cache.put(self.allocator, key_copy, value_copy);
        }
        return data;
    }

    /// Caller owns the returned slice. Decodes a JSON array of numbers.
    pub fn loadFloat32Slice(self: *ResourceManager, path: []const u8) ![]f32 {
        self.mutex.lockShared();
        if (self.float32_cache.get(path)) |cached| {
            self.mutex.unlockShared();
            return self.allocator.dupe(f32, cached);
        }
        self.mutex.unlockShared();

        const data = try self.loadBytes(path);
        defer self.allocator.free(data);

        const parsed = try std.json.parseFromSlice([]f32, self.allocator, data, .{});
        defer parsed.deinit();
        const owned = try self.allocator.dupe(f32, parsed.value);

        self.mutex.lock();
        defer self.mutex.unlock();

        if (!self.float32_cache.contains(path)) {
            const key_copy = try self.allocator.dupe(u8, path);
            const value_copy = try self.allocator.dupe(f32, owned);
            try self.float32_cache.put(self.allocator, key_copy, value_copy);
        }
        return owned;
    }

    /// Caller owns the returned slice.
    fn resolve(self: *ResourceManager, path: []const u8) ![]u8 {
        if (path.len == 0) return Error.EmptyPath;
        if (std.fs.path.isAbsolute(path)) return Error.AbsolutePath;

        const joined = try std.fs.path.join(self.allocator, &.{ self.root, path });
        errdefer self.allocator.free(joined);

        const root_clean = try self.allocator.dupe(u8, self.root);
        defer self.allocator.free(root_clean);

        if (std.mem.indexOf(u8, path, "..")) |_| {
            // Reject any path that contains "..". A stricter check would canonicalize;
            // for now this matches the Go original's posture against root escapes.
            self.allocator.free(joined);
            return Error.PathEscapesRoot;
        }
        return joined;
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
