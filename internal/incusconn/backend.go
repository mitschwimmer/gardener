package incusconn

import (
	"fmt"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

// Backend keeps project selection and official-client calls in one place.
type Backend struct{ Server incus.InstanceServer }

func (b Backend) Discover() (reconcile.Snapshot, error) {
	server := b.Server.UseProject("default")
	projects, err := server.GetProjects()
	if err != nil {
		return reconcile.Snapshot{}, err
	}
	s := reconcile.Snapshot{Projects: projects, Instances: map[string][]api.Instance{}}
	for _, p := range projects {
		instances, err := server.UseProject(p.Name).GetInstances(api.InstanceTypeAny)
		if err != nil {
			return reconcile.Snapshot{}, fmt.Errorf("project %s instances: %w", p.Name, err)
		}
		s.Instances[p.Name] = instances
	}
	networks, err := b.Server.UseProject("default").GetNetworks()
	if err != nil {
		return reconcile.Snapshot{}, fmt.Errorf("discover default-project networks: %w", err)
	}
	s.Networks = networks
	if err := b.discoverVolumes(&s); err != nil {
		return reconcile.Snapshot{}, fmt.Errorf("discover custom volumes: %w", err)
	}
	return s, nil
}

func (b Backend) CheckReferences(v definition.VM) error {
	server := b.Server.UseProject("default")
	pool, _, err := server.GetStoragePool(v.Pool)
	if err != nil {
		return fmt.Errorf("storage pool %s: %w", v.Pool, err)
	}
	if pool.Status != "Created" {
		return fmt.Errorf("storage pool %s is not ready (status %s)", v.Pool, pool.Status)
	}
	// An empty network is allowed only for preflight of a declared missing bridge.
	if v.Network != "" {
		network, _, err := server.GetNetwork(v.Network)
		if err != nil {
			return fmt.Errorf("network %s: %w", v.Network, err)
		}
		if !network.Managed || network.Type != "bridge" || network.Status != "Created" {
			return fmt.Errorf("network %s must be a ready managed bridge", v.Network)
		}

	}
	if v.Image != "" {
		image, _, err := server.GetImage(v.Image)
		if err != nil {
			return fmt.Errorf("image %s: %w", v.Image, err)
		}
		if image.Type != "virtual-machine" || image.Fingerprint != v.Image {
			return fmt.Errorf("image must be a VM image matching the full fingerprint")
		}
	}
	return nil
}

func (b Backend) Execute(op reconcile.Operation) error {
	server := b.Server.UseProject("default")
	switch op.Kind {
	case reconcile.CreateNetwork, reconcile.UpdateNetwork, reconcile.AdoptNetwork:
		return b.networkOperation(op)
	case reconcile.CreateVolume, reconcile.UpdateVolume, reconcile.AdoptVolume:
		return b.volumeOperation(op)
	case reconcile.ReconfigureVM:
		return b.reconfigure(op)
	case reconcile.AdoptProject, reconcile.DeleteProject:
		if op.Name == "default" {
			return fmt.Errorf("the default project cannot be adopted or deleted")
		}
		actual, etag, err := server.GetProject(op.Name)
		if err != nil {
			return err
		}
		if op.BeforeProject == nil || !reconcile.SameProject(*actual, *op.BeforeProject) {
			return fmt.Errorf("project changed since discovery; rerun the command")
		}
		if op.Kind == reconcile.AdoptProject {
			if actual.Config[definition.OwnerKey] != "" {
				return fmt.Errorf("project is no longer unowned; rerun adopt")
			}
			return server.UpdateProject(op.Name, op.ProjectPut, etag)
		}
		if actual.Config[definition.OwnerKey] != definition.Owner {
			return fmt.Errorf("project is no longer owned by Garden")
		}
		instances, err := server.UseProject(op.Name).GetInstances(api.InstanceTypeAny)
		if err != nil {
			return err
		}
		if len(instances) != 0 || len(actual.UsedBy) != 0 {
			return fmt.Errorf("project is no longer empty; rerun delete")
		}
		return server.UseProject("default").DeleteProject(op.Name)
	case reconcile.AdoptVM, reconcile.DeleteVM:
		if op.Project == "default" {
			return fmt.Errorf("default-project resources cannot be adopted or deleted")
		}
		project, _, err := server.GetProject(op.Project)
		if err != nil {
			return err
		}
		if project.Config[definition.OwnerKey] != definition.Owner || op.BeforeProject == nil || !reconcile.SameProject(*project, *op.BeforeProject) {
			return fmt.Errorf("project ownership or configuration changed; rerun the command")
		}
		server = server.UseProject(op.Project)
		actual, etag, err := server.GetInstance(op.Name)
		if err != nil {
			return err
		}
		if op.BeforeInstance == nil || !reconcile.SameInstance(*actual, *op.BeforeInstance) {
			return fmt.Errorf("instance changed since discovery; rerun the command")
		}
		if op.Kind == reconcile.AdoptVM {
			if actual.Config[definition.OwnerKey] != "" {
				return fmt.Errorf("instance is no longer unowned; rerun adopt")
			}
			if actual.Type != string(api.InstanceTypeVM) || actual.Ephemeral || actual.Stateful || len(actual.Profiles) != 0 ||
				actual.Status != "Running" && actual.Status != "Stopped" {
				return fmt.Errorf("instance type or lifecycle is incompatible with adoption")
			}
			return wait(server.UpdateInstance(op.Name, op.InstancePut, etag))
		}
		if err := reconcile.ValidateVMDeletion(*actual); err != nil {
			return err
		}
		return wait(server.DeleteInstance(op.Name))
	case reconcile.CreateProject:
		return server.CreateProject(api.ProjectsPost{Name: op.Name, ProjectPut: op.ProjectPut})
	case reconcile.UpdateProject:
		actual, etag, err := server.GetProject(op.Name)
		if err != nil {
			return err
		}
		if actual.Config[definition.OwnerKey] != definition.Owner {
			return fmt.Errorf("project is no longer owned by Garden")
		}
		if op.BeforeProject == nil || !reconcile.SameProject(*actual, *op.BeforeProject) {
			return fmt.Errorf("project changed since discovery; rerun apply")
		}
		return server.UpdateProject(op.Name, op.ProjectPut, etag)
	case reconcile.CreateVM:
		if err := b.checkVMNetwork(op.DesiredNetwork); err != nil {
			return err
		}
		if err := b.checkDisks(op.DesiredVM); err != nil {
			return err
		}
		// Recheck project ownership/isolation immediately before creating an
		// instance; a stale plan must not create in an unmanaged project.
		p, _, err := server.GetProject(op.Project)
		if err != nil {
			return err
		}
		for key, value := range reconcile.ProjectConfig() {
			if p.Config[key] != value {
				return fmt.Errorf("project ownership or isolation changed; rerun apply")
			}
		}
		return wait(server.UseProject(op.Project).CreateInstance(op.InstancePost))
	case reconcile.UpdateVM:
		if err := b.checkVMNetwork(op.DesiredNetwork); err != nil {
			return err
		}
		if err := b.checkDisks(op.DesiredVM); err != nil {
			return err
		}
		server = server.UseProject(op.Project)
		actual, etag, err := server.GetInstance(op.Name)
		if err != nil {
			return err
		}
		if err := mutable(*actual); err != nil {
			return err
		}
		if op.BeforeInstance == nil || !reconcile.SameInstance(*actual, *op.BeforeInstance) {
			return fmt.Errorf("instance changed since discovery; rerun apply")
		}
		return wait(server.UpdateInstance(op.Name, op.InstancePut, etag))
	case reconcile.StartVM, reconcile.StopVM:
		if op.Kind == reconcile.StartVM {
			if err := b.checkVMNetwork(op.DesiredNetwork); err != nil {
				return err
			}
			if err := b.checkDisks(op.DesiredVM); err != nil {
				return err
			}
		}
		server = server.UseProject(op.Project)
		actual, _, err := server.GetInstance(op.Name)
		if err != nil {
			return err
		}
		if err := mutable(*actual); err != nil {
			return err
		}
		state, etag, err := server.GetInstanceState(op.Name)
		if err != nil {
			return err
		}
		if state.Status != op.FromState {
			return fmt.Errorf("power state changed since discovery; rerun apply")
		}
		action := "start"
		if op.Kind == reconcile.StopVM {
			action = "stop"
		}
		return wait(server.UpdateInstanceState(op.Name, api.InstanceStatePut{Action: action, Timeout: 30, Force: false, Stateful: false}, etag))
	default:
		return fmt.Errorf("unsupported operation %q", op.Kind)
	}
}

func mutable(actual api.Instance) error {
	if actual.Config[definition.OwnerKey] != definition.Owner {
		return fmt.Errorf("instance is no longer owned by Garden")
	}
	if actual.Type != string(api.InstanceTypeVM) || actual.Ephemeral || actual.Stateful {
		return fmt.Errorf("instance type or lifecycle changed; rerun apply")
	}
	return nil
}

func wait(op incus.Operation, err error) error {
	if err != nil {
		return err
	}
	if err := op.Wait(); err != nil {
		return operationFailure{id: op.Get().ID, cause: err}
	}
	return nil
}

type operationFailure struct {
	id    string
	cause error
}

func (e operationFailure) Error() string {
	if e.id == "" {
		return e.cause.Error()
	}
	return fmt.Sprintf("Incus operation %s: %v", e.id, e.cause)
}
func (e operationFailure) Unwrap() error       { return e.cause }
func (e operationFailure) OperationID() string { return e.id }
