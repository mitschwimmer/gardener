package definition

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Volume is an independent custom volume in Incus's shared default project.
// Size is required for block volumes; filesystem quotas are not managed yet.
type Volume struct {
	Pool, Name, ContentType string
	Size                    string
	Description             *string
	Config                  Properties
}

// Disk adds a volume device. Path is absolute for filesystems and empty for block.
// Existing bindings are never replaced or detached implicitly.
type Disk struct{ Name, Pool, Volume, Path string }

func VolumeID(pool, name string) string { return pool + "/" + name }

// VolumeSizeBytes also accepts Incus's decimal byte representation for comparison.
func VolumeSizeBytes(value string) (uint64, error) {
	factor := uint64(1)
	if strings.HasSuffix(value, "MiB") {
		factor, value = 1<<20, strings.TrimSuffix(value, "MiB")
	} else if strings.HasSuffix(value, "GiB") {
		factor, value = 1<<30, strings.TrimSuffix(value, "GiB")
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || n == 0 || n > uint64(1<<63-1)/factor {
		return 0, fmt.Errorf("invalid positive volume size")
	}
	return n * factor, nil
}

func (g Garden) validateVolumes() error {
	volumes := map[string]Volume{}
	for _, v := range g.Volumes {
		id := VolumeID(v.Pool, v.Name)
		if v.Pool == "" || !namePattern.MatchString(v.Name) {
			return fmt.Errorf("volume %s needs a pool and valid name", id)
		}
		if _, exists := volumes[id]; exists {
			return fmt.Errorf("duplicate volume %s", id)
		}
		if v.ContentType != "filesystem" && v.ContentType != "block" {
			return fmt.Errorf("volume %s needs filesystem or block content type", id)
		}
		if v.ContentType == "block" {
			if _, err := VolumeSizeBytes(v.Size); err != nil || !memoryPattern.MatchString(v.Size) {
				return fmt.Errorf("block volume %s needs a positive MiB or GiB size", id)
			}
		} else if v.Size != "" {
			return fmt.Errorf("filesystem volume %s quotas are not managed", id)
		}
		for _, key := range Keys(v.Config) {
			if !strings.HasPrefix(key, "user.") || len(key) == len("user.") || strings.HasPrefix(key, "user.garden.") {
				return fmt.Errorf("volume %s cannot manage config key %q", id, key)
			}
		}
		volumes[id] = v
	}
	attached := map[string]bool{}
	for _, vm := range g.VMs {
		names, paths := map[string]bool{}, map[string]bool{}
		for _, disk := range vm.Disks {
			id := VolumeID(disk.Pool, disk.Volume)
			volume, exists := volumes[id]
			if !exists {
				return fmt.Errorf("VM %s/%s references undeclared volume %s", vm.Project, vm.Name, id)
			}
			if !namePattern.MatchString(disk.Name) || disk.Name == "root" || disk.Name == "eth0" || names[disk.Name] {
				return fmt.Errorf("VM %s/%s has an invalid, reserved, or duplicate disk name", vm.Project, vm.Name)
			}
			names[disk.Name] = true
			if attached[id] {
				return fmt.Errorf("volume %s may be attached only once in the garden", id)
			}
			attached[id] = true
			if volume.ContentType == "block" && disk.Path != "" || volume.ContentType == "filesystem" &&
				(!path.IsAbs(disk.Path) || path.Clean(disk.Path) != disk.Path || disk.Path == "/" || paths[disk.Path]) {
				return fmt.Errorf("disk %s needs an empty block path or unique absolute non-root filesystem path", disk.Name)
			}
			paths[disk.Path] = true
		}
	}
	return nil
}
