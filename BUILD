# Prefer generated BUILD files to be called BUILD over BUILD.bazel
# gazelle:build_file_name BUILD,BUILD.bazel
# gazelle:prefix github.com/luluz66/goexample
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

gazelle(name = "gazelle")

gazelle(
    name = "gazelle-update-repos",
    args = [
        "-from_file=go.mod",
        "-to_macro=deps.bzl%go_dependencies",
        "-prune",
    ],
    command = "update-repos",
)

go_library(
    name = "goexample_lib",
    srcs = ["main.go"],
    importpath = "github.com/luluz66/goexample",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_google_or_tools//ortools/sat/go/cpmodel",
		"@com_github_google_or_tools//ortools/sat:cp_model_go_proto",
    ],
    #visibility = ["//visibility:private"],
)

go_binary(
    name = "goexample",
    embed = [":goexample_lib"],
    visibility = ["//visibility:public"],
    #visibility = ["//visibility:public"],
)
