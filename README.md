# cnki-cli

中国知网命令行工具. 覆盖知网的检索体系: 一框式, 高级, 专业, 句子, 作者发文
检索, 学科分组聚合, 文献详情, 题录导出.

## 为什么需要一个登录步骤

知网对检索站点 (`kns.cnki.net`) 实施滑块验证, 且验证结果绑定在 cookie 上
而不是 IP 上 —— 实测同一个 IP 下, 浏览器过了验证, 纯 HTTP 请求依然会被
拦回验证页. 因此本工具需要一个方式拿到那份会话.

做法与 `npm login` 类似:

```shell
cnki auth login
```

该命令打开一个受控浏览器窗口停在知网页面上. 在窗口里完成滑块验证, 需要
登录时也在窗口里登录 (机构 IP 自动登录, 账号密码, 短信验证码, 微信扫码,
CARSI 联邦认证都行), 本工具随即读取会话并关闭窗口.

之后所有检索都走纯 HTTP 复用该会话, 速度快且不再需要浏览器. 会话失效时
会自动重新拉起验证窗口.

会话文件位于 `~/.config/cnki-cli/session.json`, 权限 `0600`.

## 安装

```shell
just build
```

产物在 `bin/cnki`.

## 使用

```shell
# 一框式检索, 默认按主题
cnki search 深度学习

# 指定字段与排序, 限制条数
cnki search 深度学习 --field 篇名 --sort cited --limit 50

# 限定文献库
cnki search 深度学习 --db 学位论文

# 高级检索, 多条件组合
cnki adv --cond '主题=深度学习' --cond '作者单位=清华大学'
cnki adv --cond '篇名=大语言模型' --logic OR --cond '篇名=生成式'

# 专业检索, 直接写知网检索式
cnki expr "SU=('深度学习'+'神经网络') AND AU=('张三')"

# 句子检索, 找同时包含两个词的原句
cnki sentence 大语言模型 医学影像
cnki sentence 大语言模型 医学影像 --paragraph

# 作者发文
cnki author 张三 --org 清华大学

# 分组聚合
cnki group 大语言模型 --group year --group discipline

# 文献详情
cnki detail '<详情页地址或 v 参数>'

# 题录导出
cnki export 深度学习 --export-format bibtex --limit 20 --out refs.bib
cnki export 深度学习 --export-format ris --limit 50

# 元信息
cnki info dbs
cnki info fields
cnki info formats
```

输出默认是便于阅读的表格, 加 `--json` 得到结构化数据. 日志只写 stderr,
因此 `--json` 的输出可以直接交给 `jq`:

```shell
cnki search 深度学习 --limit 20 --json | jq -r '.articles[].title'
cnki search 深度学习 --limit 5 --json | jq -r '.articles[0].detail_url' | xargs cnki detail
```

## 不方便打开浏览器的环境

远程主机或 CI 上可以用三种方式注入会话:

```shell
cnki auth login --cookie-string 'SID_kns_new=xxx; Ecp_ClientId=yyy'
cnki auth login --cookie-file ~/cnki.cookie
cnki auth login --from-curl ~/request.txt
```

`--from-curl` 读取浏览器开发者工具里 "Copy as cURL" 的内容, 自动提取其中的
Cookie 请求头. 也可以用环境变量 `CNKI_COOKIE` 临时注入, 优先级最高.

## 接管已经开着的浏览器

如果日常浏览器里已经登录过知网, 不想再开一个新窗口重新登录, 可以让本工具
直接接管它:

```shell
# 先带调试端口启动 Chrome
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222

# 再接管它取会话
cnki auth login --cdp 9222
```

`--cdp` 接受纯端口号, `主机:端口` 或完整地址三种写法. 接管模式下本工具
不会关闭该浏览器, 因为它属于你, 贸然关掉会打断你手头的工作.

## 网络环境与权限

知网的权限分两层, 需要分开看待:

- **检索不需要机构 IP.** 实测在校园网外 (家庭宽带) 使用, 检索, 分组, 详情,
  作者与句子检索全部正常. 会话 cookie 一旦取得即可持续使用.
- **知网侧的题录导出需要个人账号登录.** 机构 IP 态下请求导出接口会被重定向
  到登录页. 此时改用本地生成的 `ris` 或 `bibtex` 格式即可, 或用
  `cnki auth login` 在浏览器窗口内登录个人账号.
- **全文下载受订阅约束.** 机构订阅范围外的文献, 知网本身就不会放行.

因此在校园网外工作时, 建议在浏览器窗口里登录个人账号后再取会话, 这样导出
能力也一并具备.

## 受限环境

macOS 上 Chrome 无法在受限沙箱内运行 (会直接 `Abort trap`), 表现为
`cnki auth login` 报 "启动浏览器失败". 这不是本工具的缺陷, 而是 Chrome
自身的运行要求. 遇到时按优先级处理:

1. 在宿主环境直接运行本命令, 不要在容器或受限沙箱里运行.
2. 用 `--cdp` 接管宿主上已经开着的浏览器.
3. 用 `--cookie-string` / `--cookie-file` / `--from-curl` 直接注入会话.

## 配置

配置文件位于 `~/.config/cnki-cli/config.toml`, 首次运行自动生成.
可参考 [config.toml.example](config.toml.example).

常改的两项:

- `request.interval_ms`: 相邻请求的最小间隔. 默认 1500 毫秒. 调小可以更快,
  但被知网弹验证的概率上升.
- `session.on_expired`: 会话失效时的行为, `relogin` / `fail` / `prompt`.
  脚本环境建议用 `fail`, 避免自动化流程意外卡在等人工验证上.

## 依赖代理

本机 `proxy.golang.org` 可能不可达, 请统一使用国内代理:

```shell
GOPROXY=https://goproxy.cn,direct go mod download
```

`just deps` 与 `just tidy` 已经内置该设置.

## 开发

```shell
just test        # 单元测试, 不访问网络
just vet         # 静态检查
just fmt-check   # 检查格式
just run search 深度学习 --limit 10
```

## 已知边界

- 知网只开放检索结果的前 300 页.
- 每页最多 50 条.
- 题录导出中 `ris` 与 `bibtex` 由本地从检索结果渲染, 其余格式走知网导出
  服务, 部分格式要求登录个人账号.
- 全文下载受订阅权限约束, 本工具只负责发起请求与落盘, 不绕过权限.
- 知网目前不校验检索接口的请求签名. 若日后开始强制校验, 需要在
  `internal/cnki` 中补上等价的签名实现.

## 结构与维护

```
cmd/cnki/            程序入口
internal/cli/        cobra 命令树
internal/cnki/       知网接口调用与 HTML 解析
internal/session/    会话存取与浏览器接管
internal/taxonomy/   库, 字段, 分组等枚举常量
internal/model/      领域模型
internal/output/     表格, JSON, CSV, RIS, BibTeX 渲染
internal/config/     配置读写与版本迁移
internal/logging/    slog 日志设施
```

知网改版时的改动集中在这几处:

- HTML 结构变化: `internal/cnki/parse.go` 里的选择器常量.
- 接口参数变化: `internal/cnki/query.go`.
- 新增库或字段: `internal/taxonomy/`.

`internal/taxonomy` 中的枚举取值全部来自对知网页面与前端脚本的实测提取,
不是文档猜测, 因此可以直接与浏览器抓到的请求逐字比对.
