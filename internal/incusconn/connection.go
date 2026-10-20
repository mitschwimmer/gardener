// Package incusconn holds the official Incus connection boundary.
// It is invoked only for infrastructure commands.
package incusconn

import (
	"fmt"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/cliconfig"
)

// RemoteName is the fixed Incus CLI remote Garden will use.
// The MVP retains the scaffold's remote name. Verify the operator's configuration
// before the first live apply; Garden never falls back to the default remote.
const RemoteName = "garden"

// Connect reuses Incus CLI configuration and authentication without falling back
// to the CLI's default remote. Callers must explicitly select each project.
func Connect() (incus.InstanceServer, error) {
	config, err := cliconfig.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("load Incus CLI configuration: %w", err)
	}
	if _, ok := config.Remotes[RemoteName]; !ok {
		return nil, fmt.Errorf("Incus remote %q is not configured", RemoteName)
	}
	server, err := config.GetInstanceServer(RemoteName)
	if err != nil {
		return nil, fmt.Errorf("connect to Incus remote %q: %w", RemoteName, err)
	}
	return server, nil
}
