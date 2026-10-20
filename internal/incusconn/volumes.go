package incusconn

import (
	"fmt"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

func (b Backend) CheckVolume(v definition.Volume) error {
	pool, _, err := b.Server.UseProject("default").GetStoragePool(v.Pool)
	if err != nil {
		return err
	}
	if pool.Status != "Created" {
		return fmt.Errorf("storage pool %s is not ready", v.Pool)
	}
	if v.ContentType == "block" && pool.Driver != "dir" {
		return fmt.Errorf("managed block volumes currently require the validated dir storage driver")
	}
	return nil
}

func (b Backend) volumeOperation(op reconcile.Operation) error {
	server := b.Server.UseProject("default")
	if op.Kind == reconcile.CreateVolume {
		if err := b.CheckVolume(definition.Volume{Pool: op.Pool, ContentType: op.VolumePost.ContentType}); err != nil {
			return err
		}
		return server.CreateStoragePoolVolume(op.Pool, op.VolumePost)
	}
	actual, etag, err := server.GetStoragePoolVolume(op.Pool, "custom", op.Name)
	if err != nil {
		return err
	}
	if op.BeforeVolume == nil || !reconcile.SameVolume(*actual, *op.BeforeVolume) {
		return fmt.Errorf("volume changed since discovery; rerun the command")
	}
	if op.Kind == reconcile.AdoptVolume {
		if actual.Config[definition.OwnerKey] != "" {
			return fmt.Errorf("volume is no longer unowned; rerun adopt")
		}
	} else if actual.Config[definition.OwnerKey] != definition.Owner {
		return fmt.Errorf("volume is no longer owned by Garden")
	}
	if err := b.CheckVolume(definition.Volume{Pool: op.Pool, ContentType: actual.ContentType}); err != nil {
		return err
	}
	if actual.Config["size"] != op.VolumePut.Config["size"] {
		old, oldErr := definition.VolumeSizeBytes(actual.Config["size"])
		want, wantErr := definition.VolumeSizeBytes(op.VolumePut.Config["size"])
		if actual.ContentType != "block" || oldErr != nil || wantErr != nil || want < old || len(actual.UsedBy) != 0 {
			return fmt.Errorf("unsafe volume resize; only detached block growth is supported")
		}
	}
	return server.UpdateStoragePoolVolume(op.Pool, "custom", op.Name, op.VolumePut, etag)
}

func (b Backend) checkDisks(v definition.VM) error {
	server := b.Server.UseProject("default")
	for _, disk := range v.Disks {
		volume, _, err := server.GetStoragePoolVolume(disk.Pool, "custom", disk.Volume)
		if err != nil {
			return err
		}
		content := "filesystem"
		if disk.Path == "" {
			content = "block"
		}
		if volume.Config[definition.OwnerKey] != definition.Owner || volume.Type != "custom" || volume.ContentType != content ||
			!reconcile.VolumeUseAllowed(volume.UsedBy, v.Project, v.Name) {
			return fmt.Errorf("volume %s ownership, content type, or attachment changed; rerun apply", definition.VolumeID(disk.Pool, disk.Volume))
		}
	}
	return nil
}

func (b Backend) discoverVolumes(s *reconcile.Snapshot) error {
	server := b.Server.UseProject("default")
	pools, err := server.GetStoragePoolNames()
	if err != nil {
		return err
	}
	s.Volumes = map[string][]api.StorageVolume{}
	for _, pool := range pools {
		volumes, err := server.GetStoragePoolVolumes(pool)
		if err != nil {
			return fmt.Errorf("pool %s volumes: %w", pool, err)
		}
		for _, volume := range volumes {
			if volume.Type == "custom" {
				s.Volumes[pool] = append(s.Volumes[pool], volume)
			}
		}
	}
	return nil
}
