package reconcile

import (
	"maps"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

// VMUpdate preserves all undeclared properties, including pending restart state.
func VMUpdate(v definition.VM, actual api.Instance) (api.InstancePut, []string) {
	put := actual.Writable()
	put.Config = maps.Clone(put.Config)
	keys := patch(put.Config, v.Config)
	put.Devices = maps.Clone(put.Devices)
	if put.Devices == nil {
		put.Devices = api.DevicesMap{}
	}
	for _, disk := range v.Disks {
		if _, exists := put.Devices[disk.Name]; !exists {
			put.Devices[disk.Name] = DiskDevice(disk)
			keys = append(keys, "disk "+disk.Name)
		}
	}
	if v.Description != nil && put.Description != *v.Description {
		put.Description = *v.Description
		keys = append(keys, "description")
	}
	return put, keys
}

func NeedsStopped(keys []string) bool {
	for _, key := range keys {
		key = strings.TrimPrefix(key, "remove ")
		if key == "limits.cpu" || key == "limits.memory" || strings.HasPrefix(key, "disk ") {
			return true
		}
	}
	return false
}

// SameReconfigurationState allows Incus's volatile runtime bookkeeping to change
// during Garden's own power operations while protecting all writable user state.
func SameReconfigurationState(a, b api.Instance) bool {
	if a.Config["volatile.base_image"] != b.Config["volatile.base_image"] ||
		b.Config["volatile.uuid"] != "" && a.Config["volatile.uuid"] != b.Config["volatile.uuid"] {
		return false
	}
	stable := func(v api.Instance) api.Instance {
		v.Config = maps.Clone(v.Config)
		for key := range v.Config {
			if strings.HasPrefix(key, "volatile.") {
				delete(v.Config, key)
			}
		}
		return v
	}
	return a.Type == b.Type && SameInstance(stable(a), stable(b))
}
