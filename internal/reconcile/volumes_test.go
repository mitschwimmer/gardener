package reconcile

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/lxc/incus/v7/shared/api"
	"github.com/mitschwimmer/gardener/internal/definition"
)

func volumeGarden() definition.Garden {
	g := desired()
	g.Volumes = []definition.Volume{{Pool: "pool", Name: "data", ContentType: "filesystem"}}
	g.VMs[0].Disks = []definition.Disk{{Name: "data", Pool: "pool", Volume: "data", Path: "/data"}}
	return g
}

func TestVolumePhasesResumeAndPreservation(t *testing.T) {
	g, f := volumeGarden(), emptyBackend()
	p, err := Build(g, f)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(p), []Kind{CreateProject, CreateVolume, CreateVM, StartVM}) {
		t.Fatal(kinds(p))
	}
	f.failAt = 3
	if err := p.Apply(f, io.Discard); err == nil {
		t.Fatal("expected VM failure")
	}
	f.failAt = 0
	p, err = Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{CreateVM, StartVM}) {
		t.Fatalf("resume: %v %v", err, kinds(p))
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	p, err = Build(g, f)
	if err != nil || len(p.Operations) != 0 {
		t.Fatalf("not idempotent: %v %+v", err, p)
	}
	g.VMs[0].Disks = nil
	g.Volumes = nil
	p, err = Build(g, f)
	if err != nil || len(p.Operations) != 0 || !reflect.DeepEqual(p.Orphans, []string{"volume pool/data"}) {
		t.Fatalf("omission changed storage: %v %+v", err, p)
	}
	if f.snapshot.Instances["apps"][0].Devices["data"]["source"] != "data" {
		t.Fatal("disk detached")
	}
}

func TestVolumeUpdatesAndUnsafeChanges(t *testing.T) {
	for _, tc := range []struct {
		name, size, content string
		used                []string
		manual              bool
	}{
		{name: "grow", size: "2GiB", content: "block"},
		{name: "equal bytes", size: "1024MiB", content: "block"},
		{name: "shrink", size: "512MiB", content: "block", manual: true},
		{name: "attached growth", size: "2GiB", content: "block", used: []string{"/1.0/instances/wiki?project=apps"}, manual: true},
		{name: "type change", content: "filesystem", manual: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := api.StorageVolume{Name: "data", Type: "custom", ContentType: "block", UsedBy: tc.used, StorageVolumePut: api.StorageVolumePut{Config: api.ConfigMap{definition.OwnerKey: definition.Owner, "size": "1GiB", "user.external": "keep", "user.remove": "gone"}}}
			v := definition.Volume{Pool: "pool", Name: "data", ContentType: tc.content, Size: tc.size, Config: definition.Properties{"user.remove": nil, "user.add": definition.Value("new")}}
			put, keys, manual := VolumeUpdate(v, actual)
			if (manual != "") != tc.manual {
				t.Fatalf("manual=%q", manual)
			}
			if put.Config["user.external"] != "keep" || actual.Config["user.remove"] != "gone" {
				t.Fatal("lost or mutated original metadata")
			}
			if !tc.manual && (put.Config["user.remove"] != "" || put.Config["user.add"] != "new" || len(keys) < 2) {
				t.Fatal(put, keys)
			}
		})
	}
	g, f := volumeGarden(), existingBackend(t)
	f.snapshot.Volumes["pool"] = []api.StorageVolume{{Name: "data", Type: "custom", ContentType: "block", StorageVolumePut: api.StorageVolumePut{Config: api.ConfigMap{definition.OwnerKey: definition.Owner, "size": "1GiB"}}}}
	p, err := Build(g, f)
	if err != nil || len(p.Manual) == 0 {
		t.Fatalf("missing manual action: %v %+v", err, p)
	}
	if err := p.Apply(f, io.Discard); err == nil || len(f.executed) != 0 {
		t.Fatal("manual action did not block whole apply")
	}
}

func TestVolumeOwnershipAdoptionAndExclusiveUse(t *testing.T) {
	g, f := volumeGarden(), existingBackend(t)
	f.snapshot.Volumes["pool"] = []api.StorageVolume{{Name: "data", Type: "custom", ContentType: "filesystem", StorageVolumePut: api.StorageVolumePut{Description: "external", Config: api.ConfigMap{"user.external": "keep"}}}}
	if _, err := Build(g, f); err == nil {
		t.Fatal("unowned accepted")
	}
	p, err := BuildAdoption(g, Target{Pool: "pool", Name: "data"}, f)
	if err != nil || len(p.Operations) != 1 || p.Operations[0].Kind != AdoptVolume {
		t.Fatalf("%v %+v", err, p)
	}
	if err := p.Apply(f, io.Discard); err != nil {
		t.Fatal(err)
	}
	actual := f.snapshot.Volumes["pool"][0]
	if actual.Description != "external" || actual.Config["user.external"] != "keep" {
		t.Fatal("adoption reconciled metadata")
	}
	for _, uses := range [][]string{{"/1.0/instances/other?project=apps"}, {"/1.0/profiles/default"}, {"/1.0/instances/wiki?project=other"}} {
		f.snapshot.Volumes["pool"][0].UsedBy = uses
		if _, err := Build(g, f); err == nil {
			t.Fatal("accepted other attachment", uses)
		}
	}
	for _, owner := range []string{"other", definition.Owner} {
		f.snapshot.Volumes["pool"][0].Config[definition.OwnerKey] = owner
		_, err := BuildAdoption(g, Target{Pool: "pool", Name: "data"}, f)
		if owner == "other" && err == nil {
			t.Fatal("foreign owner adopted")
		}
	}
}

func TestDiskAdditionRestartAndBindingProtection(t *testing.T) {
	g, f := volumeGarden(), existingBackend(t)
	p, err := Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{CreateVolume, ReconfigureVM}) {
		t.Fatalf("%v %+v", err, p)
	}
	f.snapshot.Instances["apps"][0].Status = "Stopped"
	p, err = Build(g, f)
	if err != nil || !reflect.DeepEqual(kinds(p), []Kind{CreateVolume, UpdateVM, StartVM}) {
		t.Fatalf("%v %+v", err, p)
	}
	f.snapshot.Instances["apps"][0].Devices["data"] = map[string]string{"type": "disk", "pool": "pool", "source": "other", "path": "/data"}
	p, err = Build(g, f)
	if err != nil || len(p.Manual) == 0 {
		t.Fatalf("binding replaced: %v %+v", err, p)
	}
	d := DiskDevice(g.VMs[0].Disks[0])
	d["dependent"] = "true"
	if SameDiskBinding(d, g.VMs[0].Disks[0]) {
		t.Fatal("dependent disk accepted")
	}
	if !strings.Contains(p.Manual[0], "data") {
		t.Fatal(p.Manual)
	}
}
