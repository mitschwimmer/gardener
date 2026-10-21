package definition

import (
	"fmt"
	"net/netip"
	"strings"
)

// Network is a native managed bridge in the shared default project. Address and
// NAT settings are creation-only; later topology changes require intervention.
type Network struct {
	Name                     string
	Description              *string
	IPv4Address, IPv6Address string
	IPv4NAT, IPv6NAT         bool
	Config                   Properties
}

func (g Garden) validateNetworks() error {
	names := map[string]bool{}
	var prefixes []netip.Prefix
	for _, n := range g.Networks {
		if !namePattern.MatchString(n.Name) || len(n.Name) > 15 || names[n.Name] {
			return fmt.Errorf("network %q needs a unique valid bridge name of at most 15 characters", n.Name)
		}
		names[n.Name] = true
		for i, address := range []string{n.IPv4Address, n.IPv6Address} {
			nat := n.IPv4NAT
			if i == 1 {
				nat = n.IPv6NAT
			}
			if address == "none" {
				if nat {
					return fmt.Errorf("network %s cannot enable NAT without an address", n.Name)
				}
				continue
			}
			p, err := netip.ParsePrefix(address)
			if err != nil || p.Addr().Is4() != (i == 0) || p.Addr().Is4In6() || !p.Addr().IsGlobalUnicast() || p.Addr() == p.Masked().Addr() {
				return fmt.Errorf("network %s needs an explicit usable IPv%d bridge CIDR or none", n.Name, 4+2*i)
			}
			if i == 0 {
				if p.Bits() < 1 || p.Bits() > 30 {
					return fmt.Errorf("network %s IPv4 prefix must be /1 through /30", n.Name)
				}
				a := p.Addr().As4()
				v := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
				mask := uint32(1)<<(32-p.Bits()) - 1
				if v&mask == mask {
					return fmt.Errorf("network %s bridge address cannot be the IPv4 broadcast address", n.Name)
				}
			} else if p.Bits() != 64 {
				return fmt.Errorf("network %s currently requires an IPv6 /64", n.Name)
			}
			for _, previous := range prefixes {
				if previous.Overlaps(p) {
					return fmt.Errorf("network %s overlaps another declared bridge subnet", n.Name)
				}
			}
			prefixes = append(prefixes, p)
		}
		for _, key := range Keys(n.Config) {
			if !strings.HasPrefix(key, "user.") || key == "user." || strings.HasPrefix(key, "user.garden.") {
				return fmt.Errorf("network %s cannot manage config key %q", n.Name, key)
			}
		}
	}
	return nil
}
