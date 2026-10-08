#!/usr/bin/env python3

# 检查 Go 源码的 gofmt 格式, 只报告不修改.
#
# 本地经 just fmt-check 调用, CI 的 test job 也调用同一份实现, 避免两边各写一遍.
# 不修改工作区: CI 里改写文件会让后续步骤看到与提交不符的内容.

import os
import subprocess
import sys

TARGETS = ["./cmd", "./internal"]


def main():
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8")
        except (AttributeError, ValueError, OSError):
            pass

    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    os.chdir(root)

    result = subprocess.run(
        ["gofmt", "-l", *TARGETS],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
        errors="replace",
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        print(f"gofmt 执行失败 (退出码 {result.returncode})", file=sys.stderr)
        return 1

    unformatted = [line for line in result.stdout.splitlines() if line.strip()]
    if unformatted:
        print("以下文件未格式化:", file=sys.stderr)
        for path in unformatted:
            print(f"  {path}", file=sys.stderr)
        return 1

    print("格式检查通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
