// zion-node is the ZION core node program. Phase 1 exposes version metadata only.
package main

import (
	"fmt"
	"os"

	"github.com/kokora3/zion/internal/protocol"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintln(os.Stdout, protocol.VersionString("zion-node"))
		return
	}

	fmt.Fprintf(os.Stderr, "usage: zion-node --version\n")
	os.Exit(2)
}
