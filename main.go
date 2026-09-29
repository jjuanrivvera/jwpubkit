// Command jwlib reads JW publications (JWPUB) from a local library.
package main

import (
	"os"

	"github.com/jjuanrivvera/jwpubkit/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
