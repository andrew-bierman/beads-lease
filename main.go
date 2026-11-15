// beadle — the fleet's keeper of leases. Resource claims over the Beads
// coordination hub, identical on macOS and Linux.
package main

import (
	"os"

	"github.com/andrew-bierman/beadle/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
