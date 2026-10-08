// Command cnki 是中国知网的命令行工具.
package main

import (
	"os"

	"github.com/azazo1/cnki-cli/internal/buildinfo"
	"github.com/azazo1/cnki-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.Options{
		Args:    os.Args[1:],
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Version: buildinfo.Version(),
	}))
}
