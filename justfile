[private]
default:
    @just --list

# 拉取依赖. 本机 proxy.golang.org 不可达, 因此统一走国内代理.
deps:
    go mod download

# 整理依赖.
tidy:
    go mod tidy

# 构建当前平台的二进制到 bin/cnki.
build:
    go build -o bin/cnki ./cmd/cnki

# 运行全部单元测试. 测试不访问网络.
test:
    go test ./... -count=1

# 静态检查.
vet:
    go vet ./...

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

