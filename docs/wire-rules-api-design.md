# rules_go_wire API 设计（草案）

| 状态 | 目标版本 | 最后更新 |
| --- | --- | --- |
| Draft，等待评审 | `v0.1.0` | 2026-10-21 |

本文定义 `rules_go_wire` 对用户公开的 Bazel API，以及这些 API 必须满足的行为。
本文是设计文档，不包含规则实现。评审通过后再开始编写 Starlark、Go 辅助程序和测试。

设计以 [`github.com/google/wire/cmd/wire`](https://github.com/google/wire/tree/master/cmd/wire)
的 `gen`、`check`、`diff`、`show` 命令语义为上游基准。官方仓库不发布 Wire
可执行文件，因此 Bazel 集成需要从 Go module 源码构建 `cmd/wire`。Bazel 集成必须
保留 Wire 的源码分析、类型检查和代码生成语义，同时将文件生成纳入 Bazel 的 action 图。

## 目标

- 为 Go 项目提供与 `go_library` 使用体验一致的 Wire 依赖注入代码生成入口。
- 在普通 `bazel build` 中自动生成代码，不要求提交或手工执行 `wire gen`。
- 保持 hermetic、可远程缓存、跨平台和可并行执行。
- 从项目锁定的官方 Wire 源码编译 `wire` 可执行程序，支持升级、诊断和错误定位。
- 提供高层宏以满足常见场景，同时保留低层规则用于高级组合和迁移。
- 公共 API 可按 Stardoc/Bazel 规则文档惯例生成参考文档。

## 非目标

- 不在 `v0.1.0` 中实现 Gazelle 自动生成 `BUILD.bazel` 的扩展。
- 不替代 `rules_go` 的 Go 编译、测试、链接和交叉编译能力。
- 不提供运行时依赖注入，也不改变 Wire 本身的 provider set 语义。
- 不执行 `go run github.com/google/wire/cmd/wire@latest`。
- 不依赖机器上的 `GOPATH`、用户模块缓存、`PATH` 中的 `go`/`wire` 或网络。
- 不在首个版本中承诺 cgo、复杂 vendor/importmap 场景；这些场景必须显式报错或另行评审。

## 公共 API 概览

| 符号 | 类型 | 稳定性 | 用途 |
| --- | --- | --- | --- |
| `wire_library` | Macro | 稳定，推荐使用 | 创建 Wire 输入库、生成器目标和最终 `go_library` |
| `wire_gen` | Rule | 稳定，高级 API | 从兼容 `GoInfo`/`GoArchive` 的库生成 Go 源文件 |
| `wire` extension | Module extension | 稳定 | 将已有的 Wire Go 包编译成工具并注册工具链 |
| `@rules_go_wire//wire:toolchain_type` | Toolchain type | 稳定 | `wire_gen` 解析执行平台上的 Wire 工具链 |

`v0.1.0` 不公开新的 result provider。`wire_gen` 的标准产物通过 `DefaultInfo`
提供；`wire_library` 的最终目标由 `go_library` 创建，因此继续提供 `GoInfo` 和
`GoArchive`。

## 安装与工具链

推荐使用 Bzlmod。Wire 源码由项目的 Go 依赖解析统一提供，不由
`rules_go_wire` 单独下载：

```starlark
# MODULE.bazel
bazel_dep(name = "rules_go", version = "<project-compatible-version>")
bazel_dep(name = "rules_go_wire", version = "0.1.0")

go_deps = use_extension("@gazelle//:extensions.bzl", "go_deps")
go_deps.from_file(go_mod = "//:go.mod")
use_repo(go_deps, "com_github_google_wire")

wire = use_extension("@rules_go_wire//wire:extensions.bzl", "wire")
wire.toolchain(wire = "@com_github_google_wire//cmd/wire")
use_repo(wire, "rules_go_wire_toolchain")

register_toolchains("@rules_go_wire_toolchain//:all")
```

`com_github_google_wire` 是 `go_deps` 生成的仓库名，实际名称以项目的 Go
dependency extension 为准。若项目尚未在 `go.mod` 中依赖 Wire，可显式声明：

```starlark
go_deps.module(
    module = "github.com/google/wire",
    version = "v0.6.0",
)
```

版本必须是精确的 Go module 版本，不接受 `latest` 或分支名。Bazel 在 module/repository
解析阶段下载 `github.com/google/wire` 的 Go module 源码、生成 BUILD 元数据并缓存到
Bazel repository cache。`wire_gen` 构建和执行阶段都不会再次下载 Wire 源码。

工具链属性草案：

| 名称 | 描述 | 类型 | 必填 | 默认值 |
| --- | --- | --- | --- | --- |
| `wire` | `github.com/google/wire/cmd/wire` 包的目标 label | Label | 是 | |
| `toolchain_name` | 注册到仓库中的工具链名称 | String | 否 | `wire` |

同一构建图中出现多个不兼容 Wire 版本时，通过标准 Bazel toolchain resolution
选择一个版本；不提供隐式的“最后注册者获胜”行为。

## Wire 源码仓库如何获取

Bazel 对外部 Go 源码的常规处理链路是：

```text
go.mod / go_deps 解析
    -> 下载 Go module 源码
    -> 生成外部仓库及其 BUILD 元数据
    -> rules_go 编译 @com_github_google_wire//cmd/wire
    -> wire_gen action 执行编译后的 wire 可执行文件
```

这里不应把源码下载放进 `wire_gen` build action。Bazel repository rule 或 module
extension 在 action 执行前完成下载，build action 只看到已经存在的声明输入。这与
`rules_go` 处理任意 Go module 的方式一致，也能使用 Bazel 的 repository cache 和
默认的构建缓存语义。

不优先采用原始 `http_archive`，原因如下：

- Wire 仓库目前没有根级或 `cmd/wire` 目录的 `BUILD.bazel`。
- `http_archive` 只解压源码，不会自动生成 Go package 的 BUILD 目标。
- `cmd/wire` 还依赖 `x/tools`、`subcommands` 和 `go-difflib` 等传递模块。
- 手工补齐 BUILD 和全部依赖仓库会把 Go module 解析逻辑重新实现一遍。

`go_deps`/`go_repository` 正是 Bazel Go 开发生态中处理这类“只有 Go 源码、没有
BUILD 文件”的仓库的标准入口。若未来需要让 `rules_go_wire` 完全独立于用户
`go_deps`，可以在后续版本增加专用的 pinned tool repository；这属于工具分发策略，
不改变 `wire_library` 或 `wire_gen` 的公共 API。

## 快速开始

`BUILD.bazel`：

```starlark
load("@rules_go//go:def.bzl", "go_binary")
load("@rules_go_wire//wire:defs.bzl", "wire_library")

wire_library(
    name = "app",
    srcs = [
        "app.go",
        "wire.go",
    ],
    deps = [
        "//internal/config",
        "//internal/server",
        "@com_github_google_wire//wire",
    ],
    importpath = "example.com/demo/app",
)

go_binary(
    name = "server",
    embed = [":app"],
)
```

`wire.go` 保持 Wire 官方约定：

```go
//go:build wireinject

package app

import (
    "context"

    "github.com/google/wire"
)

func InitializeServer(ctx context.Context) (*Server, error) {
    wire.Build(NewServer, newConfig)
    return nil, nil
}
```

执行 `bazel build //path/to:server` 时，依赖链自动触发 Wire 生成。用户不需要先运行
`wire gen`，也不需要把 `wire_gen.go` 加入源码或提交到仓库。

## 构建图与生命周期

`wire_library(name = "app", ...)` 展开为以下逻辑结构：

```text
app
├── app_wire_input
│   ├── srcs: 用户声明的原始源码
│   ├── deps: 用户声明的依赖
│   └── gotags: gotags + wireinject + wire_tags
├── app_wire_gen
│   ├── library: :app_wire_input
│   └── output: wire_gen.go
└── :app (go_library)
    ├── srcs: 原始源码 + :app_wire_gen
    ├── deps: 用户声明的依赖
    └── gotags: 用户声明的 gotags
```

生命周期如下：

1. `app_wire_input` 在 `wireinject` build tag 下形成完整的注入声明包。
2. `app_wire_gen` 收集该包及其传递依赖的源码信息，在沙箱中执行官方 `wire gen`。
3. 生成文件成为最终 `go_library` 的一个 `src`。
4. 最终 `go_library` 不启用 `wireinject` tag，因此不编译注入器桩函数。
5. `rules_go` 对生成结果执行正常 Go 编译和类型检查。

内部目标后缀 `<name>_wire_input` 和 `<name>_wire_gen` 为保留名称。宏创建目标时
若发现名称冲突，应在 analysis 阶段失败，并指出冲突目标。

## `wire_library`

```starlark
load("@rules_go_wire//wire:defs.bzl", "wire_library")

wire_library(
    name,
    srcs = [],
    deps = [],
    embed = [],
    gotags = [],
    importpath = "",
    header_file = None,
    wire_tags = [],
    visibility = None,
    testonly = False,
)
```

这是推荐入口。宏创建 Wire 输入库、生成目标和与宏同名的最终 `go_library`。
最终目标可以被 `go_binary.embed`、`go_test.embed` 或其它 Go 包依赖。

### 属性

| 名称 | 描述 | 类型 | 必填 | 默认值 |
| --- | --- | --- | --- | --- |
| `name` | 最终 `go_library` 的目标名 | Name | 是 | |
| `srcs` | Go 源文件列表，语义同 `go_library.srcs`。不得包含旧的 `wire_gen.go` | List of labels | 否 | `[]` |
| `deps` | 当前包直接导入的 Go 库，必须包含运行时所需的 `github.com/google/wire` 库 | List of labels | 否 | `[]` |
| `embed` | 与当前包合并编译的 Go 库，语义同 `go_library.embed` | List of labels | 否 | `[]` |
| `gotags` | 最终包的 Go build tags；输入包会自动追加 `wireinject` 和 `wire_tags` | List of strings | 否 | `[]` |
| `importpath` | 最终包的 import path；为空时沿用 `rules_go` 推断行为 | String | 否 | `""` |
| `header_file` | 插入到生成文件头部的单个文件，对应 `wire gen -header_file` | Label | 否 | `None` |
| `wire_tags` | 仅用于 Wire 加载包时追加的 build tags，对应 `wire gen -tags` | List of strings | 否 | `[]` |
| `visibility` | 最终目标可见性；内部目标固定为 private | List of labels | 否 | `None` |
| `testonly` | 是否仅用于测试目标 | Boolean | 否 | `False` |

`wire_library` 在 `v0.1.0` 明确支持纯 Go 包。需要 cgo、特殊 `importmap` 或
非标准 Go provider 的目标使用低层 `wire_gen`，并需等待对应兼容性验证通过。

### 行为约束

- `wire_tags` 不会传给最终 `go_library`，除非用户也把它们写入 `gotags`。
- 重复 tag 必须去重，顺序保持稳定；`wireinject` 不能依赖用户手工声明。
- `header_file` 变化必须使生成 action 失效并重新执行。
- `srcs + [":<name>_wire_gen"]` 中的生成文件只由 `wire_gen` 产生，不由宏写回源码目录。
- `wire_library` 返回与普通 `go_library` 兼容的 provider，调用方无需特殊处理。
- 一个 Bazel package 中默认只应有一个 `wire_library`，因为默认生成文件名为
  `wire_gen.go`。多注入包场景需要使用低层规则并指定不同的 `out`。
- 旧工作流中已提交的 `wire_gen.go` 必须从 `srcs` 移除，否则会造成重复声明和绕过生成 action。

## `wire_gen`

```starlark
load("@rules_go_wire//wire:defs.bzl", "wire_gen")

wire_gen(
    name,
    library,
    out = "wire_gen.go",
    header_file = None,
    wire_tags = [],
    visibility = None,
    testonly = False,
)
```

`wire_gen` 是低层规则，接收一个已完成 build-constraint 选择、启用
`wireinject` 的 Go 包，并生成一个 `.go` 文件。典型用法：

```starlark
load("@rules_go//go:def.bzl", "go_library")
load("@rules_go_wire//wire:defs.bzl", "wire_gen")

go_library(
    name = "app_wire_input",
    srcs = [
        "app.go",
        "wire.go",
    ],
    deps = [
        "//internal/config",
        "//internal/server",
        "@com_github_google_wire//wire",
    ],
    gotags = ["wireinject"],
    importpath = "example.com/demo/app",
)

wire_gen(
    name = "app_wire_gen",
    library = ":app_wire_input",
)

go_library(
    name = "app",
    srcs = [
        "app.go",
        ":app_wire_gen",
    ],
    deps = [
        "//internal/config",
        "//internal/server",
        "@com_github_google_wire//wire",
    ],
    importpath = "example.com/demo/app",
)
```

### 属性

| 名称 | 描述 | 类型 | 必填 | 默认值 |
| --- | --- | --- | --- | --- |
| `name` | 唯一目标名 | Name | 是 | |
| `library` | Wire 输入包，必须提供兼容 `rules_go` 的 `GoInfo` 和 `GoArchive` | Label | 是 | |
| `out` | 生成文件名；必须是当前 Bazel package 相对路径，并以 `.go` 结尾 | String | 否 | `"wire_gen.go"` |
| `header_file` | 插入到生成文件头部的单个文件 | Label | 否 | `None` |
| `wire_tags` | 追加给 `wire gen -tags` 的 build tags | List of strings | 否 | `[]` |
| `visibility` | 生成文件目标的可见性 | List of labels | 否 | `None` |
| `testonly` | 是否仅用于测试目标 | Boolean | 否 | `False` |

### 产物

`wire_gen` 在 `DefaultInfo.files` 中只暴露 `out` 对应的单个文件。其它文件属于 action
临时目录，不进入缓存产物。生成内容必须与同版本、同输入、同 tags、同 header 的官方
`wire gen` 输出一致。

## CLI 语义映射

`wire_gen` 使用官方 CLI，不重新实现 provider graph 或生成算法。

| Bazel API | 官方 CLI | 说明 |
| --- | --- | --- |
| `wire_library` / `wire_gen` | `wire gen <package>` | Bazel 中由 action 自动触发 |
| `header_file` | `wire gen -header_file <file>` | 读取并嵌入生成文件头部 |
| `wire_tags` | `wire gen -tags <tag-list>` | 追加 Wire 包加载 tags |
| 输入库的 `gotags` | Bazel Go build constraints | 确保 `wireinject` 源码参与输入包 |
| `out` | 生成后复制/重命名 | 上游并不直接接收任意 Bazel 输出路径 |

`check`、`diff`、`show` 在 `v0.1.0` 不单独暴露 Bazel 规则。`wire_gen` 已包含
Wire 的加载和类型检查，生成失败必须转换为 Bazel action 失败。后续可评审独立的
`wire_check` test 规则，用于迁移 CLI 工作流或生成诊断报告。

## 生成执行模型

实现必须把官方 CLI 运行在一个 Bazel 能理解的沙箱中，不能在被修改的工作区里直接
运行 `wire gen`。建议执行流程如下：

1. 从 `library` 的 `GoInfo` 获取当前包源码、import path 和直接依赖。
2. 从 `GoArchive.transitive` 遍历传递依赖，收集每个包的源码和 import path。
3. 在当前 action 的沙箱内物化一个临时 Go module/GOPATH 视图。
4. 将 `wireinject`、用户 tags 和平台约束传给 `go/packages` 加载过程。
5. 从 Wire toolchain 获取由 `go_deps` 提供源码、由 `rules_go` 编译的 `wire`，
   以及执行平台 Go SDK。
6. 以 `GOPROXY=off`、`GOTOOLCHAIN=local` 和受控的 `GOROOT`/`GOCACHE`/`GOPATH`
   环境执行 `wire gen`。
7. 从临时目录读取生成结果，写入声明的 `out`，不修改输入源码。
8. 清理临时目录，并由 Bazel 对 action 的全部声明输入和输出计算缓存键。

设计约束：

- 所有输入必须通过 depset/inputs 声明；不得扫描 `bazel-*` 或整个 workspace。
- 不得读取用户模块缓存或通过 `go env` 读取宿主机配置。
- 不得在 action 中拉取模块；缺失依赖必须是明确的 analysis/generation 错误。
- action 的环境变量必须是显式白名单，避免 `HOME`、`GOPATH`、`GOFLAGS` 等造成缓存污染。
- 生成过程必须确定、原子化；失败时不得留下可被后续目标误用的部分输出。
- action mnemonic 使用 `GoWireGen`，progress message 应包含输入包和生成目标。
- 远程缓存键必须覆盖源码、传递依赖、build tags、header、Wire 版本和 Go SDK 版本。

临时 Go module 的路径映射是主要实现风险。若某个依赖无法以原始 import path 和源码
建图，规则必须报“unsupported package layout”，不能回退到宿主机 Go 环境。

## 诊断与错误

错误信息必须尽可能保留 Wire 的原始文件名、行号和列号，同时补充 Bazel 上下文。

| 场景 | 行为 |
| --- | --- |
| library 不提供 `GoInfo`/`GoArchive` | analysis 失败，指出 `library` label 与缺失 provider |
| 无 `wireinject` 输入源码 | 生成失败，提示检查 build tag 和输入库的 `gotags` |
| 找不到 injector 函数 | 生成失败，指出输入包；不生成空文件 |
| provider set 无法满足 | 保留 Wire 的 `wire:` 诊断，并补充输入包和规则 label |
| 找不到传递依赖源码 | 失败并指出 import path；禁止联网自动补齐 |
| `out` 为绝对路径或包含路径穿越 | analysis 失败 |
| header 不存在或不是单个文件 | analysis 失败 |
| Wire toolchain 未注册 | analysis 失败，给出 toolchain type 和注册示例 |
| 生成 Go 文件有编译错误 | 由最终 `go_library` 编译 action 报告 |

日志中不得输出 action 沙箱的真实绝对路径作为稳定诊断的一部分；应优先使用
workspace 相对路径或 `GoInfo` 中的逻辑路径。

## 文档与 Stardoc

实现阶段采用以下文档布局，保持 Bazel rules 项目惯例：

```text
README.md                       # 安装、快速开始、规则摘要
docs/rules.md                   # 由 Stardoc 生成的规则/属性参考
docs/providers.md               # 仅在公开 provider 时维护
docs/wire-rules-api-design.md   # 本设计及决策记录
```

`docs/rules.md` 应带有 Stardoc 生成头和 `load()` 路径，每个公开规则包含：

- 规则签名；
- 一句话用途和完整行为说明；
- 返回/提供的 provider；
- 按 Bazel 文档格式排列的属性表：名称、描述、类型、是否必填、默认值。

宏无法只依赖 Stardoc 获得完整语义，因此 README 必须同时说明 `wire_library`
展开的目标、`wireinject` 要求和旧 `wire_gen.go` 的迁移方式。

## 兼容性

| 项目 | `v0.1.0` 设计 |
| --- | --- |
| Bazel | Bzlmod 优先，支持当前项目使用的 Bazel 7/8 系列 |
| rules_go | 与项目现有 rules_go 版本兼容的 `GoInfo`/`GoArchive` API |
| Wire | 版本由项目 Go module 依赖锁定；工具链编译同一份源码 |
| 平台 | Linux、macOS、Windows 的 execute platform |
| Go 版本 | 使用执行平台 Go SDK；生成输入必须能由该 SDK 类型检查 |
| Build tags | 支持，至少保证 `wireinject` 和 `wire_tags` |
| cgo | `v0.1.0` 不作稳定承诺；不支持时显式失败 |
| WORKSPACE | 延后支持，不作为首个版本验收条件 |

API 稳定性规则：

- `wire_library` 和 `wire_gen` 的属性名、默认值和错误类别在 `v0.x` 内尽量稳定。
- 新增属性必须保持默认行为不变。
- 删除或重命名属性需要迁移说明，并在至少一个 minor 版本内兼容旧名称。
- 默认生成文件名和 `wireinject` 自动注入属于稳定行为。
- 内部目标名、沙箱布局和 Go 辅助程序协议不属于公共 API。

## 备选方案

| 方案 | 结论 | 原因 |
| --- | --- | --- |
| 用 `genrule` 直接执行 `wire gen` | 不采用 | action 依赖用户 checkout 的 module/GOPATH，不 hermetic |
| 直接用 `http_archive` 下载 Wire 源码 | 不作为首选 | 上游无 BUILD 文件，还需手工处理全部 Go 传递依赖 |
| 通过 `go_deps`/`go_repository` 提供源码 | 采用 | 复用 Bazel Go 生态现有的仓库下载和 BUILD 生成机制 |
| 要求用户提交 `wire_gen.go` | 不采用为主流程 | 容易过期，无法保证源码与生成结果一致 |
| 在 Bazel 中重写 Wire 生成器 | 不采用 | 与官方算法漂移，维护成本高 |
| `go run ...@latest` | 不采用 | 网络、版本和可复现性不可控 |
| 官方 CLI + 沙箱源码视图 | 采用 | 最大程度复用 Wire 语义，同时满足 Bazel action 模型 |
| 高层宏 + 低层生成规则 | 采用 | 常用场景简单，高级用户仍可组合 |

## 验收标准

进入实现后，至少需要以下文档级行为全部落地：

1. 纯 Go 示例在 Linux、macOS、Windows 上可由 `bazel build` 一次完成生成和编译。
2. 修改 provider、wire tags 或 header 会触发重新生成；无关文件不会触发。
3. 两个独立 Bazel package 的 Wire 生成可以并行执行并命中远程缓存。
4. 未使用 `wireinject`、缺失 provider、无 injector 和无效 header 均有可定位错误。
5. `wire_library` 的最终目标可直接被 `go_binary.embed` 使用。
6. CI 分别验证文件未提交时的 clean build 和第二次 build 的 cache hit。
7. 生成过程中不访问网络，不读取或写入用户模块缓存，不修改源码树。

## 待评审决策

建议本轮优先确认以下 API 决策：

1. 是否接受 `wire_library` 作为推荐入口，`wire_gen` 作为低层规则。
2. 是否接受自动向输入库注入 `wireinject` tag，最终库默认不注入。
3. 是否接受 `v0.1.0` 仅承诺纯 Go 包，cgo/vendor 复杂布局延后。
4. 是否接受 `header_file`、`wire_tags` 和 `out` 作为首版公共属性。
5. 是否接受通过 `go_deps` 获取 Wire 源码，`rules_go_wire` 不单独维护源码下载。
6. 是否接受 Bzlmod 优先、WORKSPACE 延后。

