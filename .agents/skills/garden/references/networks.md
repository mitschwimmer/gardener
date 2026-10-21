# Managed bridge networks

## Define and use a bridge

Garden can create and adopt native Incus-managed bridges in `default`. Managed
workload projects share this namespace (`features.networks=false`). Define an
independent network, then use its name in a VM's existing `Network` field:

```go
Networks: []definition.Network{{
    Name: "appsbr0",
    IPv4Address: "10.42.0.1/24", IPv4NAT: true,
    IPv6Address: "none",
    Description: definition.Value("Application network"),
    Config: definition.Properties{"user.note": definition.Value("garden")},
}},
// Within the VM definition:
Network: "appsbr0",
```

Choose actual subnets appropriate to the host; the example is not an active
workload. Both address fields must explicitly contain a CIDR or `none`; random
`auto` allocation is unsupported. IPv4 uses a unicast host address on a /1 through
/30 subnet, excluding the subnet/broadcast address. IPv6 currently requires a
unicast host address on /64. NAT is an explicit boolean for each family and cannot
be enabled when that family is disabled. A bridge name must be valid and at most
15 characters. Reserved `user.garden.*` metadata remains unavailable to definitions.

Declared subnets cannot overlap each other. Before a new bridge is planned, its
subnets are also checked against addresses in discovered networks. This is not a
host routing, uplink, cluster, or external network conflict audit; the operator
must choose appropriate addressing. Garden does not attach physical interfaces,
configure tunnels, manage OVN, firewall policy, DNS records, static guest leases,
or isolated project networks. Unspecified Incus defaults/configuration remain
owned by Incus or the operator. Incus may run DHCP/DNS on an addressed bridge.
Guest setup and services remain independently managed through NixOS/Nix.

VMs may still reference an already existing, externally managed ready bridge
without declaring it. Declared bridges require Garden ownership. A bridge that is
missing but declared is created before the VM; pool/image validation still runs
before any mutation. Existing VM NIC placement remains creation-only: changing
a VM's network produces a manual action, even if both networks exist.

## Reconcile and adopt

Fixed phases are projects, networks, volumes, then VMs; sort each resource phase
by its name or pool/name. Creation marks the bridge with `user.garden.owner=garden`.
Only description and nonreserved `user.*` metadata update automatically. Omitted
metadata remains unchanged; nil removes a declared metadata key. Other network
configuration is preserved. Metadata updates preserve attached VMs and devices.

Address, NAT, and bridge-driver changes require manual intervention and block the
whole apply. Non-native drivers, physical interfaces, unready bridges, and bridges
with external interfaces or nonstandard modes cannot be adopted as this resource.
To adopt a reviewed compatible bridge, declare its actual topology first:

```sh
garden adopt network appsbr0
garden plan
garden apply
```

Adoption changes only the ownership marker; the next apply reconciles metadata.
An unowned name collision or another owner stops apply. Before an update/adoption,
Garden rereads writable state and readiness, verifies unchanged discovery and
ownership, and sends the fresh ETag. Changing references alone does not prevent a
metadata-only update. Before VM creation, supported updates, starts, and each
reconfiguration phase, a declared network's ownership, readiness, and topology
are checked again. An ownership loss stops later work and preserves pending VM
power recovery intent. These are single-operator guards, not distributed locks.

Removing a network declaration reports a preserved orphan, even if VMs still use
it. It then follows the existing external-reference contract for those VMs. There
is no `garden delete network` command: inspect all consumers and use an explicit
Incus procedure when removal is intended. No bridge is deleted or replaced by
apply. See [Operator diagnostics](diagnostics.md) for failure output and retries.

## Validation

Recording tests cover invalid/overlapping subnets, phase order, partial failure
and resume, idempotency, ownership/adoption, metadata preservation/removal, topology
refusal, stale state/ETags, incomplete discovery, and ownership loss during a VM
restart. Live CI creates a bridge before an Alpine VM, verifies an IPv4 DHCP
address inside the guest, adopts and updates bridge metadata while the VM runs,
refuses NAT/subnet changes and stale writes, and preserves an omitted network.
It validates Ubuntu's Incus baseline, not production IncusOS, IPv6 guest behavior,
external routing, or other network drivers.

Contracts come from the pinned [network API](https://github.com/lxc/incus/blob/v7.5.1/shared/api/network.go),
[official client](https://github.com/lxc/incus/blob/v7.5.1/client/incus_networks.go),
and [native bridge driver](https://github.com/lxc/incus/blob/v7.5.1/internal/server/network/driver_bridge.go).
