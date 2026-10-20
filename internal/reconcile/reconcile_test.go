package reconcile

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

type recordingBackend struct {
	snapshot                       Snapshot
	executed                       []Operation
	checked                        []definition.VM
	discoveries                    int
	failAt                         int
	referenceError, discoveryError error
}

func (f *recordingBackend) Discover() (Snapshot, error) {
	f.discoveries++
	return f.snapshot, f.discoveryError
}
func (f *recordingBackend) CheckReferences(v definition.VM) error {
	f.checked = append(f.checked, v)
	return f.referenceError
}
func (f *recordingBackend) CheckVolume(definition.Volume) error { return f.referenceError }
func (f *recordingBackend) Execute(op Operation) error {
	f.executed = append(f.executed, op)
	if f.failAt == len(f.executed) {
		return errors.New("injected operation failure")
	}
	switch op.Kind {
	case CreateNetwork:
		f.snapshot.Networks = append(f.snapshot.Networks, api.Network{Name: op.Name, Type: "bridge", Managed: true, Status: "Created", NetworkPut: op.NetworkPost.NetworkPut})
	case UpdateNetwork, AdoptNetwork:
		for i := range f.snapshot.Networks {
			if f.snapshot.Networks[i].Name == op.Name {
				f.snapshot.Networks[i].NetworkPut = op.NetworkPut
			}
		}
	case CreateVolume:
		f.snapshot.Volumes[op.Pool] = append(f.snapshot.Volumes[op.Pool], api.StorageVolume{Name: op.Name, Type: "custom", ContentType: op.VolumePost.ContentType, StorageVolumePut: op.VolumePost.StorageVolumePut})
	case UpdateVolume, AdoptVolume:
		for i := range f.snapshot.Volumes[op.Pool] {
			if f.snapshot.Volumes[op.Pool][i].Name == op.Name {
				f.snapshot.Volumes[op.Pool][i].StorageVolumePut = op.VolumePut
			}
		}
	case CreateProject:
		f.snapshot.Projects = append(f.snapshot.Projects, api.Project{Name: op.Name, ProjectPut: op.ProjectPut})
	case UpdateProject:
		for i := range f.snapshot.Projects {
			if f.snapshot.Projects[i].Name == op.Name {
				f.snapshot.Projects[i].ProjectPut = op.ProjectPut
			}
		}
	case CreateVM:
		put := op.InstancePost.InstancePut
		put.Config = maps.Clone(put.Config)
		put.Config["volatile.base_image"] = op.InstancePost.Source.Fingerprint
		f.snapshot.Instances[op.Project] = append(f.snapshot.Instances[op.Project], api.Instance{Name: op.Name, Type: string(op.InstancePost.Type), Status: "Stopped", InstancePut: put})
	case UpdateVM, StartVM, StopVM:
		for i := range f.snapshot.Instances[op.Project] {
			v := &f.snapshot.Instances[op.Project][i]
			if v.Name != op.Name {
				continue
			}
			switch op.Kind {
			case UpdateVM:
				v.InstancePut = op.InstancePut
			case StartVM:
				v.Status = "Running"
			case StopVM:
				v.Status = "Stopped"
			}
		}
	}
	return nil
}

func desired() definition.Garden {
	return definition.Garden{Projects: []definition.Project{{Name: "apps"}}, VMs: []definition.VM{{Project: "apps", Name: "wiki", Image: strings.Repeat("a", 64), Pool: "pool", Network: "bridge", Power: definition.Running}}}
}

func emptyBackend() *recordingBackend {
	return &recordingBackend{snapshot: Snapshot{Instances: map[string][]api.Instance{}, Volumes: map[string][]api.StorageVolume{}}}
}

func existingBackend(t *testing.T) *recordingBackend {
	t.Helper()
	f := emptyBackend()
	p, err := Build(desired(), f)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	f.executed = nil
	f.checked = nil
	f.discoveries = 0
	return f
}

func kinds(p Plan) []Kind {
	result := make([]Kind, len(p.Operations))
	for i, op := range p.Operations {
		result[i] = op.Kind
	}
	return result
}

