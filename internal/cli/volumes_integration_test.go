//go:build integration && linux

package cli

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"strings"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/incustest"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func guestCommand(t *testing.T, server incus.InstanceServer, command string) string {
	t.Helper()
	var out, stderr bytes.Buffer
	done := make(chan bool)
	op, err := server.ExecInstance("test-vm", api.InstanceExecPost{Command: []string{"sh", "-c", command}, WaitForWS: true}, &incus.InstanceExecArgs{Stdout: &out, Stderr: &stderr, DataDone: done})
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Wait(); err != nil {
		t.Fatal(err)
	}
	<-done
	if op.Get().Metadata["return"] != float64(0) {
		t.Fatalf("guest command failed: %+v %s", op.Get().Metadata, stderr.String())
	}
	return out.String()
}

func TestIntegrationPersistentVolumeBytesSurviveVMRecreation(t *testing.T) {
	server := incustest.Connect(t)
	volumeName := "garden-ci-data-" + strings.ToLower(rand.Text())
	t.Cleanup(func() {
		if err := server.UseProject("default").DeleteStoragePoolVolume("default", "custom", volumeName); err != nil {
			t.Error(err)
		}
	})
	name := liveProjectName(t, server)
	g := definition.Garden{Projects: []definition.Project{{Name: name}}, Volumes: []definition.Volume{{Pool: "default", Name: volumeName, ContentType: "filesystem"}}, VMs: []definition.VM{{Project: name, Name: "test-vm", Image: os.Getenv("GARDEN_TEST_INCUS_BOOT_IMAGE"), Pool: "default", Network: "incusbr0", Config: definition.Properties{"limits.memory": definition.Value("512MiB")}}}}
	liveSuccess(t, server, g, "apply", "Apply complete: 3 operations")
	vm := server.UseProject(name)
	boot := func() {
		t.Helper()
		instance, etag, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		put := instance.Writable()
		put.Config["security.secureboot"] = "false"
		op, err := vm.UpdateInstance("test-vm", put, etag)
		if err != nil {
			t.Fatal(err)
		}
		if err := op.Wait(); err != nil {
			t.Fatal(err)
		}
		g.VMs[0].Power = definition.Running
		liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
		waitTestGuest(t, vm)
	}
	boot()
	// Add the disk to a running VM through Garden's durable restart path.
	g.VMs[0].Disks = []definition.Disk{{Name: "data", Pool: "default", Volume: volumeName, Path: "/data"}}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	waitTestGuest(t, vm)
	guestCommand(t, vm, "test -d /data && echo garden-persistent-bytes > /data/proof && sync")
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	g.VMs[0].Power = definition.Stopped
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	saved := g.VMs[0]
	g.VMs = nil
	var out bytes.Buffer
	if err := run([]string{"delete", "vm", name, "test-vm"}, strings.NewReader(name+"/test-vm\n"), &out, io.Discard, g, func() (reconcile.Backend, error) { return incusconn.Backend{Server: server}, nil }); err != nil {
		t.Fatal(err)
	}
	saved.Power = definition.Stopped
	g.VMs = []definition.VM{saved}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	boot()
	if got := guestCommand(t, vm, "cat /data/proof"); got != "garden-persistent-bytes\n" {
		t.Fatalf("persistent bytes lost: %q", got)
	}
	g.Volumes = nil
	g.VMs[0].Disks = nil
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	if got := guestCommand(t, vm, "cat /data/proof"); got != "garden-persistent-bytes\n" {
		t.Fatal("omission lost data")
	}
}

func TestIntegrationBlockVolumeAdoptionAndGrowth(t *testing.T) {
	server := incustest.Connect(t).UseProject("default")
	name := "garden-ci-block-" + strings.ToLower(rand.Text())
	if err := server.CreateStoragePoolVolume("default", api.StorageVolumesPost{Name: name, Type: "custom", ContentType: "block", StorageVolumePut: api.StorageVolumePut{Config: api.ConfigMap{"size": "64MiB", "user.external": "preserve"}}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.DeleteStoragePoolVolume("default", "custom", name); err != nil {
			t.Error(err)
		}
	})
	g := definition.Garden{Volumes: []definition.Volume{{Pool: "default", Name: name, ContentType: "block", Size: "128MiB", Config: definition.Properties{"user.note": definition.Value("managed")}}}}
	if _, err := liveCommand(t, server, g, "apply"); err == nil {
		t.Fatal("unowned volume accepted")
	}
	var out bytes.Buffer
	if err := run([]string{"adopt", "volume", "default", name}, strings.NewReader(""), &out, io.Discard, g, func() (reconcile.Backend, error) { return incusconn.Backend{Server: server}, nil }); err != nil {
		t.Fatal(err)
	}
	before, _, err := server.GetStoragePoolVolume("default", "custom", name)
	if err != nil {
		t.Fatal(err)
	}
	if before.Config["size"] != "64MiB" || before.Config["user.note"] != "" {
		t.Fatal("adoption changed settings")
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	actual, _, err := server.GetStoragePoolVolume("default", "custom", name)
	if err != nil || actual.Config["size"] != "128MiB" || actual.Config["user.external"] != "preserve" {
		t.Fatalf("growth failed: %v %+v", err, actual)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	g.Volumes[0].Size = "64MiB"
	if _, err := liveCommand(t, server, g, "apply"); err == nil {
		t.Fatal("shrink accepted")
	}
}
