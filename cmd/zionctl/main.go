// zionctl is the local ZION node control client. Phase 1 exposes version metadata only.
package main

import (
	"fmt"
	"os"

	"github.com/kokora3/zion/internal/protocol"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Fprintln(os.Stdout, protocol.VersionString("zionctl"))
		return
	}

	fmt.Fprintf(os.Stderr, "usage: zionctl --version\n")
	os.Exit(2)
}
