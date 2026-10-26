load("@rules_go//go/private:context.bzl", "go_context")

_WIREGEN_TOOL = Label("@com_github_google_wire//cmd")

def _gowire_source_impl(ctx):
    go = go_context(ctx, include_deprecated_properties = False)
