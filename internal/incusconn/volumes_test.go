package incusconn

import (
	"errors"
	"maps"
	"testing"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
	"github.com/mitschwimmer/gardener/internal/reconcile"
)

type volumeClient struct {
	recordingClient
	volume    api.StorageVolume
	pool      api.StoragePool
	put       api.StorageVolumePut
	volumes   []api.StorageVolume
	readError error
}

func (f *volumeClient) UseProject(p string) incus.InstanceServer { f.selected = p; return f }
func (f *volumeClient) GetStoragePool(string) (*api.StoragePool, string, error) {
	return &f.pool, "", nil
}
func (f *volumeClient) GetStoragePoolNames() ([]string, error) { return []string{"pool"}, nil }
func (f *volumeClient) GetStoragePoolVolumes(string) ([]api.StorageVolume, error) {
	return f.volumes, f.readError
}
func (f *volumeClient) GetStoragePoolVolume(string, string, string) (*api.StorageVolume, string, error) {
	return &f.volume, "volume-etag", f.readError
}
func (f *volumeClient) UpdateStoragePoolVolume(pool, kind, name string, put api.StorageVolumePut, etag string) error {
	if f.selected != "default" || pool != "pool" || kind != "custom" || name != "data" {
		return errors.New("wrong namespace")
	}
	f.writes++
	f.etag = etag
	f.put = put
	return f.requestError
}
func (f *volumeClient) CreateStoragePoolVolume(pool string, post api.StorageVolumesPost) error {
	if f.selected != "default" || pool != "pool" || post.Type != "custom" {
		return errors.New("wrong namespace")
	}
	f.writes++
	return f.requestError
}
func volumeFixture() *volumeClient {
	return &volumeClient{pool: api.StoragePool{Driver: "dir", Status: "Created"}, volume: api.StorageVolume{Name: "data", Type: "custom", ContentType: "block", StorageVolumePut: api.StorageVolumePut{Config: api.ConfigMap{definition.OwnerKey: definition.Owner, "size": "1GiB"}}}}
}
func TestVolumeExecutionGuardsAndETag(t *testing.T) {
	for _, change := range []string{"none", "owner", "size", "used", "driver", "read", "write"} {
		t.Run(change, func(t *testing.T) {
			f := volumeFixture()
			before := f.volume
			before.Config = maps.Clone(before.Config)
			put := before.Writable()
			put.Config = maps.Clone(put.Config)
			put.Config["size"] = "2GiB"
			switch change {
			case "owner":
				f.volume.Config[definition.OwnerKey] = "other"
			case "size":
				f.volume.Config["size"] = "3GiB"
			case "used":
				f.volume.UsedBy = []string{"/1.0/instances/other"}
			case "driver":
				f.pool.Driver = "zfs"
			case "read":
				f.readError = errors.New("read")
			case "write":
				f.requestError = errors.New("write")
			}
			err := (Backend{Server: f}).Execute(reconcile.Operation{Kind: reconcile.UpdateVolume, Pool: "pool", Name: "data", BeforeVolume: &before, VolumePut: put})
			if (err != nil) != (change != "none") {
				t.Fatalf("err=%v", err)
			}
			if change == "none" || change == "write" {
				if f.writes != 1 || f.etag != "volume-etag" || f.put.Config["size"] != "2GiB" {
					t.Fatal("missing ETag resize")
				}
			} else if f.writes != 0 {
				t.Fatal("unsafe mutation")
			}
		})
	}
}
func TestVolumeDiscoveryAndDiskGuards(t *testing.T) {
	f := volumeFixture()
	f.volumes = []api.StorageVolume{f.volume, {Name: "root", Type: "virtual-machine"}}
	s := reconcile.Snapshot{}
	if err := (Backend{Server: f}).discoverVolumes(&s); err != nil || len(s.Volumes["pool"]) != 1 {
		t.Fatal(err, s)
	}
	f.readError = errors.New("incomplete")
	if err := (Backend{Server: f}).discoverVolumes(&s); err == nil {
		t.Fatal("ignored discovery failure")
	}
	f.readError = nil
	v := definition.VM{Project: "apps", Name: "wiki", Disks: []definition.Disk{{Name: "data", Pool: "pool", Volume: "data"}}}
	for _, uses := range [][]string{nil, {"/1.0/instances/wiki?project=apps"}, {"/1.0/instances/other?project=apps"}} {
		f.volume.UsedBy = uses
		err := (Backend{Server: f}).checkDisks(v)
		if (err != nil) != (len(uses) > 0 && uses[0] == "/1.0/instances/other?project=apps") {
			t.Fatal(err, uses)
		}
	}
	f.volume.UsedBy = nil
	f.volume.Config[definition.OwnerKey] = "other"
	if err := (Backend{Server: f}).checkDisks(v); err == nil {
		t.Fatal("foreign volume attached")
	}
}
