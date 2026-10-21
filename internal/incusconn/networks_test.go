package incusconn

import (
	"errors"
	"maps"
	"strings"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

type networkClient struct {
	discoveryClient
	network      api.Network
	networks     []api.Network
	networkPut   api.NetworkPut
	networkPost  api.NetworksPost
	networkError error
}

func (f *networkClient) UseProject(p string) incus.InstanceServer { f.selected = p; return f }
func (f *networkClient) GetNetworks() ([]api.Network, error) {
	if f.selected != "default" {
		return nil, errors.New("wrong namespace")
	}
	return f.networks, f.networkError
}
func (f *networkClient) GetNetwork(string) (*api.Network, string, error) {
	return &f.network, "network-etag", f.networkError
}
func (f *networkClient) CreateNetwork(post api.NetworksPost) error {
	if f.selected != "default" {
		return errors.New("wrong namespace")
	}
	f.writes++
	f.networkPost = post
	return f.requestError
}
func (f *networkClient) UpdateNetwork(name string, put api.NetworkPut, etag string) error {
	if f.selected != "default" || name != "appsbr0" {
		return errors.New("wrong namespace/name")
	}
	f.writes++
	f.networkPut = put
	f.etag = etag
	return f.requestError
}
func networkFixture() (*networkClient, definition.Network) {
	n := definition.Network{Name: "appsbr0", IPv4Address: "192.0.2.1/24", IPv6Address: "none"}
	c := reconcile.NetworkConfig(n)
	c[definition.OwnerKey] = definition.Owner
	f := &networkClient{network: api.Network{Name: n.Name, Type: "bridge", Managed: true, Status: "Created", NetworkPut: api.NetworkPut{Config: c}}}
	f.networks = []api.Network{f.network}
	return f, n
}
func TestNetworkAdapterStaleStateOwnershipAndETags(t *testing.T) {
	for _, change := range []string{"none", "owner", "address", "status", "read", "write"} {
		f, n := networkFixture()
		before := f.network
		before.Config = maps.Clone(before.Config)
		put := before.Writable()
		put.Config = maps.Clone(put.Config)
		put.Description = "updated"
		switch change {
		case "owner":
			f.network.Config[definition.OwnerKey] = "other"
		case "address":
			f.network.Config["ipv4.address"] = "192.0.3.1/24"
		case "status":
			f.network.Status = "Pending"
		case "read":
			f.networkError = errors.New("read")
		case "write":
			f.requestError = errors.New("write")
		}
		err := (Backend{Server: f}).Execute(reconcile.Operation{Kind: reconcile.UpdateNetwork, Name: n.Name, BeforeNetwork: &before, NetworkPut: put})
		if (err != nil) != (change != "none") {
			t.Fatalf("%s: %v", change, err)
		}
		if change == "none" || change == "write" {
			if f.writes != 1 || f.etag != "network-etag" {
				t.Fatal("missing ETag or write")
			}
		} else if f.writes != 0 {
			t.Fatal("unsafe mutation")
		}
	}
}
func TestDeclaredNetworkRecheckedBeforeVMChange(t *testing.T) {
	for _, change := range []string{"none", "owner", "address", "unmanaged", "read"} {
		f, n := networkFixture()
		switch change {
		case "owner":
			f.network.Config[definition.OwnerKey] = "other"
		case "address":
			f.network.Config["ipv4.nat"] = "true"
		case "unmanaged":
			f.network.Managed = false
		case "read":
			f.networkError = errors.New("missing")
		}
		err := (Backend{Server: f}).checkVMNetwork(&n)
		if (err != nil) != (change != "none") || f.writes != 0 {
			t.Fatal(change, err)
		}
	}
}
func TestNetworkDiscoveryAndCreation(t *testing.T) {
	f, n := networkFixture()
	b := Backend{Server: f}
	s, err := b.Discover()
	if err != nil || len(s.Networks) != 1 || s.Networks[0].Name != n.Name || f.writes != 0 {
		t.Fatal(err, s)
	}
	f.networkError = errors.New("incomplete")
	if _, err := b.Discover(); err == nil || !strings.Contains(err.Error(), "discover default-project networks") {
		t.Fatal("ignored network discovery failure", err)
	}
	f.networkError = nil
	if err := b.Execute(reconcile.Operation{Kind: reconcile.CreateNetwork, Name: n.Name, NetworkPost: api.NetworksPost{Name: n.Name, Type: "bridge", NetworkPut: api.NetworkPut{Config: reconcile.NetworkConfig(n)}}}); err != nil || f.writes != 1 {
		t.Fatal(err)
	}
}
