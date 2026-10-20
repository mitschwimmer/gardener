//go:build integration && linux

package cli

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/incustest"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func TestIntegrationManagedBridgeLifecycle(t *testing.T) {
	server := incustest.Connect(t)
	network := "gci" + strings.ToLower(rand.Text())[:10]
	t.Cleanup(func() {
		if _, _, err := server.UseProject("default").GetNetwork(network); api.StatusErrorCheck(err, 404) {
			return
		}
		if err := server.UseProject("default").DeleteNetwork(network); err != nil {
			t.Error(err)
		}
	})
	project := liveProjectName(t, server)
	g := definition.Garden{Projects: []definition.Project{{Name: project}}, Networks: []definition.Network{{Name: network, IPv4Address: "192.0.2.1/24", IPv6Address: "none", Config: definition.Properties{"user.remove": definition.Value("old")}}}, VMs: []definition.VM{{Project: project, Name: "test-vm", Image: os.Getenv("GARDEN_TEST_INCUS_BOOT_IMAGE"), Pool: "default", Network: network, Config: definition.Properties{"limits.memory": definition.Value("512MiB")}}}}
	liveSuccess(t, server, g, "plan", "create network "+network)
	if _, _, err := server.GetNetwork(network); !api.StatusErrorCheck(err, 404) {
		t.Fatal("plan created a network", err)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 3 operations")
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	vm := server.UseProject(project)
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
	guestCommand(t, vm, `for i in $(seq 1 30); do ip -4 addr show | grep -F 'inet 192.0.2.' && exit 0; sleep 1; done; exit 1`)
	instance, _, err = vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	identity := instance.Config["volatile.uuid"]
	actual, etag, err := server.GetNetwork(network)
	if err != nil {
		t.Fatal(err)
	}
	netPut := actual.Writable()
	netPut.Config["user.external"] = "keep"
	delete(netPut.Config, definition.OwnerKey)
	if err := server.UpdateNetwork(network, netPut, etag); err != nil {
		t.Fatal(err)
	}
	if _, err := liveCommand(t, server, g, "apply"); err == nil {
		t.Fatal("unowned network accepted")
	}
	g.Networks[0].Description = definition.Value("managed bridge")
	g.Networks[0].Config["user.remove"] = nil
	var out bytes.Buffer
	if err := run([]string{"adopt", "network", network}, strings.NewReader(""), &out, io.Discard, g, func() (reconcile.Backend, error) { return incusconn.Backend{Server: server}, nil }); err != nil {
		t.Fatal(err)
	}
	actual, _, err = server.GetNetwork(network)
	if err != nil || actual.Description != "" || actual.Config["user.remove"] != "old" {
		t.Fatalf("adoption changed settings: %v %+v", err, actual)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	actual, _, err = server.GetNetwork(network)
	if err != nil || actual.Config["user.external"] != "keep" || actual.Config["user.remove"] != "" || actual.Description != "managed bridge" {
		t.Fatalf("metadata update failed: %v %+v", err, actual)
	}
	instance, _, err = vm.GetInstance("test-vm")
	if err != nil || instance.Status != "Running" || instance.Config["volatile.uuid"] != identity {
		t.Fatal("network metadata update disturbed VM", err)
	}
	g.Networks[0].IPv4NAT = true
	if _, err := liveCommand(t, server, g, "apply"); err == nil {
		t.Fatal("NAT changed automatically")
	}
	g.Networks[0].IPv4NAT = false
	g.Networks[0].IPv4Address = "192.0.3.1/24"
	if _, err := liveCommand(t, server, g, "apply"); err == nil {
		t.Fatal("subnet changed automatically")
	}
	g.Networks[0].IPv4Address = "192.0.2.1/24"
	// A stale plan must not overwrite an external network edit.
	g.Networks[0].Description = definition.Value("new description")
	backend := incusconn.Backend{Server: server}
	plan, err := reconcile.Build(g, backend)
	if err != nil {
		t.Fatal(err)
	}
	actual, etag, err = server.GetNetwork(network)
	if err != nil {
		t.Fatal(err)
	}
	netPut = actual.Writable()
	netPut.Config["user.concurrent"] = "external"
	if err := server.UpdateNetwork(network, netPut, etag); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(backend, io.Discard); err == nil {
		t.Fatal("stale network plan accepted")
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
	g.Networks = nil
	liveSuccess(t, server, g, "apply", "orphan: network "+network+" (preserved)")
	if _, _, err := server.GetNetwork(network); err != nil {
		t.Fatal("orphan network removed", err)
	}
}
