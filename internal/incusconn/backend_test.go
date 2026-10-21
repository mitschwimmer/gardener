package incusconn

import (
	"errors"
	"maps"
	"reflect"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

// Embed the upstream interfaces solely to avoid implementing unused methods.
// Any accidental extra client call fails the test rather than emulating a server.
type recordingClient struct {
	incus.InstanceServer
	selected     string
	selections   []string
	project      api.Project
	instance     api.Instance
	state        api.InstanceState
	projectPut   api.ProjectPut
	instancePut  api.InstancePut
	created      api.InstancesPost
	statePut     api.InstanceStatePut
	etag         string
	writes       int
	op           recordingOperation
	requestError error
	instances    []api.Instance
	deleted      string
}

type recordingOperation struct {
	incus.Operation
	waits int
	err   error
}

func (o *recordingOperation) Get() api.Operation { return api.Operation{ID: "test-operation"} }
func (o *recordingOperation) Wait() error        { o.waits++; return o.err }
func (f *recordingClient) UseProject(name string) incus.InstanceServer {
	f.selected = name
	f.selections = append(f.selections, name)
	return f
}
func (f *recordingClient) GetProject(name string) (*api.Project, string, error) {
	return &f.project, "project-etag", nil
}
func (f *recordingClient) GetInstance(name string) (*api.Instance, string, error) {
	return &f.instance, "instance-etag", nil
}
func (f *recordingClient) GetInstances(api.InstanceType) ([]api.Instance, error) {
	return f.instances, nil
}
func (f *recordingClient) DeleteProject(name string) error {
	f.writes++
	f.deleted = name
	return f.requestError
}
func (f *recordingClient) DeleteInstance(name string) (incus.Operation, error) {
	f.writes++
	f.deleted = name
	return &f.op, f.requestError
}
func (f *recordingClient) GetInstanceState(name string) (*api.InstanceState, string, error) {
	return &f.state, "state-etag", nil
}
func (f *recordingClient) CreateProject(p api.ProjectsPost) error {
	f.writes++
	f.projectPut = p.ProjectPut
	return f.requestError
}
func (f *recordingClient) UpdateProject(name string, put api.ProjectPut, etag string) error {
	f.writes++
	f.projectPut = put
	f.etag = etag
	return f.requestError
}
func (f *recordingClient) CreateInstance(p api.InstancesPost) (incus.Operation, error) {
	f.writes++
	f.created = p
	return &f.op, f.requestError
}
func (f *recordingClient) UpdateInstance(name string, put api.InstancePut, etag string) (incus.Operation, error) {
	f.writes++
	f.instancePut = put
	f.etag = etag
	return &f.op, f.requestError
}
func (f *recordingClient) UpdateInstanceState(name string, put api.InstanceStatePut, etag string) (incus.Operation, error) {
	f.writes++
	f.statePut = put
	f.etag = etag
	return &f.op, f.requestError
}

func clientFixture() *recordingClient {
	return &recordingClient{project: api.Project{Name: "apps", ProjectPut: api.ProjectPut{Config: reconcile.ProjectConfig()}}, instance: api.Instance{Name: "wiki", Type: "virtual-machine", Status: "Stopped", InstancePut: api.InstancePut{Config: api.ConfigMap{definition.OwnerKey: definition.Owner, "user.external": "keep"}, Profiles: []string{}, Devices: api.DevicesMap{"data": {"type": "disk", "source": "keep"}}}}, state: api.InstanceState{Status: "Stopped"}}
}

func TestUpdatePreservesRequestUsesProjectAndETagAndWaits(t *testing.T) {
	f := clientFixture()
	before := f.instance
	put := before.Writable()
	put.Config = maps.Clone(put.Config)
	put.Config["limits.cpu"] = "2"
	op := reconcile.Operation{Kind: reconcile.UpdateVM, Project: "apps", Name: "wiki", BeforeInstance: &before, InstancePut: put}
	if err := (Backend{Server: f}).Execute(op); err != nil {
		t.Fatal(err)
	}
	if f.selected != "apps" || f.etag != "instance-etag" || f.writes != 1 || f.op.waits != 1 || !reflect.DeepEqual(f.instancePut, put) {
		t.Fatalf("incorrect API usage: %+v", f)
	}
}

func TestRefuseStaleOrUnownedInstance(t *testing.T) {
	for _, change := range []func(*recordingClient){
		func(f *recordingClient) {
			f.instance.Config = maps.Clone(f.instance.Config)
			delete(f.instance.Config, definition.OwnerKey)
		},
		func(f *recordingClient) { f.instance.Description = "external edit" },
		func(f *recordingClient) { f.instance.Status = "Running" },
		func(f *recordingClient) { f.instance.Ephemeral = true },
	} {
		f := clientFixture()
		before := f.instance
		change(f)
		op := reconcile.Operation{Kind: reconcile.UpdateVM, Project: "apps", Name: "wiki", BeforeInstance: &before, InstancePut: before.Writable()}
		if err := (Backend{Server: f}).Execute(op); err == nil {
			t.Fatal("expected refusal")
		}
		if f.writes != 0 {
			t.Fatal("unsafe write")
		}
	}
}

func TestProjectUpdateGuardAndETag(t *testing.T) {
	f := clientFixture()
	before := f.project
	put := before.Writable()
	put.Description = "new"
	op := reconcile.Operation{Kind: reconcile.UpdateProject, Name: "apps", BeforeProject: &before, ProjectPut: put}
	if err := (Backend{Server: f}).Execute(op); err != nil {
		t.Fatal(err)
	}
	if f.selected != "default" || f.etag != "project-etag" || f.projectPut.Description != "new" {
		t.Fatal("incorrect project update")
	}
	f.writes = 0
	f.project.Description = "external edit"
	if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
		t.Fatal("stale project was updated")
	}
}

