package reconcile

import (
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func findVolume(s Snapshot, pool, name string) *api.StorageVolume {
	for _, v := range s.Volumes[pool] {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func buildVolumeAdoption(g definition.Garden, target Target, b Backend) (Plan, error) {
	if err := g.Validate(); err != nil {
		return Plan{}, err
	}
	if err := target.validate(); err != nil {
		return Plan{}, err
	}
	for _, v := range g.Volumes {
		if v.Pool != target.Pool || v.Name != target.Name {
			continue
		}
		if err := b.CheckVolume(v); err != nil {
			return Plan{}, err
		}
		s, err := b.Discover()
		if err != nil {
			return Plan{}, err
		}
		actual := findVolume(s, v.Pool, v.Name)
		if actual == nil {
			return Plan{}, fmt.Errorf("volume %s does not exist", target)
		}
		if err := adoptionOwner(actual.Config); err != nil {
			return Plan{}, err
		}
		if _, _, manual := VolumeUpdate(v, *actual); manual != "" {
			return Plan{}, fmt.Errorf("cannot adopt volume %s: %s", target, manual)
		}
		if owned(actual.Config) {
			return Plan{}, nil
		}
		put := actual.Writable()
		put.Config = maps.Clone(put.Config)
		if put.Config == nil {
			put.Config = api.ConfigMap{}
		}
		put.Config[definition.OwnerKey] = definition.Owner
		return Plan{Operations: []Operation{{Kind: AdoptVolume, Pool: v.Pool, Name: v.Name, BeforeVolume: actual, VolumePut: put}}}, nil
	}
	return Plan{}, fmt.Errorf("volume %s must be declared before adoption", target)
}

// VolumeUpdate permits metadata changes and detached block growth only.
func VolumeUpdate(v definition.Volume, actual api.StorageVolume) (api.StorageVolumePut, []string, string) {
	put := actual.Writable()
	put.Config = maps.Clone(put.Config)
	if actual.Type != "custom" || actual.ContentType != v.ContentType {
		return put, nil, "content type differs; storage replacement requires manual intervention"
	}
	keys := patch(put.Config, v.Config)
	if v.Description != nil && put.Description != *v.Description {
		put.Description = *v.Description
		keys = append(keys, "description")
	}
	if v.ContentType == "block" {
		old, oldErr := definition.VolumeSizeBytes(actual.Config["size"])
		want, wantErr := definition.VolumeSizeBytes(v.Size)
		if oldErr != nil || wantErr != nil {
			return put, keys, "explicit valid current and desired block sizes are required"
		}
		if want < old {
			return put, keys, "shrinking persistent storage requires manual intervention"
		}
		if want > old {
			if len(actual.UsedBy) != 0 {
				return put, keys, "detach the volume before growing it"
			}
			put.Config["size"] = v.Size
			keys = append(keys, "size")
		}
	}
	return put, keys, ""
}

func planVolumes(g definition.Garden, s Snapshot, b Backend) (Plan, error) {
	var plan Plan
	declared := map[string]bool{}
	volumes := append([]definition.Volume(nil), g.Volumes...)
	sort.Slice(volumes, func(i, j int) bool {
		return definition.VolumeID(volumes[i].Pool, volumes[i].Name) < definition.VolumeID(volumes[j].Pool, volumes[j].Name)
	})
	for _, v := range volumes {
		id := definition.VolumeID(v.Pool, v.Name)
		declared[id] = true
		if err := b.CheckVolume(v); err != nil {
			return Plan{}, fmt.Errorf("volume %s references: %w", id, err)
		}
		actual := findVolume(s, v.Pool, v.Name)
		if actual == nil {
			config := api.ConfigMap{definition.OwnerKey: definition.Owner}
			patch(config, v.Config)
			if v.ContentType == "block" {
				config["size"] = v.Size
			}
			put := api.StorageVolumePut{Config: config}
			if v.Description != nil {
				put.Description = *v.Description
			}
			plan.Operations = append(plan.Operations, Operation{Kind: CreateVolume, Pool: v.Pool, Name: v.Name,
				VolumePost: api.StorageVolumesPost{Name: v.Name, Type: "custom", ContentType: v.ContentType, StorageVolumePut: put}})
			continue
		}
		if !owned(actual.Config) {
			return Plan{}, fmt.Errorf("volume %s is not owned by Garden; adopt it explicitly", id)
		}
		put, keys, manual := VolumeUpdate(v, *actual)
		if manual != "" {
			plan.Manual = append(plan.Manual, "volume "+id+": "+manual)
		} else if len(keys) != 0 {
			plan.Operations = append(plan.Operations, Operation{Kind: UpdateVolume, Pool: v.Pool, Name: v.Name, Keys: keys, BeforeVolume: actual, VolumePut: put})
		}
	}
	for pool, volumes := range s.Volumes {
		for _, v := range volumes {
			id := definition.VolumeID(pool, v.Name)
			if owned(v.Config) && !declared[id] {
				plan.Orphans = append(plan.Orphans, "volume "+id)
			}
		}
	}
	return plan, nil
}

func DiskDevice(d definition.Disk) map[string]string {
	device := map[string]string{"type": "disk", "pool": d.Pool, "source": d.Volume}
	if d.Path != "" {
		device["path"] = d.Path
	}
	return device
}

func SameDiskBinding(device map[string]string, disk definition.Disk) bool {
	for key, value := range DiskDevice(disk) {
		if device[key] != value {
			return false
		}
	}
	if disk.Path == "" && device["path"] != "" {
		return false
	}
	switch strings.ToLower(device["dependent"]) {
	case "", "false", "0", "off", "no":
		return true
	default:
		return false
	}
}

// VolumeUseAllowed prevents accidental multi-writer attachment, including uses
// by unmanaged resources or profiles. The target's existing attachment is allowed.
func VolumeUseAllowed(usedBy []string, project, name string) bool {
	for _, ref := range usedBy {
		u, err := url.Parse(ref)
		if err != nil || u.Path != "/1.0/instances/"+name {
			return false
		}
		actualProject := u.Query().Get("project")
		if actualProject == "" {
			actualProject = "default"
		}
		if actualProject != project {
			return false
		}
	}
	return true
}

func SameVolume(a, b api.StorageVolume) bool {
	au, bu := slices.Clone(a.UsedBy), slices.Clone(b.UsedBy)
	slices.Sort(au)
	slices.Sort(bu)
	return a.Name == b.Name && a.Type == b.Type && a.ContentType == b.ContentType &&
		reflect.DeepEqual(a.Writable(), b.Writable()) && slices.Equal(au, bu)
}
