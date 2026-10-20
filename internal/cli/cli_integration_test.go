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

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/incusconn"
	"github.com/mitschwimmer/gardener/internal/incustest"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func liveCommand(t *testing.T, server incus.InstanceServer, g definition.Garden, command string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := run([]string{command}, &output, io.Discard, g, func() (reconcile.Backend, error) {
		return incusconn.Backend{Server: server}, nil
	})
	return output.String(), err
}

func liveSuccess(t *testing.T, server incus.InstanceServer, g definition.Garden, command, want string) string {
	t.Helper()
	output, err := liveCommand(t, server, g, command)
	if err != nil || !strings.Contains(output, want) {
		t.Fatalf("%s: error=%v output=%s; want %q", command, err, output, want)
	}
	return output
}

func liveProjectName(t *testing.T, server incus.InstanceServer) string {
	t.Helper()
	name := "garden-ci-" + strings.ToLower(rand.Text())
	t.Cleanup(func() {
		_, _, err := server.GetProject(name)
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return
		}
		if err != nil {
			t.Error(err)
			return
		}
		project := server.UseProject(name)
		instances, err := project.GetInstances(api.InstanceTypeAny)
		if err != nil {
			t.Error(err)
			return
		}
		for _, instance := range instances {
			op, err := project.DeleteInstance(instance.Name)
			if err == nil {
				err = op.Wait()
			}
			if err != nil {
				t.Errorf("clean up VM %s/%s: %v", name, instance.Name, err)
				return
			}
		}
		if err := server.DeleteProject(name); err != nil {
			t.Errorf("clean up project %s: %v", name, err)
		}
	})
	return name
}

func TestIntegrationCompiledGardenCommands(t *testing.T) {
	incustest.Connect(t)
	// Exercise the public entry point with the intentionally empty compiled garden.
	for _, command := range []string{"plan", "apply"} {
		var output bytes.Buffer
		if err := Run([]string{command}, &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Plan: 0 operations, 0 orphans, 0 manual actions") {
			t.Fatalf("unexpected %s output: %s", command, output.String())
		}
	}
}

