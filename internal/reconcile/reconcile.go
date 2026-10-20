// Package reconcile plans explicit project and VM operations from Incus state.
package reconcile

import (
	"fmt"
	"io"
	"maps"
	"reflect"
	"sort"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

type Snapshot struct {
	Projects  []api.Project
	Instances map[string][]api.Instance
}

// Backend is Garden's operation boundary. Implementations wait for asynchronous
// operations and guard each mutation against stale state and ownership loss.
type Backend interface {
	Discover() (Snapshot, error)
	CheckReferences(definition.VM) error
	Execute(Operation) error
}

type Kind string

const (
	CreateProject Kind = "create project"
	UpdateProject Kind = "update project"
	CreateVM      Kind = "create VM"
	UpdateVM      Kind = "update VM"
	StartVM       Kind = "start VM"
	StopVM        Kind = "stop VM"
	AdoptProject  Kind = "adopt project"
	AdoptVM       Kind = "adopt VM"
	DeleteProject Kind = "delete project"
	DeleteVM      Kind = "delete VM"
)

type Operation struct {
	Kind           Kind
	Project, Name  string
	Keys           []string
	ProjectPut     api.ProjectPut
	InstancePost   api.InstancesPost
	InstancePut    api.InstancePut
	BeforeProject  *api.Project
	BeforeInstance *api.Instance
	FromState      string
}

func (op Operation) String() string {
	id := op.Name
	if op.Project != "" {
		id = op.Project + "/" + op.Name
	}
	if len(op.Keys) != 0 {
		return fmt.Sprintf("%s %s (%s)", op.Kind, id, strings.Join(op.Keys, ", "))
	}
	return fmt.Sprintf("%s %s", op.Kind, id)
}

type Plan struct {
	Operations []Operation
	Orphans    []string
	Manual     []string
}

// ProjectConfig shares existing default-project infrastructure. These feature
// choices are creation-only; Garden never changes project isolation in place.
func ProjectConfig() api.ConfigMap {
	return api.ConfigMap{
		definition.OwnerKey: definition.Owner,
		"features.images":   "false", "features.networks": "false",
		"features.profiles": "false", "features.storage.volumes": "false",
	}
}

func owned(config api.ConfigMap) bool { return config[definition.OwnerKey] == definition.Owner }

// Build validates the complete garden and all references before returning a
// plan. Manual actions block the entire apply, including otherwise safe work.
func Build(g definition.Garden, b Backend) (Plan, error) {
	var plan Plan
	if err := g.Validate(); err != nil {
		return plan, err
	}
	s, err := b.Discover()
	if err != nil {
		return plan, fmt.Errorf("discover Incus state: %w", err)
	}
	projects := map[string]api.Project{}
	for _, p := range s.Projects {
		projects[p.Name] = p
	}
	declaredProjects := map[string]bool{}
	declaredVMs := map[string]bool{}
	ps := append([]definition.Project(nil), g.Projects...)
	vs := append([]definition.VM(nil), g.VMs...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	sort.Slice(vs, func(i, j int) bool { return vs[i].Project+"/"+vs[i].Name < vs[j].Project+"/"+vs[j].Name })
	for _, p := range ps {
		declaredProjects[p.Name] = true
		actual, exists := projects[p.Name]
		if !exists {
			put := api.ProjectPut{Config: ProjectConfig()}
			if p.Description != nil {
				put.Description = *p.Description
			}
			plan.Operations = append(plan.Operations, Operation{Kind: CreateProject, Name: p.Name, ProjectPut: put})
			continue
		}
		if !owned(actual.Config) {
			return Plan{}, fmt.Errorf("project %s is not owned by Garden", p.Name)
		}
		for _, key := range []string{"features.images", "features.networks", "features.profiles", "features.storage.volumes"} {
			if actual.Config[key] != "false" {
				plan.Manual = append(plan.Manual, fmt.Sprintf("project %s: %s differs; project isolation changes require manual intervention", p.Name, key))
			}
		}
		if p.Description != nil && actual.Description != *p.Description {
			put := actual.Writable()
			put.Config = maps.Clone(put.Config)
			put.Description = *p.Description
			plan.Operations = append(plan.Operations, Operation{Kind: UpdateProject, Name: p.Name, Keys: []string{"description"}, ProjectPut: put, BeforeProject: &actual})
		}
	}
	for _, v := range vs {
		id := v.Project + "/" + v.Name
		declaredVMs[id] = true
		var actual *api.Instance
		for _, inst := range s.Instances[v.Project] {
			if inst.Name == v.Name {
				actual = &inst
				break
			}
		}
		if actual != nil && !owned(actual.Config) {
			return Plan{}, fmt.Errorf("instance %s is not owned by Garden", id)
		}
		// A seed image is needed only for creation. Existing VMs remain usable if
		// the cached seed is removed; pool/network references are always checked.
		reference := v
		if actual != nil {
			reference.Image = ""
		}
		if err := b.CheckReferences(reference); err != nil {
			return Plan{}, fmt.Errorf("VM %s references: %w", id, err)
		}
		if actual == nil {
			config := api.ConfigMap{definition.OwnerKey: definition.Owner}
			patch(config, v.Config)
			put := api.InstancePut{Config: config, Devices: api.DevicesMap{
				"root": {"type": "disk", "path": "/", "pool": v.Pool},
				"eth0": {"type": "nic", "name": "eth0", "network": v.Network},
			}, Profiles: []string{}}
			if v.Description != nil {
				put.Description = *v.Description
			}
			req := api.InstancesPost{InstancePut: put, Name: v.Name, Type: api.InstanceTypeVM, Source: api.InstanceSource{Type: "image", Fingerprint: v.Image, Project: "default"}}
			plan.Operations = append(plan.Operations, Operation{Kind: CreateVM, Project: v.Project, Name: v.Name, InstancePost: req})
			if v.Power == definition.Running {
				plan.Operations = append(plan.Operations, Operation{Kind: StartVM, Project: v.Project, Name: v.Name, FromState: "Stopped"})
			}
			continue
		}
		manual := compatibility(v, *actual)
		if len(manual) != 0 {
			plan.Manual = append(plan.Manual, manual...)
			continue
		}
		put := actual.Writable()
		put.Config = maps.Clone(put.Config)
		keys := patch(put.Config, v.Config)
		if v.Description != nil && put.Description != *v.Description {
			put.Description = *v.Description
			keys = append(keys, "description")
		}
		needsStopped := false
		for _, key := range keys {
			key = strings.TrimPrefix(key, "remove ")
			if key == "limits.cpu" || key == "limits.memory" {
				needsStopped = true
			}
		}
		if needsStopped && actual.Status != "Stopped" {
			plan.Manual = append(plan.Manual, fmt.Sprintf("VM %s: stop the VM explicitly before changing CPU or memory", id))
			continue
		}
		if len(keys) != 0 {
			plan.Operations = append(plan.Operations, Operation{Kind: UpdateVM, Project: v.Project, Name: v.Name, Keys: keys, InstancePut: put, BeforeInstance: actual})
		}
		if v.Power == definition.Running && actual.Status == "Stopped" {
			plan.Operations = append(plan.Operations, Operation{Kind: StartVM, Project: v.Project, Name: v.Name, FromState: "Stopped"})
		}
		if v.Power == definition.Stopped && actual.Status == "Running" {
			plan.Operations = append(plan.Operations, Operation{Kind: StopVM, Project: v.Project, Name: v.Name, FromState: "Running"})
		}
	}
	for _, p := range s.Projects {
		if owned(p.Config) && !declaredProjects[p.Name] {
			plan.Orphans = append(plan.Orphans, "project "+p.Name)
		}
		for _, v := range s.Instances[p.Name] {
			id := p.Name + "/" + v.Name
			if owned(v.Config) && !declaredVMs[id] {
				plan.Orphans = append(plan.Orphans, "instance "+id)
			}
		}
	}
	sort.Strings(plan.Orphans)
	sort.Strings(plan.Manual)
	return plan, nil
}

func patch(actual api.ConfigMap, desired definition.Properties) []string {
	var changed []string
	for _, key := range definition.Keys(desired) {
		value := desired[key]
		old, exists := actual[key]
		if value == nil {
			if exists {
				delete(actual, key)
				changed = append(changed, "remove "+key)
			}
		} else if !exists || old != *value {
			actual[key] = *value
			changed = append(changed, key)
		}
	}
	return changed
}

func compatibility(v definition.VM, actual api.Instance) []string {
	id := v.Project + "/" + v.Name
	var issues []string
	add := func(reason string) { issues = append(issues, "VM "+id+": "+reason+"; manual intervention required") }
	if actual.Type != string(api.InstanceTypeVM) {
		add("instance type differs")
	}
	if actual.Ephemeral {
		add("ephemeral instances are unsupported")
	}
	if actual.Stateful {
		add("saved-state instances are unsupported")
	}
	if len(actual.Profiles) != 0 {
		add("profiles are unsupported")
	}
	if actual.Status != "Running" && actual.Status != "Stopped" {
		add("instance is neither running nor stopped")
	}
	if actual.Config["volatile.base_image"] != v.Image {
		add("seed image differs; Garden will not rebuild or replace the VM")
	}
	root := actual.Devices["root"]
	if root["type"] != "disk" || root["path"] != "/" || root["pool"] != v.Pool {
		add("root disk placement differs")
	}
	nic := actual.Devices["eth0"]
	if nic["type"] != "nic" || nic["name"] != "eth0" || nic["network"] != v.Network {
		add("NIC placement differs")
	}
	return issues
}

func (p Plan) Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Plan: %d operations, %d orphans, %d manual actions\n", len(p.Operations), len(p.Orphans), len(p.Manual)); err != nil {
		return err
	}
	for _, op := range p.Operations {
		if _, err := fmt.Fprintln(w, "  "+op.String()); err != nil {
			return err
		}
	}
	for _, orphan := range p.Orphans {
		if _, err := fmt.Fprintln(w, "  orphan: "+orphan+" (preserved)"); err != nil {
			return err
		}
	}
	for _, manual := range p.Manual {
		if _, err := fmt.Fprintln(w, "  manual: "+manual); err != nil {
			return err
		}
	}
	return nil
}

// Apply writes the complete summary before mutation and stops on the first
// operation or output failure. It never rolls back completed operations.
func (p Plan) Apply(b Backend, w io.Writer) error {
	if err := p.Write(w); err != nil {
		return err
	}
	if len(p.Manual) != 0 {
		return fmt.Errorf("apply blocked by %d manual actions; no operations executed", len(p.Manual))
	}
	for i, op := range p.Operations {
		if err := b.Execute(op); err != nil {
			return fmt.Errorf("%s failed after %d completed operations: %w; fix the cause and rerun apply", op.String(), i, err)
		}
		if _, err := fmt.Fprintln(w, "Completed: "+op.String()); err != nil {
			return fmt.Errorf("report after %d completed operations: %w", i+1, err)
		}
	}
	_, err := fmt.Fprintf(w, "Apply complete: %d operations\n", len(p.Operations))
	return err
}

// Matches guard adapter updates against intervening edits. Writable preserves
// properties Garden does not manage; ETags then protect the write itself.
func SameProject(a, b api.Project) bool { return reflect.DeepEqual(a.Writable(), b.Writable()) }
func SameInstance(a, b api.Instance) bool {
	return reflect.DeepEqual(a.Writable(), b.Writable()) && a.Status == b.Status
}
