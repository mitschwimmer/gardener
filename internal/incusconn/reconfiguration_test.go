package incusconn

import (
	"errors"
	"io"
	"maps"
	"reflect"
	"strings"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

type reconfigurationClient struct {
	recordingClient
	actions   []string
	failAt    int
	failAfter bool
	after     func(int)
}

type completedOperation struct {
	incus.Operation
	complete func() error
}

func (o completedOperation) Wait() error { return o.complete() }
func (f *reconfigurationClient) UseProject(name string) incus.InstanceServer {
	f.selected = name
	return f
}
func (f *reconfigurationClient) GetInstance(string) (*api.Instance, string, error) {
	v := f.instance
	v.Config = maps.Clone(v.Config)
	return &v, "current-etag", nil
}
func (f *reconfigurationClient) GetProjects() ([]api.Project, error) {
	return []api.Project{f.project}, nil
}
func (f *reconfigurationClient) GetInstances(api.InstanceType) ([]api.Instance, error) {
	return []api.Instance{f.instance}, nil
}
func (f *reconfigurationClient) GetStoragePool(string) (*api.StoragePool, string, error) {
	return &api.StoragePool{Status: "Created"}, "", nil
}
func (f *reconfigurationClient) GetNetwork(string) (*api.Network, string, error) {
	return &api.Network{Managed: true, Type: "bridge", Status: "Created"}, "", nil
}
func (f *reconfigurationClient) GetInstanceState(string) (*api.InstanceState, string, error) {
	return &api.InstanceState{Status: f.instance.Status}, "power-etag", nil
}
func (f *reconfigurationClient) operation(action string, change func()) (incus.Operation, error) {
	f.actions = append(f.actions, action)
	step := len(f.actions)
	return completedOperation{complete: func() error {
		if step == f.failAt && !f.failAfter {
			return errors.New("injected wait failure")
		}
		change()
		if f.after != nil {
			f.after(step)
		}
		if step == f.failAt {
			return errors.New("injected lost success response")
		}
		return nil
	}}, nil
}
func (f *reconfigurationClient) UpdateInstance(_ string, put api.InstancePut, etag string) (incus.Operation, error) {
	if etag != "current-etag" {
		return nil, errors.New("missing fresh config ETag")
	}
	return f.operation("update", func() { f.instance.InstancePut = put })
}
func (f *reconfigurationClient) UpdateInstanceState(_ string, put api.InstanceStatePut, etag string) (incus.Operation, error) {
	if etag != "power-etag" || put.Force || put.Stateful || put.Timeout != 30 {
		return nil, errors.New("unsafe power operation")
	}
	return f.operation(put.Action, func() {
		f.instance.Status = map[string]string{"stop": "Stopped", "start": "Running"}[put.Action]
		f.instance.Config["volatile.last_state.power"] = strings.ToUpper(f.instance.Status)
	})
}

func reconfigurationFixture() (*reconfigurationClient, definition.Garden) {
	f := &reconfigurationClient{recordingClient: *clientFixture()}
	f.instance.Status = "Running"
	f.instance.Config["volatile.base_image"] = strings.Repeat("a", 64)
	f.instance.Config["volatile.uuid"] = "original-identity"
	f.instance.Config["limits.cpu"] = "1"
	f.instance.Devices["root"] = map[string]string{"type": "disk", "pool": "pool", "path": "/"}
	f.instance.Devices["eth0"] = map[string]string{"type": "nic", "network": "bridge", "name": "eth0"}
	g := definition.Garden{Projects: []definition.Project{{Name: "apps"}}, VMs: []definition.VM{{
		Project: "apps", Name: "wiki", Image: strings.Repeat("a", 64), Pool: "pool", Network: "bridge",
		Config: definition.Properties{"limits.cpu": definition.Value("2")},
	}}}
	return f, g
}

func TestReconfigurationOrdersAndPreservesPowerAndProperties(t *testing.T) {
	for _, stop := range []bool{false, true} {
		f, g := reconfigurationFixture()
		want := []string{"update", "stop", "update", "start", "update"}
		status := "Running"
		if stop {
			g.VMs[0].Power = definition.Stopped
			want = []string{"update", "stop", "update", "update"}
			status = "Stopped"
		}
		b := Backend{Server: f}
		plan, err := reconcile.Build(g, b)
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(b, io.Discard); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f.actions, want) || f.instance.Status != status || f.instance.Config[definition.PendingPowerKey] != "" ||
			f.instance.Config["limits.cpu"] != "2" || f.instance.Config["user.external"] != "keep" || f.instance.Devices["data"]["source"] != "keep" {
			t.Fatalf("wrong reconfiguration result: actions=%v instance=%+v", f.actions, f.instance)
		}
	}
}

func TestReconfigurationResumesEveryInterruptedPhase(t *testing.T) {
	for _, lostResponse := range []bool{false, true} {
		for failAt := 1; failAt <= 5; failAt++ {
			f, g := reconfigurationFixture()
			f.failAt, f.failAfter = failAt, lostResponse
			b := Backend{Server: f}
			plan, err := reconcile.Build(g, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(b, io.Discard); err == nil || len(f.actions) != failAt {
				t.Fatalf("phase %d lost=%v did not stop: %v actions=%v", failAt, lostResponse, err, f.actions)
			}
			f.failAt = 0
			f.actions = nil
			plan, err = reconcile.Build(g, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(b, io.Discard); err != nil {
				t.Fatalf("phase %d lost=%v failed resume: %v", failAt, lostResponse, err)
			}
			if failAt == 5 && !lostResponse && !reflect.DeepEqual(f.actions, []string{"update"}) {
				t.Fatalf("cleanup recovery caused an unnecessary restart: %v", f.actions)
			}
			if f.instance.Status != "Running" || f.instance.Config["limits.cpu"] != "2" || f.instance.Config[definition.PendingPowerKey] != "" {
				t.Fatalf("lost unmanaged original power intent: %+v", f.instance)
			}
			plan, err = reconcile.Build(g, b)
			if err != nil || len(plan.Operations) != 0 {
				t.Fatalf("not idempotent after recovery: %v %+v", err, plan)
			}
		}
	}
}

func TestReconfigurationRefusesConcurrentChangesBetweenPhases(t *testing.T) {
	for _, change := range []func(*reconfigurationClient){
		func(f *reconfigurationClient) { f.instance.Description = "external edit" },
		func(f *reconfigurationClient) { f.instance.Config[definition.OwnerKey] = "other" },
		func(f *reconfigurationClient) { f.instance.Config["volatile.uuid"] = "replacement" },
		func(f *reconfigurationClient) { f.project.Config[definition.OwnerKey] = "other" },
		func(f *reconfigurationClient) { f.project.Config["features.networks"] = "true" },
	} {
		f, g := reconfigurationFixture()
		f.after = func(step int) {
			if step == 2 {
				change(f)
			}
		}
		b := Backend{Server: f}
		plan, err := reconcile.Build(g, b)
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(b, io.Discard); err == nil || len(f.actions) != 2 || f.instance.Config[definition.PendingPowerKey] != "running" {
			t.Fatalf("continued after concurrent edit: %v actions=%v", err, f.actions)
		}
	}
}
