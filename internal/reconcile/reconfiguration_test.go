package reconcile

import (
	"maps"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func TestRunningOfflineChangesPlanReconfiguration(t *testing.T) {
	for _, power := range []definition.Power{definition.Unmanaged, definition.Running, definition.Stopped} {
		for _, remove := range []bool{false, true} {
			b := existingBackend(t)
			g := desired()
			g.VMs[0].Power = power
			g.VMs[0].Config = definition.Properties{"limits.cpu": definition.Value("2")}
			if remove {
				b.snapshot.Instances["apps"][0].Config["limits.memory"] = "2GiB"
				g.VMs[0].Config = definition.Properties{"limits.memory": nil}
			}
			plan, err := Build(g, b)
			if err != nil || len(plan.Operations) != 1 || len(plan.Manual) != 0 || plan.Operations[0].Kind != ReconfigureVM {
				t.Fatalf("power=%s remove=%v plan=%+v error=%v", power, remove, plan, err)
			}
			want := "Running"
			if power == definition.Stopped {
				want = "Stopped"
			}
			if plan.Operations[0].FinalState != want || len(b.executed) != 0 {
				t.Fatal("wrong final state or planning mutated infrastructure")
			}
		}
	}
}

func TestPendingRestartResumesEvenWithoutConfigDiffOrManagedPower(t *testing.T) {
	for _, state := range []string{"Running", "Stopped"} {
		for _, pending := range []string{"running", "stopped"} {
			for _, override := range []definition.Power{definition.Unmanaged, definition.Running, definition.Stopped} {
				b := existingBackend(t)
				g := desired()
				g.VMs[0].Power = override
				actual := &b.snapshot.Instances["apps"][0]
				actual.Status = state
				actual.Config[definition.PendingPowerKey] = pending
				plan, err := Build(g, b)
				if err != nil || len(plan.Operations) != 1 || plan.Operations[0].Kind != ReconfigureVM {
					t.Fatalf("lost pending restart: %v %+v", err, plan)
				}
				want := map[string]string{"running": "Running", "stopped": "Stopped"}[pending]
				if override == definition.Running {
					want = "Running"
				} else if override == definition.Stopped {
					want = "Stopped"
				}
				if plan.Operations[0].FinalState != want {
					t.Fatalf("explicit power did not override pending intent: %+v", plan)
				}
			}
		}
	}
}

func TestMetadataOnlyAndStoppedUnmanagedChangesNeedNoRestart(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		b := existingBackend(t)
		g := desired()
		g.VMs[0].Power = definition.Unmanaged
		g.VMs[0].Config = definition.Properties{"user.note": definition.Value("new")}
		if stopped {
			b.snapshot.Instances["apps"][0].Status = "Stopped"
			g.VMs[0].Config = definition.Properties{"limits.cpu": definition.Value("2")}
		}
		plan, err := Build(g, b)
		if err != nil || len(plan.Operations) != 1 || plan.Operations[0].Kind != UpdateVM {
			t.Fatalf("unnecessary restart: %v %+v", err, plan)
		}
	}
}

func TestReconfigurationStateAllowsRuntimeBookkeepingButProtectsIdentity(t *testing.T) {
	a := api.Instance{Type: "virtual-machine", Status: "Running", InstancePut: api.InstancePut{Config: api.ConfigMap{
		"volatile.base_image": "seed", "volatile.uuid": "identity", "volatile.last_state.power": "RUNNING", "user.note": "keep",
	}}}
	b := a
	b.Config = maps.Clone(a.Config)
	b.Config["volatile.last_state.power"] = "STOPPED"
	if !SameReconfigurationState(a, b) {
		t.Fatal("runtime bookkeeping prevented recovery")
	}
	for _, key := range []string{"volatile.base_image", "volatile.uuid", "user.note"} {
		b.Config = maps.Clone(a.Config)
		b.Config[key] = "changed"
		if SameReconfigurationState(a, b) {
			t.Fatalf("ignored changed %s", key)
		}
	}
}
