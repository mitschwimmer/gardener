package definition

import "testing"

func TestVolumeValidation(t *testing.T) {
	valid := func() Garden {
		return Garden{Volumes: []Volume{{Pool: "pool", Name: "data", ContentType: "block", Size: "1GiB"}}}
	}
	for _, mutate := range []func(*Garden){
		func(g *Garden) { g.Volumes[0].Pool = "" },
		func(g *Garden) { g.Volumes = append(g.Volumes, g.Volumes[0]) },
		func(g *Garden) { g.Volumes[0].ContentType = "iso" },
		func(g *Garden) { g.Volumes[0].Size = "0GiB" },
		func(g *Garden) { g.Volumes[0].Size = "9999999999999999GiB" },
		func(g *Garden) { g.Volumes[0].ContentType = "filesystem" },
		func(g *Garden) { g.Volumes[0].Config = Properties{OwnerKey: Value("other")} },
		func(g *Garden) { g.VMs = []VM{{Disks: []Disk{{Name: "data", Pool: "pool", Volume: "missing"}}}} },
	} {
		g := valid()
		mutate(&g)
		if err := g.Validate(); err == nil {
			t.Fatalf("invalid accepted: %+v", g)
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"-1", "0", "1GB", "18446744073709551615"} {
		if _, err := VolumeSizeBytes(value); err == nil {
			t.Fatal(value)
		}
	}
}

func TestDiskDefinitionValidation(t *testing.T) {
	for _, change := range []func(*Garden){
		func(g *Garden) { g.VMs[0].Disks[0].Name = "root" },
		func(g *Garden) { g.VMs[0].Disks[0].Path = "relative" },
		func(g *Garden) { g.VMs[0].Disks[0].Path = "/" },
		func(g *Garden) { g.VMs[0].Disks[0].Path = "/data/../other" },
		func(g *Garden) { g.VMs[0].Disks = append(g.VMs[0].Disks, g.VMs[0].Disks[0]) },
		func(g *Garden) { other := g.VMs[0]; other.Name = "other"; g.VMs = append(g.VMs, other) },
		func(g *Garden) { g.Volumes[0].ContentType = "block"; g.Volumes[0].Size = "1GiB" },
	} {
		g := valid()
		g.Volumes = []Volume{{Pool: "pool", Name: "data", ContentType: "filesystem"}}
		g.VMs[0].Disks = []Disk{{Name: "data", Pool: "pool", Volume: "data", Path: "/data"}}
		if err := g.Validate(); err != nil {
			t.Fatal(err)
		}
		change(&g)
		if err := g.Validate(); err == nil {
			t.Fatal("invalid disk definition accepted")
		}
	}
}