func TestCreateChecksProjectOwnershipAndWaits(t *testing.T) {
	f := clientFixture()
	op := reconcile.Operation{Kind: reconcile.CreateVM, Project: "apps", Name: "wiki", InstancePost: api.InstancesPost{Name: "wiki", Type: api.InstanceTypeVM}}
	if err := (Backend{Server: f}).Execute(op); err != nil {
		t.Fatal(err)
	}
	if f.selected != "apps" || f.op.waits != 1 || f.created.Type != api.InstanceTypeVM {
		t.Fatal("incorrect instance creation")
	}
	for _, key := range []string{definition.OwnerKey, "features.images", "features.networks", "features.profiles", "features.storage.volumes"} {
		f := clientFixture()
		f.project.Config[key] = "changed"
		if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
			t.Fatalf("created after %s changed", key)
		}
	}
}

func TestPowerCallsAreGracefulAndWaited(t *testing.T) {
	for _, kind := range []reconcile.Kind{reconcile.StartVM, reconcile.StopVM} {
		f := clientFixture()
		from, action := "Stopped", "start"
		if kind == reconcile.StopVM {
			from, action = "Running", "stop"
			f.instance.Status = from
			f.state.Status = from
		}
		op := reconcile.Operation{Kind: kind, Project: "apps", Name: "wiki", FromState: from}
		if err := (Backend{Server: f}).Execute(op); err != nil {
			t.Fatal(err)
		}
		if f.selected != "apps" || f.statePut.Action != action || f.statePut.Force || f.statePut.Stateful || f.statePut.Timeout != 30 || f.etag != "state-etag" || f.op.waits != 1 {
			t.Fatalf("unsafe power operation: %+v", f.statePut)
		}
		f.writes = 0
		f.state.Status = "Frozen"
		if err := (Backend{Server: f}).Execute(op); err == nil || f.writes != 0 {
			t.Fatal("acted on changed power state")
		}
	}
	f := clientFixture()
	delete(f.instance.Config, definition.OwnerKey)
	if err := (Backend{Server: f}).Execute(reconcile.Operation{Kind: reconcile.StartVM, Project: "apps", Name: "wiki", FromState: "Stopped"}); err == nil || f.writes != 0 {
		t.Fatal("started unowned VM")
	}
}

func TestRequestAndWaitErrorsPropagate(t *testing.T) {
	want := errors.New("injected failure")
	for _, requestFails := range []bool{true, false} {
		f := clientFixture()
		if requestFails {
			f.requestError = want
		} else {
			f.op.err = want
		}
		err := (Backend{Server: f}).Execute(reconcile.Operation{Kind: reconcile.StartVM, Project: "apps", Name: "wiki", FromState: "Stopped"})
		if !errors.Is(err, want) {
			t.Fatalf("got %v", err)
		}
		if requestFails && f.op.waits != 0 {
			t.Fatal("waited after request failure")
		}
		if !requestFails && f.op.waits != 1 {
			t.Fatal("did not wait for operation")
		}
	}
}

