#!/usr/bin/env python3

# 生成当前平台的发布产物 (Go).
# 需要 scripts/archive.py, scripts/build-version.py 与 scripts/release_common.py 同时存在.
#
# 平台判断与版本解析都在脚本内部完成, 因此三个平台共用同一条命令:
#   uv run --no-project python scripts/dist.py

import os
import shutil

import release_common as rc

# Go 没有包版本可读, 仓库还没有版本 tag 时用占位版本兜底.
PROJECT_NAME = "cnki"
MAIN_PACKAGE = "./cmd/cnki"
BINARY_NAME = "cnki"
PACKAGE_VERSION = "0.0.0"
# 冒烟检查用的参数, 要求二进制能报出版本号.
SMOKE_ARGS = ["--version"]
# 除二进制外要一并放进归档的文件, 不存在的会被跳过.
EXTRA_ARCHIVE_FILES = ["README.md", "LICENSE"]

# go env GOOS / GOARCH 到产物命名 token 的映射.
PLATFORM_BY_GOOS = {"darwin": "macos", "linux": "linux", "windows": "windows"}
ARCH_BY_GOARCH = {"amd64": "x86_64", "arm64": "aarch64"}


def go_target():
    """取本次构建的目标平台与架构.

    与 go build 实际使用的目标保持一致, 用 go env GOOS / GOARCH 判断 (因此交叉编译时
    产物名跟着变), 不要另设 TARGET_PLATFORM / TARGET_ARCH, 否则产物名会与实际格式不符.
    """
    goos = rc.try_run(["go", "env", "GOOS"]) or ""
    goarch = rc.try_run(["go", "env", "GOARCH"]) or ""
    target_platform = PLATFORM_BY_GOOS.get(goos)
    target_arch = ARCH_BY_GOARCH.get(goarch)
    if not target_platform or not target_arch:
        rc.die(f"无法识别目标平台 {goos or '?'}-{goarch or '?'}, 请检查 go env GOOS / GOARCH")
    return target_platform, target_arch


def main():
    rc.configure_output_encoding()
    rc.repo_root()

    target_platform, target_arch = go_target()
    display_version, archive_version = rc.resolve_versions(
        rc.script_path("build-version.py"), PACKAGE_VERSION
    )
    print(f"构建 {PROJECT_NAME} {display_version} ({target_platform}-{target_arch})")

    binary = BINARY_NAME + (".exe" if target_platform == "windows" else "")
    staging = rc.stage_dir()
    binary_path = os.path.join(staging, binary)

    # 注入路径必须是完整包路径, 所以这里用 go list -m 取当前模块名.
    module = rc.run(["go", "list", "-m"])
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    rc.run(
        [
            "go",
            "build",
            "-trimpath",
            "-ldflags",
            f"-s -w -X {module}/internal/buildinfo.version={display_version}",
            "-o",
            binary_path,
            MAIN_PACKAGE,
        ],
        env=env,
        capture=False,
    )

    rc.smoke_check([binary_path], SMOKE_ARGS, display_version)

    # 归档里带上许可证与用法说明: MIT 要求随副本附带版权声明, 而解压后能
    # 直接看到说明对使用者也更方便. 源码与 go.mod 一律不打进归档.
    extras = []
    for name in EXTRA_ARCHIVE_FILES:
        if os.path.isfile(name):
            shutil.copy2(name, os.path.join(staging, name))
            extras.append(name)

    rc.archive(
        staging,
        [binary, *extras],
        PROJECT_NAME,
        archive_version,
        target=(target_platform, target_arch),
    )


if __name__ == "__main__":
    main()
