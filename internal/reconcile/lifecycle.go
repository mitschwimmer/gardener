package reconcile

import (
	"fmt"
	"maps"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

// Target identifies a network, volume in Pool, VM in Project, or project.
type Target struct {
	Project, Pool, Name string
	Network             bool
}

func (t Target) String() string {
	if t.Pool != "" {
		return t.Pool + "/" + t.Name
	}
	if t.Project == "" {
		return t.Name
	}
	return t.Project + "/" + t.Name
}

func (t Target) validate() error {
	if t.Network {
		if t.Name == "" || t.Pool != "" || t.Project != "" {
			return fmt.Errorf("a network needs only its name")
		}
		return nil
	}
	if t.Pool != "" {
		if t.Name == "" || t.Project != "" {
			return fmt.Errorf("a volume needs a pool and name")
		}
		return nil
	}
	if t.Name == "" || t.Name == "default" && t.Project == "" || t.Project == "default" {
		return fmt.Errorf("a named non-default project or VM is required")
	}
	return nil
}

func declared(g definition.Garden, target Target) bool {
	if target.Project == "" {
		for _, p := range g.Projects {
			if p.Name == target.Name {
				return true
			}
		}
	} else {
		for _, v := range g.VMs {
			if v.Project == target.Project && v.Name == target.Name {
				return true
			}
		}
	}
	return false
}

func lifecycleSnapshot(g definition.Garden, target Target, b Backend) (Snapshot, api.Project, *api.Instance, error) {
	if err := g.Validate(); err != nil {
		return Snapshot{}, api.Project{}, nil, err
	}
	if err := target.validate(); err != nil {
		return Snapshot{}, api.Project{}, nil, err
	}
	s, err := b.Discover()
	if err != nil {
		return s, api.Project{}, nil, fmt.Errorf("discover Incus state: %w", err)
	}
	project := target.Project
	if project == "" {
		project = target.Name
	}
	for _, p := range s.Projects {
		if p.Name != project {
			continue
		}
		if target.Project == "" {
			return s, p, nil, nil
		}
		for _, v := range s.Instances[project] {
			if v.Name == target.Name {
				return s, p, &v, nil
			}
		}
		return s, p, nil, fmt.Errorf("VM %s does not exist", target)
	}
	return s, api.Project{}, nil, fmt.Errorf("project %s does not exist", project)
}

func adoptionOwner(config api.ConfigMap) error {
	if owner := config[definition.OwnerKey]; owner != "" && owner != definition.Owner {
		return fmt.Errorf("resource has a different owner %q", owner)
	}
	if !owned(config) && config[definition.PendingPowerKey] != "" {
		return fmt.Errorf("unowned resource has pending Garden power intent; resolve it before adoption")
	}
	return nil
}

// BuildAdoption validates one declared resource and changes only its owner marker.
// Other unadopted resources do not prevent adopting the garden one target at a time.
func BuildAdoption(g definition.Garden, target Target, b Backend) (Plan, error) {
	if target.Network {
		return buildNetworkAdoption(g, target, b)
	}
	if target.Pool != "" {
		return buildVolumeAdoption(g, target, b)
	}
	if !declared(g, target) {
		return Plan{}, fmt.Errorf("%s must be declared before adoption", target)
	}
	_, p, instance, err := lifecycleSnapshot(g, target, b)
	if err != nil {
		return Plan{}, err
	}
	for _, key := range []string{"features.images", "features.networks", "features.profiles", "features.storage.volumes"} {
		if p.Config[key] != "false" {
			return Plan{}, fmt.Errorf("project %s: %s must be false before adoption", p.Name, key)
		}
	}
	if instance == nil {
		if err := adoptionOwner(p.Config); err != nil {
			return Plan{}, err
		}
		if owned(p.Config) {
			return Plan{}, nil
		}
		put := p.Writable()
		put.Config = maps.Clone(put.Config)
		put.Config[definition.OwnerKey] = definition.Owner
		return Plan{Operations: []Operation{{Kind: AdoptProject, Name: p.Name, BeforeProject: &p, ProjectPut: put}}}, nil
	}
	if !owned(p.Config) {
		return Plan{}, fmt.Errorf("adopt project %s before adopting its VMs", p.Name)
	}
	if err := adoptionOwner(instance.Config); err != nil {
		return Plan{}, err
	}
	for _, v := range g.VMs {
		if v.Project != target.Project || v.Name != target.Name {
			continue
		}
		if issues := compatibility(v, *instance); len(issues) != 0 {
			return Plan{}, fmt.Errorf("cannot adopt: %s", strings.Join(issues, "; "))
		}
		v.Image = "" // Existing VMs need their original fingerprint, not a cached seed.
		if err := b.CheckReferences(v); err != nil {
			return Plan{}, fmt.Errorf("VM %s references: %w", target, err)
		}
	}
	if owned(instance.Config) {
		return Plan{}, nil
	}
	put := instance.Writable()
	put.Config = maps.Clone(put.Config)
	put.Config[definition.OwnerKey] = definition.Owner
	return Plan{Operations: []Operation{{Kind: AdoptVM, Project: target.Project, Name: target.Name,
		BeforeProject: &p, BeforeInstance: instance, InstancePut: put}}}, nil
}

// BuildDeletion accepts only owned resources absent from the compiled definitions.
// Confirmation belongs to the CLI; apply never builds these operations implicitly.
func BuildDeletion(g definition.Garden, target Target, b Backend) (Plan, error) {
	if target.Network {
		return Plan{}, fmt.Errorf("network deletion requires an explicit external procedure")
	}
	if target.Pool != "" {
		return Plan{}, fmt.Errorf("volume deletion requires an explicit external procedure")
	}
	if declared(g, target) {
		return Plan{}, fmt.Errorf("remove %s from the compiled definitions before deletion", target)
	}
	s, p, instance, err := lifecycleSnapshot(g, target, b)
	if err != nil {
		return Plan{}, err
	}
	if !owned(p.Config) {
		return Plan{}, fmt.Errorf("project %s is not owned by Garden", p.Name)
	}
	if instance == nil {
		if len(s.Instances[p.Name]) != 0 || len(p.UsedBy) != 0 {
			return Plan{}, fmt.Errorf("project %s is not empty; delete or move its resources explicitly first", p.Name)
		}
		return Plan{Operations: []Operation{{Kind: DeleteProject, Name: p.Name, BeforeProject: &p}}}, nil
	}
	if err := ValidateVMDeletion(*instance); err != nil {
		return Plan{}, fmt.Errorf("cannot delete VM %s: %w", target, err)
	}
	return Plan{Operations: []Operation{{Kind: DeleteVM, Project: target.Project, Name: target.Name,
		BeforeProject: &p, BeforeInstance: instance}}}, nil
}

// ValidateVMDeletion is shared by discovery and the last check before deletion.
// Incus may delete dependent custom disks along with an instance. Refuse those
// attachments rather than silently weakening Garden's volume preservation rule.
func ValidateVMDeletion(instance api.Instance) error {
	if !owned(instance.Config) {
		return fmt.Errorf("instance is not owned by Garden")
	}
	if instance.Type != string(api.InstanceTypeVM) || instance.Status != "Stopped" || instance.Ephemeral || instance.Stateful {
		return fmt.Errorf("a stopped, non-ephemeral VM without saved state is required")
	}
	for _, devices := range []api.DevicesMap{instance.Devices, instance.ExpandedDevices} {
		for name, disk := range devices {
			dependent := strings.ToLower(disk["dependent"])
			if disk["type"] == "disk" && disk["path"] != "/" && disk["pool"] != "" &&
				dependent != "" && dependent != "false" && dependent != "0" && dependent != "no" && dependent != "off" {
				return fmt.Errorf("disk %s is dependent; preserve its volume explicitly before deleting the VM", name)
			}
		}
	}
	return nil
}
