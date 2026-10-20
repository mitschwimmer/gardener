//go:build integration && linux

package incusconn

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/lxc/incus/v7/shared/cliconfig"
)

func TestIntegrationProjectLifecycle(t *testing.T) {
	// Never load the operator's remotes or credentials. The caller must explicitly
	// supply a Unix socket belonging to a disposable test daemon.
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
		RemoteName: {Addr: "unix://" + socket, Protocol: "incus"},
	}
	config.SetDefaultRemote("unused")
	if err := config.SaveConfig(filepath.Join(configDir, "config.yml")); err != nil {
		t.Fatal(err)
	}

	server, err := Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Disconnect)
	status, _, err := server.GetServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Incus %s", status.Environment.ServerVersion)

	name := "garden-ci-" + strings.ToLower(rand.Text())
	want := api.ProjectPut{
		Description: "Garden integration test",
		Config: map[string]string{
			"features.images":          "false",
			"features.profiles":        "false",
			"features.storage.volumes": "false",
			"user.garden.integration":  "created",
		},
	}
	if err := server.CreateProject(api.ProjectsPost{Name: name, ProjectPut: want}); err != nil {
		t.Fatal(err)
	}
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			if err := server.DeleteProject(name); err != nil {
				t.Errorf("clean up project %s: %v", name, err)
			}
		}
	})

	project, etag, err := server.GetProject(name)
	if err != nil {
		t.Fatal(err)
	}
	if project.Description != want.Description || project.Config["user.garden.integration"] != "created" {
		t.Fatalf("created project differs: %+v", project)
	}
	want.Description = "Garden integration test updated"
	want.Config["user.garden.integration"] = "updated"
	if err := server.UpdateProject(name, want, etag); err != nil {
		t.Fatal(err)
	}
	project, _, err = server.GetProject(name)
	if err != nil {
		t.Fatal(err)
	}
	if project.Description != want.Description || project.Config["user.garden.integration"] != "updated" {
		t.Fatalf("updated project differs: %+v", project)
	}
	if err := server.DeleteProject(name); err != nil {
		t.Fatal(err)
	}
	deleted = true
	names, err := server.GetProjectNames()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(names, name) {
		t.Fatalf("deleted project %s still exists", name)
	}
}