func TestCreateResumeAndIdempotency(t *testing.T) {
	f := emptyBackend()
	g := desired()
	p, err := Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.executed) != 0 {
		t.Fatal("planning mutated infrastructure")
	}
	if !reflect.DeepEqual(kinds(p), []Kind{CreateProject, CreateVM, StartVM}) {
		t.Fatalf("unexpected phases: %v", kinds(p))
	}
	create := p.Operations[1].InstancePost
	if create.Type != api.InstanceTypeVM || create.Profiles == nil || len(create.Profiles) != 0 || create.Ephemeral || create.Source.Project != "default" {
		t.Fatalf("unsafe creation request: %+v", create)
	}
	f.failAt = 3
	var output bytes.Buffer
	if err := p.Apply(f, &output); err == nil || !strings.Contains(err.Error(), "2 completed operations") {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(output.String(), "Completed: create VM apps/wiki") {
		t.Fatal(output.String())
	}
	if len(f.snapshot.Projects) != 1 || len(f.snapshot.Instances["apps"]) != 1 {
		t.Fatal("successful operations were not retained")
	}
	f.failAt = 0
	f.executed = nil
	p, err = Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(p), []Kind{StartVM}) {
		t.Fatalf("resume plan: %v", kinds(p))
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	p, err = Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Operations) != 0 || len(p.Manual) != 0 {
		t.Fatalf("not idempotent: %+v", p)
	}
	if f.checked[len(f.checked)-1].Image != "" {
		t.Fatal("existing VM unnecessarily depends on cached image")
	}
}

func TestValidationAndOwnershipFailBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*definition.Garden, *recordingBackend)
	}{
		{"invalid definition", func(g *definition.Garden, f *recordingBackend) { g.VMs[0].Project = "unknown" }},
		{"unmanaged project", func(g *definition.Garden, f *recordingBackend) {
			delete(f.snapshot.Projects[0].Config, definition.OwnerKey)
		}},
		{"unmanaged instance", func(g *definition.Garden, f *recordingBackend) {
			delete(f.snapshot.Instances["apps"][0].Config, definition.OwnerKey)
		}},
		{"missing reference", func(g *definition.Garden, f *recordingBackend) { f.referenceError = errors.New("missing network") }},
		{"discovery failure", func(g *definition.Garden, f *recordingBackend) { f.discoveryError = errors.New("connection failure") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := desired()
			f := existingBackend(t)
			test.change(&g, f)
			if _, err := Build(g, f); err == nil {
				t.Fatal("expected failure")
			}
			if len(f.executed) != 0 {
				t.Fatal("mutated during preflight")
			}
			if test.name == "invalid definition" && f.discoveries != 0 {
				t.Fatal("invalid definition contacted Incus")
			}
		})
	}
}

func TestPreserveAndRemoveDeclaredProperties(t *testing.T) {
	f := existingBackend(t)
	g := desired()
	v := &f.snapshot.Instances["apps"][0]
	v.Config["user.external"] = "keep"
	v.Config["user.obsolete"] = "remove"
	v.Config["volatile.id"] = "keep"
	v.Devices["data"] = map[string]string{"type": "disk", "source": "persistent"}
	v.Description = "keep VM description"
	f.snapshot.Projects[0].Config["user.external"] = "keep project metadata"
	g.Projects[0].Description = definition.Value("new project description")
	g.VMs[0].Config = definition.Properties{"user.obsolete": nil, "user.note": definition.Value("new")}
	beforeConfig := maps.Clone(v.Config)
	p, err := Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeConfig, v.Config) {
		t.Fatal("plan changed discovered state")
	}
	if !reflect.DeepEqual(kinds(p), []Kind{UpdateProject, UpdateVM}) {
		t.Fatalf("plan: %v", kinds(p))
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	v = &f.snapshot.Instances["apps"][0]
	if _, ok := v.Config["user.obsolete"]; ok {
		t.Fatal("explicit removal was ignored")
	}
	if v.Config["user.external"] != "keep" || v.Config["volatile.id"] != "keep" || v.Config["user.note"] != "new" || v.Devices["data"]["source"] != "persistent" || v.Description != "keep VM description" {
		t.Fatalf("unspecified properties were lost: %+v", v)
	}
	if f.snapshot.Projects[0].Config["user.external"] != "keep project metadata" {
		t.Fatal("project metadata lost")
	}
	p, err = Build(g, f)
	if err != nil || len(p.Operations) != 0 {
		t.Fatalf("not idempotent: %+v %v", p, err)
	}
	g.VMs[0].Description = definition.Value("")
	p, err = Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Operations) != 1 || p.Operations[0].InstancePut.Description != "" {
		t.Fatal("explicit description removal ignored")
	}
}

