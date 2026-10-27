load("@rules_go//go:def.bzl", "go_binary")
load("@bazel_gowire//rules:gowire.bzl", "gowire")

gowire(
    name = "wire",
    srcs = [
        "wire.go"
    ],
    deps = [
        "//internal/adapter",
        "//internal/application",
        "//internal/infra",
        "@com_github_google_wire//wire",
    ],
)


go_binary(
    name = "app"
    srcs = [
        "main.go",
        ":wire"
    ]
)