func TestIntegrationPlanApplyVM(t *testing.T) {
	server := incustest.Connect(t)
	name := liveProjectName(t, server)
	image := os.Getenv("GARDEN_TEST_INCUS_IMAGE")
	if image == "" {
		t.Fatal("GARDEN_TEST_INCUS_IMAGE must be the full fingerprint of the disposable VM image")
	}
	g := definition.Garden{
		Projects: []definition.Project{{Name: name, Description: definition.Value("initial project")}},
		VMs: []definition.VM{{
			Project: name, Name: "test-vm", Image: image, Pool: "default", Network: "incusbr0",
			Description: definition.Value("initial VM"), Power: definition.Stopped,
			Config: definition.Properties{
				"limits.cpu": definition.Value("1"), "limits.memory": definition.Value("256MiB"),
				"user.keep": definition.Value("keep"), "user.remove": definition.Value("remove"),
			},
		}},
	}
	vm := server.UseProject(name)
	backend := incusconn.Backend{Server: server}

	t.Run("plan is read-only and validates references", func(t *testing.T) {
		liveSuccess(t, server, g, "plan", "Plan: 2 operations, 0 orphans, 0 manual actions")
		if _, _, err := server.GetProject(name); !api.StatusErrorCheck(err, http.StatusNotFound) {
			t.Fatalf("plan created a project or read failed: %v", err)
		}
		bad := g
		bad.VMs = append([]definition.VM(nil), g.VMs...)
		bad.VMs[0].Pool = "garden-ci-missing-pool"
		if _, err := liveCommand(t, server, bad, "apply"); err == nil || !strings.Contains(err.Error(), "storage pool") {
			t.Fatalf("missing reference was accepted: %v", err)
		}
		if _, _, err := server.GetProject(name); !api.StatusErrorCheck(err, http.StatusNotFound) {
			t.Fatalf("invalid references allowed a mutation: %v", err)
		}
	})
	if t.Failed() {
		return
	}

	liveSuccess(t, server, g, "apply", "Apply complete: 2 operations")
	project, _, err := server.GetProject(name)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range reconcile.ProjectConfig() {
		if project.Config[key] != value {
			t.Fatalf("project config %s=%q; want %q", key, project.Config[key], value)
		}
	}
	instance, _, err := vm.GetInstance("test-vm")
	if err != nil {
		t.Fatal(err)
	}
	if instance.Status != "Stopped" || instance.Type != "virtual-machine" || len(instance.Profiles) != 0 ||
		instance.Config[definition.OwnerKey] != definition.Owner || instance.Config["volatile.base_image"] != image ||
		instance.Devices["root"]["pool"] != "default" || instance.Devices["eth0"]["network"] != "incusbr0" {
		t.Fatalf("unexpected created VM: %+v", instance)
	}
	liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")

	t.Run("updates preserve unspecified properties and remove explicit keys", func(t *testing.T) {
		put := instance.Writable()
		put.Config["user.external"] = "preserve"
		op, err := vm.UpdateInstance("test-vm", put, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := op.Wait(); err != nil {
			t.Fatal(err)
		}
		g.Projects[0].Description = definition.Value("updated project")
		g.VMs[0].Description = nil
		g.VMs[0].Config = definition.Properties{
			"limits.cpu": definition.Value("2"), "limits.memory": definition.Value("512MiB"), "user.remove": nil,
		}
		before, _, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		liveSuccess(t, server, g, "plan", "Plan: 2 operations")
		afterPlan, _, err := vm.GetInstance("test-vm")
		if err != nil || !reflect.DeepEqual(before.Writable(), afterPlan.Writable()) {
			t.Fatalf("plan mutated VM or read failed: %v", err)
		}
		liveSuccess(t, server, g, "apply", "Apply complete: 2 operations")
		after, _, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		if after.Description != "initial VM" || after.Config["user.external"] != "preserve" || after.Config["user.keep"] != "keep" ||
			after.Config["limits.cpu"] != "2" || after.Config["limits.memory"] != "512MiB" {
			t.Fatalf("unexpected updated VM: %+v", after)
		}
		if _, exists := after.Config["user.remove"]; exists {
			t.Fatal("explicitly removed property remains")
		}
		liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	})
	if t.Failed() {
		return
	}

	t.Run("stale VM stops apply after project update and fresh apply resumes", func(t *testing.T) {
		g.Projects[0].Description = definition.Value("resumed project")
		g.VMs[0].Description = definition.Value("resumed VM")
		plan, err := reconcile.Build(g, backend)
		if err != nil || len(plan.Operations) != 2 {
			t.Fatalf("build partial-failure plan: %v %+v", err, plan)
		}
		current, etag, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		put := current.Writable()
		put.Config["user.concurrent"] = "preserve too"
		op, err := vm.UpdateInstance("test-vm", put, etag)
		if err != nil {
			t.Fatal(err)
		}
		if err := op.Wait(); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := plan.Apply(backend, &output); err == nil || !strings.Contains(err.Error(), "after 1 completed operations") {
			t.Fatalf("stale state did not stop partial apply: %v %s", err, output.String())
		}
		p, _, err := server.GetProject(name)
		if err != nil || p.Description != "resumed project" {
			t.Fatalf("successful project update was lost: %v %+v", err, p)
		}
		liveSuccess(t, server, g, "apply", "Apply complete: 1 operations")
		v, _, err := vm.GetInstance("test-vm")
		if err != nil || v.Description != "resumed VM" || v.Config["user.concurrent"] != "preserve too" {
			t.Fatalf("resume lost concurrent data: %v %+v", err, v)
		}
		liveSuccess(t, server, g, "apply", "Apply complete: 0 operations")
	})

	t.Run("replacement requirement blocks otherwise safe project update", func(t *testing.T) {
		bad := definition.Garden{Projects: append([]definition.Project(nil), g.Projects...), VMs: append([]definition.VM(nil), g.VMs...)}
		bad.Projects[0].Description = definition.Value("must not apply")
		bad.VMs[0].Image = strings.Repeat("0", 64)
		for _, command := range []string{"plan", "apply"} {
			output, err := liveCommand(t, server, bad, command)
			if err == nil || !strings.Contains(output, "seed image differs") {
				t.Fatalf("%s allowed replacement: %v %s", command, err, output)
			}
		}
		p, _, err := server.GetProject(name)
		if err != nil || p.Description == "must not apply" {
			t.Fatalf("blocked apply mutated project: %v %+v", err, p)
		}
	})

	t.Run("removed definitions preserve project and VM as orphans", func(t *testing.T) {
		output := liveSuccess(t, server, definition.Garden{}, "apply", "Plan: 0 operations, 2 orphans, 0 manual actions")
		if !strings.Contains(output, "orphan: project "+name) || !strings.Contains(output, "orphan: instance "+name+"/test-vm") {
			t.Fatalf("missing orphan reports: %s", output)
		}
		if _, _, err := server.GetProject(name); err != nil {
			t.Fatal(err)
		}
		if _, _, err := vm.GetInstance("test-vm"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unmanaged VM collision refuses all mutations", func(t *testing.T) {
		current, etag, err := vm.GetInstance("test-vm")
		if err != nil {
			t.Fatal(err)
		}
		put := current.Writable()
		delete(put.Config, definition.OwnerKey)
		op, err := vm.UpdateInstance("test-vm", put, etag)
		if err != nil {
			t.Fatal(err)
		}
		if err := op.Wait(); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"plan", "apply"} {
			if _, err := liveCommand(t, server, g, command); err == nil || !strings.Contains(err.Error(), "not owned by Garden") {
				t.Fatalf("%s accepted unmanaged VM: %v", command, err)
			}
		}
		after, _, err := vm.GetInstance("test-vm")
		if err != nil || !reflect.DeepEqual(put, after.Writable()) {
			t.Fatalf("unmanaged VM was changed: %v %+v", err, after)
		}
	})
}

func TestIntegrationUnmanagedProjectCollision(t *testing.T) {
	server := incustest.Connect(t)
	name := liveProjectName(t, server)
	config := reconcile.ProjectConfig()
	delete(config, definition.OwnerKey)
	if err := server.CreateProject(api.ProjectsPost{Name: name, ProjectPut: api.ProjectPut{Description: "unmanaged", Config: config}}); err != nil {
		t.Fatal(err)
	}
	g := definition.Garden{Projects: []definition.Project{{Name: name, Description: definition.Value("must not apply")}}}
	for _, command := range []string{"plan", "apply"} {
		if _, err := liveCommand(t, server, g, command); err == nil || !strings.Contains(err.Error(), "not owned by Garden") {
			t.Fatalf("%s accepted unmanaged project: %v", command, err)
		}
	}
	p, _, err := server.GetProject(name)
	if err != nil || p.Description != "unmanaged" || p.Config[definition.OwnerKey] != "" {
		t.Fatalf("unmanaged project was changed: %v %+v", err, p)
	}
}