func TestManualChangesBlockEntireApply(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*definition.Garden, *recordingBackend)
	}{
		{"invalid recovery marker", func(g *definition.Garden, f *recordingBackend) {
			f.snapshot.Instances["apps"][0].Config[definition.PendingPowerKey] = "unknown"
		}},
		{"seed change", func(g *definition.Garden, f *recordingBackend) { g.VMs[0].Image = strings.Repeat("b", 64) }},
		{"pool change", func(g *definition.Garden, f *recordingBackend) { g.VMs[0].Pool = "other" }},
		{"network change", func(g *definition.Garden, f *recordingBackend) { g.VMs[0].Network = "other" }},
		{"container collision", func(g *definition.Garden, f *recordingBackend) { f.snapshot.Instances["apps"][0].Type = "container" }},
		{"ephemeral", func(g *definition.Garden, f *recordingBackend) { f.snapshot.Instances["apps"][0].Ephemeral = true }},
		{"saved state", func(g *definition.Garden, f *recordingBackend) { f.snapshot.Instances["apps"][0].Stateful = true }},
		{"profiles", func(g *definition.Garden, f *recordingBackend) {
			f.snapshot.Instances["apps"][0].Profiles = []string{"default"}
		}},
		{"transient state", func(g *definition.Garden, f *recordingBackend) { f.snapshot.Instances["apps"][0].Status = "Starting" }},
		{"project isolation", func(g *definition.Garden, f *recordingBackend) {
			f.snapshot.Projects[0].Config["features.networks"] = "true"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := desired()
			f := existingBackend(t)
			g.Projects = append(g.Projects, definition.Project{Name: "new-project"})
			test.change(&g, f)
			p, err := Build(g, f)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Manual) == 0 {
				t.Fatal("missing manual action")
			}
			if err := p.Apply(f, io.Discard); err == nil {
				t.Fatal("apply should be blocked")
			}
			if len(f.executed) != 0 {
				t.Fatal("executed safe subset of blocked plan")
			}
		})
	}
}

func TestStoppedUpdateStartRecovery(t *testing.T) {
	f := existingBackend(t)
	g := desired()
	f.snapshot.Instances["apps"][0].Status = "Stopped"
	g.VMs[0].Config = definition.Properties{"limits.cpu": definition.Value("4"), "limits.memory": definition.Value("4GiB")}
	p, err := Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(p), []Kind{UpdateVM, StartVM}) || len(p.Manual) != 0 {
		t.Fatalf("plan: %+v", p)
	}
	f.failAt = 2
	if err := p.Apply(f, io.Discard); err == nil {
		t.Fatal("expected start failure")
	}
	f.failAt = 0
	f.executed = nil
	p, err = Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(p), []Kind{StartVM}) {
		t.Fatalf("lost pending power intent: %v", kinds(p))
	}
}

func TestOrphansArePreservedAndIdentitiesIncludeProject(t *testing.T) {
	f := existingBackend(t)
	f.snapshot.Projects = append(f.snapshot.Projects, api.Project{Name: "other", ProjectPut: api.ProjectPut{Config: ProjectConfig()}})
	v := f.snapshot.Instances["apps"][0]
	f.snapshot.Instances["other"] = []api.Instance{v}
	p, err := Build(desired(), f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Orphans, []string{"instance other/wiki", "project other"}) {
		t.Fatalf("orphans: %v", p.Orphans)
	}
	p, err = Build(definition.Garden{}, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Orphans) != 4 || len(p.Operations) != 0 {
		t.Fatalf("empty garden: %+v", p)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(f.executed) != 0 {
		t.Fatal("orphan mutation")
	}
}

func TestPowerIsOptionalAndStopsExplicitly(t *testing.T) {
	f := existingBackend(t)
	g := desired()
	g.VMs[0].Power = definition.Unmanaged
	f.snapshot.Instances["apps"][0].Status = "Stopped"
	p, err := Build(g, f)
	if err != nil || len(p.Operations) != 0 {
		t.Fatalf("unmanaged power: %+v %v", p, err)
	}
	f.snapshot.Instances["apps"][0].Status = "Running"
	g.VMs[0].Power = definition.Stopped
	p, err = Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(p), []Kind{StopVM}) {
		t.Fatalf("stop plan: %v", kinds(p))
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestSummaryFailurePreventsMutation(t *testing.T) {
	f := emptyBackend()
	p, err := Build(desired(), f)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(f, brokenWriter{}); err == nil {
		t.Fatal("expected output failure")
	}
	if len(f.executed) != 0 {
		t.Fatal("mutated without summary")
	}
}

func TestFirstFailureStopsRemainingWork(t *testing.T) {
	for failAt := 1; failAt <= 3; failAt++ {
		f := emptyBackend()
		f.failAt = failAt
		p, err := Build(desired(), f)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Apply(f, io.Discard); err == nil {
			t.Fatal("expected injected failure")
		}
		if len(f.executed) != failAt {
			t.Fatalf("continued after operation %d: %d calls", failAt, len(f.executed))
		}
	}
}