type referenceClient struct {
	recordingClient
	pool       api.StoragePool
	network    api.Network
	image      api.Image
	imageReads int
	readError  error
}

func (f *referenceClient) UseProject(name string) incus.InstanceServer { f.selected = name; return f }
func (f *referenceClient) GetStoragePool(name string) (*api.StoragePool, string, error) {
	return &f.pool, "", f.readError
}
func (f *referenceClient) GetNetwork(name string) (*api.Network, string, error) {
	return &f.network, "", f.readError
}
func (f *referenceClient) GetImage(name string) (*api.Image, string, error) {
	f.imageReads++
	return &f.image, "", f.readError
}

func TestReferenceValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*referenceClient, *definition.VM)
		bad    bool
	}{
		{"valid", func(*referenceClient, *definition.VM) {}, false},
		{"missing", func(f *referenceClient, v *definition.VM) { f.readError = errors.New("missing reference") }, true},
		{"unready pool", func(f *referenceClient, v *definition.VM) { f.pool.Status = "Pending" }, true},
		{"unmanaged network", func(f *referenceClient, v *definition.VM) { f.network.Managed = false }, true},
		{"unsupported network", func(f *referenceClient, v *definition.VM) { f.network.Type = "ovn" }, true},
		{"unready network", func(f *referenceClient, v *definition.VM) { f.network.Status = "Pending" }, true},
		{"container image", func(f *referenceClient, v *definition.VM) { f.image.Type = "container" }, true},
		{"wrong fingerprint", func(f *referenceClient, v *definition.VM) { f.image.Fingerprint = "different" }, true},
		{"existing VM needs no cached seed", func(f *referenceClient, v *definition.VM) { v.Image = "" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &referenceClient{pool: api.StoragePool{Status: "Created"}, network: api.Network{Managed: true, Type: "bridge", Status: "Created"}, image: api.Image{Type: "virtual-machine", Fingerprint: "seed"}}
			v := definition.VM{Image: "seed", Pool: "pool", Network: "bridge"}
			test.change(f, &v)
			err := (Backend{Server: f}).CheckReferences(v)
			if (err != nil) != test.bad {
				t.Fatalf("unexpected result: %v", err)
			}
			if f.selected != "default" || f.writes != 0 {
				t.Fatal("reference validation mutated or used the wrong project")
			}
			if v.Image == "" && f.imageReads != 0 {
				t.Fatal("queried unnecessary seed image")
			}
		})
	}
}

type discoveryClient struct {
	recordingClient
	projects    []api.Project
	instances   map[string][]api.Instance
	reads       []string
	failProject string
}

func (f *discoveryClient) UseProject(name string) incus.InstanceServer { f.selected = name; return f }
func (f *discoveryClient) GetProjects() ([]api.Project, error) {
	f.reads = append(f.reads, "projects:"+f.selected)
	return f.projects, nil
}
func (f *discoveryClient) GetInstances(kind api.InstanceType) ([]api.Instance, error) {
	if kind != api.InstanceTypeAny {
		return nil, errors.New("must discover all instance types")
	}
	f.reads = append(f.reads, "instances:"+f.selected)
	if f.selected == f.failProject {
		return nil, errors.New("discovery failed")
	}
	return f.instances[f.selected], nil
}

func TestDiscoveryIncludesAllProjectsAndInstanceTypes(t *testing.T) {
	f := &discoveryClient{projects: []api.Project{{Name: "default"}, {Name: "apps"}}, instances: map[string][]api.Instance{"apps": {{Name: "wiki", Type: "container"}}}}
	s, err := (Backend{Server: f}).Discover()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.reads, []string{"projects:default", "instances:default", "instances:apps"}) || s.Instances["apps"][0].Type != "container" || f.writes != 0 {
		t.Fatal("incomplete or mutating discovery")
	}
	f.failProject = "apps"
	if _, err := (Backend{Server: f}).Discover(); err == nil {
		t.Fatal("ignored incomplete discovery")
	}
}

func (f *discoveryClient) GetStoragePoolNames() ([]string, error) { return nil, nil }

func (f *discoveryClient) GetNetworks() ([]api.Network, error) { return nil, nil }
