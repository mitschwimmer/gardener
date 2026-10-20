package reconcile

import (
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func TestAdoptionOnlyAddsOwner(t *testing.T) {
	for _, target := range []Target{{Name: "apps"}, {Project: "apps", Name: "wiki"}} {
		t.Run(target.String(), func(t *testing.T) {
			b := existingBackend(t)
			g := desired()
			var old api.ConfigMap
			if target.Project == "" {
				old = b.snapshot.Projects[0].Config
			} else {
				old = b.snapshot.Instances["apps"][0].Config
			}
			delete(old, definition.OwnerKey)
			old["user.external"] = "keep"
			before := maps.Clone(old)
			plan, err := BuildAdoption(g, target, b)
			if err != nil || len(plan.Operations) != 1 {
				t.Fatalf("adoption: %v %+v", err, plan)
			}
			op := plan.Operations[0]
			config, kind := op.ProjectPut.Config, AdoptProject
			if target.Project != "" {
				config, kind = op.InstancePut.Config, AdoptVM
				if len(b.checked) != 1 || b.checked[0].Image != "" {
					t.Fatal("existing VM adoption requires its cached seed")
				}
			}
			if op.Kind != kind || !reflect.DeepEqual(old, before) || config[definition.OwnerKey] != definition.Owner || len(config) != len(old)+1 {
				t.Fatalf("adoption changed more than ownership: %+v", op)
			}
			for key, value := range old {
				if config[key] != value {
					t.Fatalf("lost %s", key)
				}
			}
			if len(b.executed) != 0 {
				t.Fatal("adoption planning performed a write")
			}
		})
	}
}

func TestAdoptionRefusalsAndNoOp(t *testing.T) {
	vm := Target{Project: "apps", Name: "wiki"}
	for _, test := range []struct {
		name   string
		target Target
		change func(*recordingBackend)
	}{
		{"other project owner", Target{Name: "apps"}, func(b *recordingBackend) { b.snapshot.Projects[0].Config[definition.OwnerKey] = "other" }},
		{"unowned project", vm, func(b *recordingBackend) { delete(b.snapshot.Projects[0].Config, definition.OwnerKey) }},
		{"other VM owner", vm, func(b *recordingBackend) { b.snapshot.Instances["apps"][0].Config[definition.OwnerKey] = "other" }},
		{"incompatible project", Target{Name: "apps"}, func(b *recordingBackend) { b.snapshot.Projects[0].Config["features.images"] = "true" }},
		{"incompatible image", vm, func(b *recordingBackend) { b.snapshot.Instances["apps"][0].Config["volatile.base_image"] = "different" }},
		{"container", vm, func(b *recordingBackend) { b.snapshot.Instances["apps"][0].Type = "container" }},
		{"profiles", vm, func(b *recordingBackend) { b.snapshot.Instances["apps"][0].Profiles = []string{"default"} }},
		{"missing resource", vm, func(b *recordingBackend) { b.snapshot.Instances["apps"] = nil }},
		{"bad reference", vm, func(b *recordingBackend) { b.referenceError = errors.New("missing network") }},
		{"discovery failure", vm, func(b *recordingBackend) { b.discoveryError = errors.New("offline") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := existingBackend(t)
			test.change(b)
			if _, err := BuildAdoption(desired(), test.target, b); err == nil || len(b.executed) != 0 {
				t.Fatal("unsafe adoption accepted")
			}
		})
	}
	b := existingBackend(t)
	if _, err := BuildAdoption(definition.Garden{}, vm, b); err == nil {
		t.Fatal("adopted undeclared VM")
	}
	for _, target := range []Target{{Name: "apps"}, vm} {
		plan, err := BuildAdoption(desired(), target, b)
		if err != nil || len(plan.Operations) != 0 {
			t.Fatalf("already owned resource was not a no-op: %v %+v", err, plan)
		}
	}
}

func TestDeletionRequiresOwnedOrphanAndSafeLifecycle(t *testing.T) {
	vm := Target{Project: "apps", Name: "wiki"}
	b := existingBackend(t)
	b.snapshot.Instances["apps"][0].Status = "Stopped"
	plan, err := BuildDeletion(definition.Garden{}, vm, b)
	if err != nil || len(plan.Operations) != 1 || plan.Operations[0].Kind != DeleteVM || len(b.executed) != 0 {
		t.Fatalf("delete plan: %v %+v", err, plan)
	}
	if _, err := BuildDeletion(desired(), vm, b); err == nil {
		t.Fatal("declared VM deletion accepted")
	}
	if _, err := BuildDeletion(definition.Garden{}, Target{Name: "apps"}, b); err == nil {
		t.Fatal("nonempty project deletion accepted")
	}
	for _, test := range []struct {
		name   string
		change func(*api.Instance)
	}{
		{"unowned", func(v *api.Instance) { delete(v.Config, definition.OwnerKey) }},
		{"running", func(v *api.Instance) { v.Status = "Running" }},
		{"saved state", func(v *api.Instance) { v.Stateful = true }},
		{"ephemeral", func(v *api.Instance) { v.Ephemeral = true }},
		{"container", func(v *api.Instance) { v.Type = "container" }},
		{"dependent disk", func(v *api.Instance) {
			v.Devices["data"] = map[string]string{"type": "disk", "pool": "pool", "source": "data", "path": "/data", "dependent": "true"}
		}},
		{"inherited dependent disk", func(v *api.Instance) {
			v.ExpandedDevices = api.DevicesMap{"data": {"type": "disk", "pool": "pool", "source": "data", "dependent": "1"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := existingBackend(t)
			b.snapshot.Instances["apps"][0].Status = "Stopped"
			test.change(&b.snapshot.Instances["apps"][0])
			if _, err := BuildDeletion(definition.Garden{}, vm, b); err == nil {
				t.Fatal("unsafe deletion accepted")
			}
		})
	}
	b.snapshot.Instances["apps"] = nil
	plan, err = BuildDeletion(definition.Garden{}, Target{Name: "apps"}, b)
	if err != nil || plan.Operations[0].Kind != DeleteProject {
		t.Fatalf("empty project refused: %v", err)
	}
	b.snapshot.Projects[0].UsedBy = []string{"/1.0/profiles/important"}
	if _, err := BuildDeletion(definition.Garden{}, Target{Name: "apps"}, b); err == nil {
		t.Fatal("project with non-instance resources accepted")
	}
}

func TestDefaultProjectAndInvalidDefinitionsCannotBeLifecycleTargets(t *testing.T) {
	for _, target := range []Target{{Name: "default"}, {Project: "default", Name: "wiki"}, {}} {
		if _, err := BuildDeletion(definition.Garden{}, target, emptyBackend()); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
	g := desired()
	g.Projects = append(g.Projects, g.Projects[0])
	if _, err := BuildAdoption(g, Target{Name: "apps"}, existingBackend(t)); err == nil {
		t.Fatal("invalid definitions accepted")
	}
}
