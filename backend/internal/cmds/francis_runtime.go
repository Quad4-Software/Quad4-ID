package cmds

import (
	"fmt"
	"os"
)

// printRemoteActorDataNotice warns that the command does not cover the actor data, which a standalone Francis runtime owns rather than Quad4 ID
// It goes to stderr so it stays visible when the archive itself is streamed to stdout
func printRemoteActorDataNotice(consequence string, remedy string) {
	fmt.Fprintf(os.Stderr, `WARNING: FRANCIS_HOST points to a standalone Francis runtime.
  %s.
  To cover all data, %s.

`, consequence, remedy)
}
