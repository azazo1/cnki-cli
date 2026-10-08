[private]
default:
    @just --list

# 拉取依赖.
deps:
    go mod download

# 整理依赖.
tidy:
    go mod tidy

# 构建当前平台的二进制到 bin/cnki, 版本号显示 dev-build.
build:
    go build -o bin/cnki ./cmd/cnki

# 平台判断与版本解析都在 scripts/dist.py 内部完成, 三个平台共用同一条命令.
# 生成当前平台的发布产物.
dist:
    uv run --no-project python scripts/dist.py

# 当前构建应当显示的版本号.
version:
    @uv run --no-project python scripts/build-version.py

# 运行全部单元测试. 测试不访问网络.
test:
    go test ./... -count=1

# 静态检查.
vet:
    go vet ./...

# 检查格式是否符合 gofmt, 只报告不修改.
fmt-check:
    uv run --no-project python scripts/fmt_check.py

# 直接运行 CLI.
# just run search 深度学习 --limit 10
run *args:
    go run ./cmd/cnki {{args}}

# 打开受控浏览器完成一次知网验证并保存会话.
login:
    go run ./cmd/cnki auth login

# 查看当前会话状态.
status:
    go run ./cmd/cnki auth status

# 打印配置文件位置与当前取值.
config-show:
    go run ./cmd/cnki config show
