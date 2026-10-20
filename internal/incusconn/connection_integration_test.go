//go:build integration && linux

package incusconn_test

import (
	"crypto/rand"
	"slices"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/incustest"
)

func TestIntegrationProjectLifecycle(t *testing.T) {
	server := incustest.Connect(t)
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
