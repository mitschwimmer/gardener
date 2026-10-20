//go:build integration && linux

package cli

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/incustest"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

type confirmationHook struct {
	io.Reader
	before func()
}

func (r *confirmationHook) Read(p []byte) (int, error) {
	if r.before != nil {
		r.before()
		r.before = nil
	}
	return r.Reader.Read(p)
}

func TestIntegrationAdoptDeletePreservesCustomVolume(t *testing.T) {
	server := incustest.Connect(t)
	volumeName := "garden-ci-vol-" + strings.ToLower(rand.Text())
	// Register volume cleanup before project cleanup so failure cleanup removes
	// its attachment before deleting the volume itself.
	t.Cleanup(func() {
		_, _, err := server.GetStoragePoolVolume("default", "custom", volumeName)
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return
		}
		if err != nil {
			t.Error(err)
			return
		}
		if err := server.DeleteStoragePoolVolume("default", "custom", volumeName); err != nil {
			t.Error(err)
		}
	})
	name := liveProjectName(t, server)
	g := definition.Garden{
		Projects: []definition.Project{{Name: name, Description: definition.Value("declared project")}},
		VMs: []definition.VM{{Project: name, Name: "test-vm", Image: os.Getenv("GARDEN_TEST_INCUS_IMAGE"),
			Pool: "default", Network: "incusbr0", Power: definition.Stopped, Description: definition.Value("declared VM")}},
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 2 operations")
	vm := server.UseProject(name)
	if err := server.CreateStoragePoolVolume("default", api.StorageVolumesPost{Name: volumeName, Type: "custom", ContentType: "filesystem",
		StorageVolumePut: api.StorageVolumePut{Description: "persistent data", Config: api.ConfigMap{"user.external": "preserve"}}}); err != nil {
		t.Fatal(err)
	}
	instance, etag, err := vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	put := instance.Writable()
	delete(put.Config, definition.OwnerKey)
	put.Config["user.external"] = "preserve"
	put.Description = "external VM description"
	put.Devices["data"] = map[string]string{"type": "disk", "pool": "default", "source": volumeName, "path": "/data"}
	op, err := vm.UpdateInstance("test-vm", put, etag)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Wait(); err != nil {
		t.Fatal(err)
	}
	project, etag, err := server.GetProject(name)
	if err != nil {
		t.Fatal(err)
	}
	projectPut := project.Writable()
	delete(projectPut.Config, definition.OwnerKey)
	projectPut.Description = "external project description"
	if err := server.UpdateProject(name, projectPut, etag); err != nil {
		t.Fatal(err)
	}
	beforeAdoption, _, err := vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	command := func(definitions definition.Garden, args []string, input io.Reader) (string, error) {
		t.Helper()
		var output bytes.Buffer
		err := run(args, input, &output, io.Discard, definitions, func() (reconcile.Backend, error) {
			return incusconn.Backend{Server: server}, nil
		})
		return output.String(), err
	}
	adoptVM := []string{"adopt", "vm", name, "test-vm"}
	if _, err := command(g, adoptVM, strings.NewReader("")); err == nil {
		t.Fatal("adopted VM before parent ownership")
	}
	for _, args := range [][]string{{"adopt", "project", name}, adoptVM} {
		if output, err := command(g, args, strings.NewReader("")); err != nil || !strings.Contains(output, "Apply complete: 1 operations") {
			t.Fatalf("adopt: %v %s", err, output)
		}
		if output, err := command(g, args, strings.NewReader("")); err != nil || !strings.Contains(output, "Apply complete: 0 operations") {
			t.Fatalf("repeat adopt: %v %s", err, output)
		}
	}
	project, _, err = server.GetProject(name)
	if err != nil || project.Description != "external project description" || project.Config[definition.OwnerKey] != definition.Owner {
		t.Fatalf("project adoption changed fields: %v %+v", err, project)
	}
	after, _, err := vm.GetInstance("test-vm")
	expected := beforeAdoption.Writable()
	expected.Config[definition.OwnerKey] = definition.Owner
	if err != nil || !reflect.DeepEqual(expected, after.Writable()) {
		t.Fatalf("VM adoption changed fields: %v %+v", err, after)
	}
	deleteVM := []string{"delete", "vm", name, "test-vm"}
	confirmation := name + "/test-vm\n"
	if _, err := command(g, deleteVM, strings.NewReader(confirmation)); err == nil {
		t.Fatal("deleted a declared VM")
	}
	empty := definition.Garden{}
	if _, err := command(empty, []string{"delete", "project", name}, strings.NewReader(name+"\n")); err == nil {
		t.Fatal("deleted a nonempty project")
	}
	for _, input := range []string{"", "yes\n", name + "\n"} {
		if _, err := command(empty, deleteVM, strings.NewReader(input)); err == nil {
			t.Fatal("deleted without exact VM confirmation")
		}
	}
	if _, _, err := vm.GetInstance("test-vm"); err != nil {
		t.Fatal("declined deletion removed VM", err)
	}
	hook := &confirmationHook{Reader: strings.NewReader(confirmation), before: func() {
		actual, etag, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		put := actual.Writable()
		put.Description = "changed during confirmation"
		op, err := vm.UpdateInstance("test-vm", put, etag)
		if err != nil {
			t.Fatal(err)
		}
		if err := op.Wait(); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := command(empty, deleteVM, hook); err == nil || !strings.Contains(err.Error(), "changed since discovery") {
		t.Fatalf("deleted after concurrent edit: %v", err)
	}
	if _, _, err := vm.GetInstance("test-vm"); err != nil {
		t.Fatal("stale confirmation removed VM", err)
	}
	if output, err := command(empty, deleteVM, strings.NewReader(confirmation)); err != nil || !strings.Contains(output, "Apply complete: 1 operations") {
		t.Fatalf("confirmed deletion: %v %s", err, output)
	}
	if _, _, err := vm.GetInstance("test-vm"); !api.StatusErrorCheck(err, http.StatusNotFound) {
		t.Fatalf("VM remains after deletion: %v", err)
	}
	if output, err := command(empty, []string{"delete", "project", name}, strings.NewReader(name+"\n")); err != nil || !strings.Contains(output, "Apply complete: 1 operations") {
		t.Fatalf("empty project deletion: %v %s", err, output)
	}
	if _, _, err := server.GetProject(name); !api.StatusErrorCheck(err, http.StatusNotFound) {
		t.Fatalf("project remains after deletion: %v", err)
	}
	volume, _, err := server.GetStoragePoolVolume("default", "custom", volumeName)
	if err != nil || volume.Description != "persistent data" || volume.Config["user.external"] != "preserve" {
		t.Fatalf("attached persistent volume was lost: %v %+v", err, volume)
	}
}
