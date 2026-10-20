//go:build integration && linux

// Package incustest isolates opt-in tests from the operator's Incus settings.
package incustest

import (
	"os"
	"path/filepath"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/cliconfig"
	"github.com/mitschwimmer/gardener/internal/incusconn"
)

// Connect requires an explicitly supplied disposable daemon and configures only
// its Unix socket. The caller must ensure the daemon contains no valuable data.
func Connect(t *testing.T) incus.InstanceServer {
	t.Helper()
	socket := os.Getenv("GARDEN_TEST_INCUS_SOCKET")
	if !filepath.IsAbs(socket) {
		t.Fatal("GARDEN_TEST_INCUS_SOCKET must name an absolute socket path on a disposable daemon")
	}
	info, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("%s is not a Unix socket", socket)
	}
	configDir := t.TempDir()
	t.Setenv("INCUS_CONF", configDir)
	t.Setenv("INCUS_GLOBAL_CONF", t.TempDir())
	t.Setenv("INCUS_REMOTE", "unused")
	config := cliconfig.NewConfig(configDir, false)
	config.Remotes = map[string]cliconfig.Remote{
		incusconn.RemoteName: {Addrs: []string{"unix://" + socket}, Protocol: "incus"},
	}
	config.SetDefaultRemote("unused")
	if err := config.SaveConfig(filepath.Join(configDir, "config.yml")); err != nil {
		t.Fatal(err)
	}
	server, err := incusconn.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Disconnect)
	return server
}
