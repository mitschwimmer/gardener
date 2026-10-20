package definition

import "testing"

func validNetwork() Network {
	return Network{Name: "appsbr0", IPv4Address: "192.0.2.1/24", IPv6Address: "none"}
}
func TestNetworkDefinitions(t *testing.T) {
	for _, change := range []func(*Network){
		func(n *Network) { n.Name = "bridge-name-too-long" }, func(n *Network) { n.IPv4Address = "auto" },
		func(n *Network) { n.IPv4Address = "192.0.2.0/24" }, func(n *Network) { n.IPv4Address = "192.0.2.255/24" },
		func(n *Network) { n.IPv4Address = "192.0.2.1/32" }, func(n *Network) { n.IPv4Address = "::1/64" },
		func(n *Network) { n.IPv6Address = "" }, func(n *Network) { n.IPv6Address = "fd42::1/48" },
		func(n *Network) { n.IPv6Address = "ff02::1/64" }, func(n *Network) { n.IPv6NAT = true },
		func(n *Network) { n.Config = Properties{OwnerKey: Value("other")} },
	} {
		n := validNetwork()
		change(&n)
		if err := (Garden{Networks: []Network{n}}).Validate(); err == nil {
			t.Fatalf("invalid network accepted: %+v", n)
		}
	}
	n := validNetwork()
	n.IPv6Address = "fd42::1/64"
	n.IPv6NAT = true
	if err := (Garden{Networks: []Network{n}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, other := range []Network{n, {Name: "other", IPv4Address: "192.0.2.129/25", IPv6Address: "none"}} {
		if err := (Garden{Networks: []Network{n, other}}).Validate(); err == nil {
			t.Fatal("duplicate or overlapping networks accepted")
		}
	}
}
