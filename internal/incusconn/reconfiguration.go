package incusconn

import (
	"fmt"
	"maps"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

// reconfigure persists final power intent before any stop. Every failure leaves
// the marker intact so a fresh whole-garden apply can resume from Incus state.
func (b Backend) reconfigure(op reconcile.Operation) error {
	if op.FinalState != "Running" && op.FinalState != "Stopped" || op.BeforeInstance == nil {
		return fmt.Errorf("invalid VM reconfiguration operation")
	}
	server := b.Server.UseProject(op.Project)
	current, etag, err := server.GetInstance(op.Name)
	if err != nil {
		return err
	}
	if !reconcile.SameInstance(*current, *op.BeforeInstance) || current.Type != op.BeforeInstance.Type {
		return fmt.Errorf("instance changed since discovery; rerun apply")
	}
	check := func(v api.Instance) error {
		if err := mutable(v); err != nil {
			return err
		}
		if len(v.Profiles) != 0 || v.Status != "Running" && v.Status != "Stopped" {
			return fmt.Errorf("VM lifecycle changed; rerun apply")
		}
		project, _, err := b.Server.UseProject("default").GetProject(op.Project)
		if err != nil {
			return err
		}
		if project.Config[definition.OwnerKey] != definition.Owner {
			return fmt.Errorf("project is no longer owned by Garden")
		}
		return nil
	}
	if err := check(*current); err != nil {
		return err
	}
	refresh := func(expected api.Instance) error {
		actual, newETag, err := server.GetInstance(op.Name)
		if err != nil {
			return err
		}
		if err := check(*actual); err != nil {
			return err
		}
		if !reconcile.SameReconfigurationState(*actual, expected) {
			return fmt.Errorf("instance changed during reconfiguration; rerun apply")
		}
		current, etag = actual, newETag
		return nil
	}
	update := func(put api.InstancePut) error {
		expected := *current
		expected.InstancePut = put
		if err := wait(server.UpdateInstance(op.Name, put, etag)); err != nil {
			return err
		}
		return refresh(expected)
	}
	power := func(action, final string) error {
		state, stateETag, err := server.GetInstanceState(op.Name)
		if err != nil {
			return err
		}
		if state.Status != current.Status {
			return fmt.Errorf("power state changed during reconfiguration; rerun apply")
		}
		expected := *current
		expected.Status = final
		if err := wait(server.UpdateInstanceState(op.Name, api.InstanceStatePut{Action: action, Timeout: 30, Force: false, Stateful: false}, stateETag)); err != nil {
			return err
		}
		return refresh(expected)
	}
	final := strings.ToLower(op.FinalState)
	if current.Config[definition.PendingPowerKey] != final {
		put := current.Writable()
		put.Config = maps.Clone(put.Config)
		put.Config[definition.PendingPowerKey] = final
		if err := update(put); err != nil {
			return fmt.Errorf("record pending power intent: %w", err)
		}
	}
	_, keys := reconcile.VMUpdate(op.DesiredVM, *current)
	if current.Status == "Running" && (reconcile.NeedsStopped(keys) || op.FinalState == "Stopped") {
		if err := power("stop", "Stopped"); err != nil {
			return fmt.Errorf("stop VM: %w", err)
		}
	}
	put, keys := reconcile.VMUpdate(op.DesiredVM, *current)
	if len(keys) != 0 {
		if err := update(put); err != nil {
			return fmt.Errorf("update VM: %w", err)
		}
	}
	if current.Status == "Stopped" && op.FinalState == "Running" {
		if err := power("start", "Running"); err != nil {
			return fmt.Errorf("start VM: %w", err)
		}
	}
	put = current.Writable()
	put.Config = maps.Clone(put.Config)
	delete(put.Config, definition.PendingPowerKey)
	if err := update(put); err != nil {
		return fmt.Errorf("clear pending power intent: %w", err)
	}
	return nil
}
