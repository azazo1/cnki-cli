// Command cnki 是中国知网的命令行工具.
package main

import (
	"os"

	"github.com/azazo1/cnki-cli/internal/cli"
)

// version 由发布构建通过 -ldflags 注入, 日常开发构建显示 dev-build.
var version = "dev-build"

func main() {
	os.Exit(cli.Execute(cli.Options{
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: version,
	}))
}
