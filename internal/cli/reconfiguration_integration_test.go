//go:build integration && linux

package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/incustest"
)

// Guest readiness is a fixture prerequisite for graceful ACPI stops. Garden
// itself continues to trust Incus operation completion without health polling.
func waitTestGuest(t *testing.T, server incus.InstanceServer) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		state, _, err := server.GetInstanceState("test-vm")
		if err == nil && state.Status == "Running" && state.Processes > 0 {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("disposable guest agent did not become ready for graceful shutdown")
}

func TestIntegrationVMRestartAndPendingStartRecovery(t *testing.T) {
	server := incustest.Connect(t)
	name := liveProjectName(t, server)
	image := os.Getenv("GARDEN_TEST_INCUS_BOOT_IMAGE")
	if image == "" {
		t.Fatal("GARDEN_TEST_INCUS_BOOT_IMAGE must name the bootable VM fixture")
	}
	g := definition.Garden{Projects: []definition.Project{{Name: name}}, VMs: []definition.VM{{
		Project: name, Name: "test-vm", Image: image, Pool: "default", Network: "incusbr0",
		Config: definition.Properties{"limits.cpu": definition.Value("1"), "limits.memory": definition.Value("512MiB")},
	}}}
	liveSuccess(t, server, g, "apply", "Apply complete: 2 operations")
	vm := server.UseProject(name)
	instance, etag, err := vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	put := instance.Writable()
	put.Config["security.secureboot"] = "false" // Alpine fixture, outside Garden's managed properties.
	put.Config["user.external"] = "preserve"
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
	before, _, err := vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	identity := before.Config["volatile.uuid"]
	g.VMs[0].Power = definition.Unmanaged
	g.VMs[0].Config["limits.cpu"] = definition.Value("2")
	liveSuccess(t, server, g, "plan", "reconfigure VM "+name+"/test-vm")
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	instance, _, err = vm.GetInstance("test-vm")
	if err != nil || instance.Status != "Running" || instance.Config["limits.cpu"] != "2" ||
		instance.Config[definition.PendingPowerKey] != "" || instance.Config["user.external"] != "preserve" || instance.Config["volatile.uuid"] != identity {
		t.Fatalf("restart did not preserve identity/state: %v %+v", err, instance)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	waitTestGuest(t, vm)
	g.VMs[0].Config["limits.memory"] = definition.Value("768MiB")
	g.VMs[0].Power = definition.Stopped
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	instance, etag, err = vm.GetInstance("test-vm")
	if err != nil || instance.Status != "Stopped" || instance.Config["limits.memory"] != "768MiB" || instance.Config[definition.PendingPowerKey] != "" {
		t.Fatalf("explicit stopped intent was ignored: %v %+v", err, instance)
	}

	// Reproduce an interruption after the config update but before start. The
	// requested config already matches, and power is deliberately unmanaged.
	g.VMs[0].Power = definition.Unmanaged
	put = instance.Writable()
	put.Config[definition.PendingPowerKey] = "running"
	put.Config["raw.qemu"] = "-garden-ci-invalid-option"
	op, err = vm.UpdateInstance("test-vm", put, etag)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Wait(); err != nil {
		t.Fatal(err)
	}
	_, startErr := liveCommand(t, server, g, "apply")
	if startErr == nil || !strings.Contains(startErr.Error(), "start VM") {
		t.Fatalf("expected a genuine QEMU start failure: %v", startErr)
	}
	for _, want := range []string{"power=Stopped, pending-power=running", "current state unverified", "Confirmed substeps within this reconfiguration:", "incus info 'garden:test-vm' --show-log --project '" + name + "'", "incus config show 'garden:test-vm' --project '" + name + "'", "garden plan, then garden apply"} {
		if !strings.Contains(startErr.Error(), want) {
			t.Fatalf("missing diagnostic %q: %s", want, startErr)
		}
	}
	var detail interface{ OperationID() string }
	if !errors.As(startErr, &detail) || detail.OperationID() == "" || !strings.Contains(startErr.Error(), "incus operation show 'garden:"+detail.OperationID()+"'") {
		t.Fatal("missing live operation ID", startErr)
	}
	// Validate the displayed CLI syntax against the same isolated remote. Never
	// execute an error string as shell code, and never use the operator's config.
	for _, args := range [][]string{
		{"info", "garden:test-vm", "--show-log", "--project", name},
		{"config", "show", "garden:test-vm", "--project", name},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		output, err := exec.CommandContext(ctx, "incus", args...).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("diagnostic command %v failed: %v %s", args, err, output)
		}
	}
	instance, etag, err = vm.GetInstance("test-vm")
	if err != nil || instance.Status != "Stopped" || instance.Config[definition.PendingPowerKey] != "running" {
		t.Fatalf("failed start lost pending intent: %v %+v", err, instance)
	}
	put = instance.Writable()
	delete(put.Config, "raw.qemu")
	op, err = vm.UpdateInstance("test-vm", put, etag)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Wait(); err != nil {
		t.Fatal(err)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	instance, _, err = vm.GetInstance("test-vm")
	if err != nil || instance.Status != "Running" || instance.Config[definition.PendingPowerKey] != "" || instance.Config["volatile.uuid"] != identity {
		t.Fatalf("failed unmanaged restart did not resume: %v %+v", err, instance)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
}
