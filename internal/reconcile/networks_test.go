package reconcile

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func networkGarden() definition.Garden {
	g := desired()
	g.Networks = []definition.Network{{Name: "bridge", IPv4Address: "192.0.2.1/24", IPv6Address: "none", Config: definition.Properties{"user.note": definition.Value("managed")}}}
	return g
}
func TestNetworkPhasesRecoveryAndOmission(t *testing.T) {
	g, f := networkGarden(), emptyBackend()
	p, err := Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{CreateProject, CreateNetwork, CreateVM, StartVM}) {
		t.Fatalf("%v %+v", err, p)
	}
	if len(f.checked) != 1 || f.checked[0].Network != "" || p.Operations[2].DesiredNetwork == nil {
		t.Fatal("new bridge preflight or execution contract missing")
	}
	f.failAt = 3
	if err := p.Apply(f, io.Discard); err == nil {
		t.Fatal("expected failure")
	}
	f.failAt = 0
	p, err = Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{CreateVM, StartVM}) {
		t.Fatalf("resume: %v %+v", err, p)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	p, err = Build(g, f)
	if err != nil || len(p.Operations) > 0 {
		t.Fatalf("not idempotent: %v %+v", err, p)
	}
	g.Networks = nil
	p, err = Build(g, f)
	if err != nil || len(p.Operations) > 0 || !reflect.DeepEqual(p.Orphans, []string{"network bridge"}) {
		t.Fatalf("omission: %v %+v", err, p)
	}
}
func TestNetworkAdoptionPreservationAndManualBlocking(t *testing.T) {
	g, f := networkGarden(), existingBackend(t)
	n := api.Network{Name: "bridge", Type: "bridge", Managed: true, Status: "Created", NetworkPut: api.NetworkPut{Config: NetworkConfig(g.Networks[0]), Description: "external"}}
	n.Config["user.external"] = "keep"
	n.Config["user.remove"] = "old"
	f.snapshot.Networks = []api.Network{n}
	if _, err := Build(g, f); err == nil {
		t.Fatal("unowned network accepted")
	}
	p, err := BuildAdoption(g, Target{Network: true, Name: "bridge"}, f)
	if err != nil || len(p.Operations) != 1 || p.Operations[0].Kind != AdoptNetwork {
		t.Fatalf("%v %+v", err, p)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	if f.snapshot.Networks[0].Description != "external" || f.snapshot.Networks[0].Config["user.note"] != "" {
		t.Fatal("adoption changed metadata")
	}
	g.Networks[0].Config["user.remove"] = nil
	g.Networks[0].Description = definition.Value("managed")
	p, err = Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{UpdateNetwork}) {
		t.Fatalf("%v %+v", err, p)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	if f.snapshot.Networks[0].Config["user.external"] != "keep" || f.snapshot.Networks[0].Config["user.remove"] != "" {
		t.Fatal("metadata not preserved/removed")
	}
	for _, change := range []func(*definition.Network){func(n *definition.Network) { n.IPv4Address = "192.0.3.1/24" }, func(n *definition.Network) { n.IPv4NAT = true }} {
		altered := g
		altered.Networks = append([]definition.Network(nil), g.Networks...)
		change(&altered.Networks[0])
		f.executed = nil
		p, err = Build(altered, f)
		if err != nil || len(p.Manual) == 0 {
			t.Fatalf("unsafe update accepted: %v %+v", err, p)
		}
		if err := p.Apply(f, io.Discard); err == nil || len(f.executed) > 0 {
			t.Fatal("manual action did not block apply")
		}
	}
}
func TestNetworkForeignTypesAndSubnetOverlap(t *testing.T) {
	g, f := networkGarden(), emptyBackend()
	f.snapshot.Networks = []api.Network{{Name: "existing", NetworkPut: api.NetworkPut{Config: api.ConfigMap{"ipv4.address": "192.0.2.254/24"}}}}
	if _, err := Build(g, f); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatal("overlap accepted", err)
	}
	for _, change := range []func(*api.Network){
		func(n *api.Network) { n.Managed = false }, func(n *api.Network) { n.Type = "ovn" }, func(n *api.Network) { n.Status = "Pending" },
		func(n *api.Network) { n.Config["bridge.external_interfaces"] = "eth0" }, func(n *api.Network) { n.Config["bridge.driver"] = "openvswitch" },
		func(n *api.Network) { n.Config[definition.OwnerKey] = "other" },
	} {
		n := api.Network{Name: "bridge", Type: "bridge", Managed: true, Status: "Created", NetworkPut: api.NetworkPut{Config: NetworkConfig(g.Networks[0])}}
		change(&n)
		f.snapshot.Networks = []api.Network{n}
		if _, err := BuildAdoption(g, Target{Network: true, Name: "bridge"}, f); err == nil {
			t.Fatal("incompatible adoption accepted", n)
		}
	}
}
