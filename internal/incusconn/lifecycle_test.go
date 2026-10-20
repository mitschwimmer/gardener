package incusconn

import (
	"errors"
	"maps"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func lifecycleOperation(kind reconcile.Kind) (*recordingClient, reconcile.Operation) {
	f := clientFixture()
	op := reconcile.Operation{Kind: kind, Name: "apps"}
	if kind == reconcile.AdoptProject {
		delete(f.project.Config, definition.OwnerKey)
	}
	if kind == reconcile.AdoptVM {
		delete(f.instance.Config, definition.OwnerKey)
	}
	p, v := f.project, f.instance
	op.BeforeProject = &p
	if kind == reconcile.AdoptVM || kind == reconcile.DeleteVM {
		op.Project, op.Name = "apps", "wiki"
		op.BeforeInstance = &v
		op.InstancePut = v.Writable()
		op.InstancePut.Config = maps.Clone(v.Config)
		op.InstancePut.Config[definition.OwnerKey] = definition.Owner
	} else {
		op.ProjectPut = p.Writable()
		op.ProjectPut.Config = maps.Clone(p.Config)
		op.ProjectPut.Config[definition.OwnerKey] = definition.Owner
	}
	return f, op
}

func TestLifecycleUsesCorrectProjectETagAndWait(t *testing.T) {
	for _, kind := range []reconcile.Kind{reconcile.AdoptProject, reconcile.AdoptVM, reconcile.DeleteProject, reconcile.DeleteVM} {
		t.Run(string(kind), func(t *testing.T) {
			f, op := lifecycleOperation(kind)
			if err := (Backend{Server: f}).Execute(op); err != nil {
				t.Fatal(err)
			}
			if f.writes != 1 {
				t.Fatal("expected exactly one mutation")
			}
			if op.Project == "" {
				if f.selected != "default" || f.op.waits != 0 {
					t.Fatal("project operation used wrong project or wait")
				}
			} else if f.selected != "apps" || f.op.waits != 1 {
				t.Fatal("VM operation used wrong project or did not wait")
			}
			switch kind {
			case reconcile.AdoptProject:
				if f.etag != "project-etag" || f.projectPut.Config[definition.OwnerKey] != definition.Owner {
					t.Fatal("missing project ETag or owner")
				}
			case reconcile.AdoptVM:
				if f.etag != "instance-etag" || f.instancePut.Config["user.external"] != "keep" {
					t.Fatal("missing VM ETag or preserved metadata")
				}
			default:
				if f.deleted != op.Name {
					t.Fatal("wrong resource deleted")
				}
			}
		})
	}
}

func TestLifecycleRefusesChangedResources(t *testing.T) {
	for _, kind := range []reconcile.Kind{reconcile.AdoptProject, reconcile.AdoptVM, reconcile.DeleteProject, reconcile.DeleteVM} {
		f, op := lifecycleOperation(kind)
		if op.Project == "" {
			f.project.Description = "concurrent edit"
		} else {
			f.instance.Description = "concurrent edit"
		}
		if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
			t.Fatalf("%s ignored changed resource", kind)
		}
	}
	for _, change := range []func(*recordingClient){
		func(f *recordingClient) { f.project.UsedBy = []string{"new resource"} },
		func(f *recordingClient) { f.instances = []api.Instance{{Name: "new-vm"}} },
	} {
		f, op := lifecycleOperation(reconcile.DeleteProject)
		change(f)
		if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
			t.Fatal("deleted project that became nonempty after confirmation")
		}
	}
	f, op := lifecycleOperation(reconcile.DeleteVM)
	f.instance.ExpandedDevices = api.DevicesMap{"data": {"type": "disk", "pool": "pool", "source": "keep", "dependent": "true"}}
	if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
		t.Fatal("deleted VM with an inherited dependent disk")
	}
	f, op = lifecycleOperation(reconcile.DeleteVM)
	f.project.Config = maps.Clone(f.project.Config)
	delete(f.project.Config, definition.OwnerKey)
	if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
		t.Fatal("deleted VM after parent ownership changed")
	}
}

func TestLifecycleErrorsPropagate(t *testing.T) {
	want := errors.New("injected failure")
	for _, kind := range []reconcile.Kind{reconcile.AdoptProject, reconcile.AdoptVM, reconcile.DeleteProject, reconcile.DeleteVM} {
		f, op := lifecycleOperation(kind)
		f.requestError = want
		if err := (Backend{Server: f}).Execute(op); !errors.Is(err, want) || f.op.waits != 0 {
			t.Fatalf("%s request error: %v", kind, err)
		}
		if op.Project != "" {
			f, op = lifecycleOperation(kind)
			f.op.err = want
			if err := (Backend{Server: f}).Execute(op); !errors.Is(err, want) || f.op.waits != 1 {
				t.Fatalf("%s wait error: %v", kind, err)
			}
		}
	}
}
