package reconcile

import (
	"fmt"
	"maps"
	"net/netip"
	"reflect"
	"sort"
	"strconv"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func desiredNetwork(g definition.Garden, name string) *definition.Network {
	for _, n := range g.Networks {
		if n.Name == name {
			return &n
		}
	}
	return nil
}
func findNetwork(s Snapshot, name string) *api.Network {
	for _, n := range s.Networks {
		if n.Name == name {
			return &n
		}
	}
	return nil
}
func NetworkConfig(n definition.Network) api.ConfigMap {
	canonical := func(s string) string {
		if p, err := netip.ParsePrefix(s); err == nil {
			return p.String()
		}
		return s
	}
	return api.ConfigMap{"bridge.driver": "native", "ipv4.address": canonical(n.IPv4Address), "ipv6.address": canonical(n.IPv6Address), "ipv4.nat": strconv.FormatBool(n.IPv4NAT), "ipv6.nat": strconv.FormatBool(n.IPv6NAT)}
}

func NetworkCompatibility(n definition.Network, actual api.Network) string {
	if !actual.Managed || actual.Type != "bridge" || actual.Status != "Created" {
		return "a ready managed bridge is required"
	}
	if actual.Config["bridge.external_interfaces"] != "" || actual.Config["bridge.mode"] != "" && actual.Config["bridge.mode"] != "standard" {
		return "external interfaces and nonstandard bridge modes require manual management"
	}
	config := NetworkConfig(n)
	for _, key := range []string{"bridge.driver", "ipv4.address", "ipv4.nat", "ipv6.address", "ipv6.nat"} {
		want := config[key]
		got := actual.Config[key]
		if key == "bridge.driver" && got == "" {
			got = "native"
		}
		if key == "ipv4.nat" || key == "ipv6.nat" {
			if got == "" {
				got = "false"
			}
			if value, err := strconv.ParseBool(got); err == nil {
				got = strconv.FormatBool(value)
			}
		}
		if p, err := netip.ParsePrefix(got); err == nil {
			got = p.String()
		}
		if got != want {
			return key + " differs; address, NAT, and driver changes require manual intervention"
		}
	}
	return ""
}
func SameNetwork(a, b api.Network) bool {
	return a.Name == b.Name && a.Managed == b.Managed && a.Type == b.Type && a.Status == b.Status && reflect.DeepEqual(a.Writable(), b.Writable())
}

func planNetworks(g definition.Garden, s Snapshot) (Plan, error) {
	var p Plan
	networks := append([]definition.Network(nil), g.Networks...)
	sort.Slice(networks, func(i, j int) bool { return networks[i].Name < networks[j].Name })
	for _, n := range networks {
		actual := findNetwork(s, n.Name)
		if actual == nil {
			// Refuse a new bridge subnet overlapping another discoverable managed net.
			for _, other := range s.Networks {
				for _, key := range []string{"ipv4.address", "ipv6.address"} {
					want, e1 := netip.ParsePrefix(NetworkConfig(n)[key])
					have, e2 := netip.ParsePrefix(other.Config[key])
					if e1 == nil && e2 == nil && want.Overlaps(have) {
						return Plan{}, fmt.Errorf("network %s overlaps existing network %s", n.Name, other.Name)
					}
				}
			}
			config := NetworkConfig(n)
			config[definition.OwnerKey] = definition.Owner
			patch(config, n.Config)
			put := api.NetworkPut{Config: config}
			if n.Description != nil {
				put.Description = *n.Description
			}
			p.Operations = append(p.Operations, Operation{Kind: CreateNetwork, Name: n.Name, NetworkPost: api.NetworksPost{Name: n.Name, Type: "bridge", NetworkPut: put}})
			continue
		}
		if !owned(actual.Config) {
			return Plan{}, fmt.Errorf("network %s is not owned by Garden; adopt it explicitly", n.Name)
		}
		if manual := NetworkCompatibility(n, *actual); manual != "" {
			p.Manual = append(p.Manual, "network "+n.Name+": "+manual)
			continue
		}
		put := actual.Writable()
		put.Config = maps.Clone(put.Config)
		keys := patch(put.Config, n.Config)
		if n.Description != nil && put.Description != *n.Description {
			put.Description = *n.Description
			keys = append(keys, "description")
		}
		if len(keys) > 0 {
			p.Operations = append(p.Operations, Operation{Kind: UpdateNetwork, Name: n.Name, BeforeNetwork: actual, NetworkPut: put})
		}
	}
	for _, n := range s.Networks {
		if owned(n.Config) && desiredNetwork(g, n.Name) == nil {
			p.Orphans = append(p.Orphans, "network "+n.Name)
		}
	}
	return p, nil
}

func buildNetworkAdoption(g definition.Garden, target Target, b Backend) (Plan, error) {
	if err := g.Validate(); err != nil {
		return Plan{}, err
	}
	if err := target.validate(); err != nil {
		return Plan{}, err
	}
	n := desiredNetwork(g, target.Name)
	if n == nil {
		return Plan{}, fmt.Errorf("network %s must be declared before adoption", target.Name)
	}
	s, err := b.Discover()
	if err != nil {
		return Plan{}, err
	}
	actual := findNetwork(s, n.Name)
	if actual == nil {
		return Plan{}, fmt.Errorf("network %s does not exist", n.Name)
	}
	if err := adoptionOwner(actual.Config); err != nil {
		return Plan{}, err
	}
	if manual := NetworkCompatibility(*n, *actual); manual != "" {
		return Plan{}, fmt.Errorf("cannot adopt network %s: %s", n.Name, manual)
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
	return Plan{Operations: []Operation{{Kind: AdoptNetwork, Name: n.Name, BeforeNetwork: actual, NetworkPut: put}}}, nil
}
