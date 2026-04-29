const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const ash_dep = b.dependency("vulkan_ash", .{
        .target = target,
        .optimize = optimize,
    });
    const ash_module = ash_dep.module("ash");

    const gem_module = b.addModule("gem", .{
        .root_source_file = b.path("src/root.zig"),
        .target = target,
        .optimize = optimize,
        .link_libc = true,
        .imports = &.{
            .{ .name = "ash", .module = ash_module },
        },
    });

    const lib = b.addLibrary(.{
        .name = "gem",
        .linkage = .static,
        .root_module = gem_module,
    });
    b.installArtifact(lib);

    const tests = b.addTest(.{
        .root_module = gem_module,
    });
    const test_run = b.addRunArtifact(tests);
    const test_step = b.step("test", "Run gem tests");
    test_step.dependOn(&test_run.step);
}
