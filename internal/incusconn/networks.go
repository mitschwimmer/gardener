package incusconn

import (
	"fmt"

	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func (b Backend) networkOperation(op reconcile.Operation) error {
	server := b.Server.UseProject("default")
	if op.Kind == reconcile.CreateNetwork {
		return server.CreateNetwork(op.NetworkPost)
	}
	actual, etag, err := server.GetNetwork(op.Name)
	if err != nil {
		return err
	}
	if op.BeforeNetwork == nil || !reconcile.SameNetwork(*actual, *op.BeforeNetwork) {
		return fmt.Errorf("network changed since discovery; rerun the command")
	}
	if !actual.Managed || actual.Type != "bridge" || actual.Status != "Created" {
		return fmt.Errorf("network is no longer a ready managed bridge")
	}
	if op.Kind == reconcile.AdoptNetwork {
		if actual.Config[definition.OwnerKey] != "" {
			return fmt.Errorf("network is no longer unowned; rerun adopt")
		}
	} else if actual.Config[definition.OwnerKey] != definition.Owner {
		return fmt.Errorf("network is no longer owned by Garden")
	}
	return server.UpdateNetwork(op.Name, op.NetworkPut, etag)
}

func (b Backend) checkVMNetwork(n *definition.Network) error {
	if n == nil {
		return nil
	} // Externally managed references retain their existing contract.
	actual, _, err := b.Server.UseProject("default").GetNetwork(n.Name)
	if err != nil {
		return fmt.Errorf("read declared network %s before VM change: %w", n.Name, err)
	}
	if actual.Config[definition.OwnerKey] != definition.Owner {
		return fmt.Errorf("declared network %s is no longer owned by Garden", n.Name)
	}
	if reason := reconcile.NetworkCompatibility(*n, *actual); reason != "" {
		return fmt.Errorf("declared network %s changed: %s", n.Name, reason)
	}
	return nil
}
