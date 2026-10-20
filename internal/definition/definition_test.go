package definition

import (
	"strings"
	"testing"
)

func valid() Garden {
	return Garden{Projects: []Project{{Name: "apps"}}, VMs: []VM{{Project: "apps", Name: "wiki", Image: strings.Repeat("a", 64), Pool: "pool", Network: "bridge"}}}
}

func TestValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Garden)
		want   string
	}{
		{"undeclared project", func(g *Garden) { g.Projects = nil }, "undeclared project"},
		{"duplicate project", func(g *Garden) { g.Projects = append(g.Projects, g.Projects[0]) }, "duplicate project"},
		{"duplicate VM", func(g *Garden) { g.VMs = append(g.VMs, g.VMs[0]) }, "duplicate VM"},
		{"reserved project", func(g *Garden) { g.Projects[0].Name = "default" }, "reserved"},
		{"image alias", func(g *Garden) { g.VMs[0].Image = "nixos/latest" }, "fingerprint"},
		{"pool missing", func(g *Garden) { g.VMs[0].Pool = "" }, "pool"},
		{"network missing", func(g *Garden) { g.VMs[0].Network = "" }, "network"},
		{"power", func(g *Garden) { g.VMs[0].Power = "paused" }, "power"},
		{"cpu pinning", func(g *Garden) { g.VMs[0].Config = Properties{"limits.cpu": Value("0-3")} }, "CPU count"},
		{"zero cpu", func(g *Garden) { g.VMs[0].Config = Properties{"limits.cpu": Value("0")} }, "CPU count"},
		{"memory percentage", func(g *Garden) { g.VMs[0].Config = Properties{"limits.memory": Value("50%")} }, "MiB or GiB"},
		{"reserved ownership", func(g *Garden) { g.VMs[0].Config = Properties{OwnerKey: nil} }, "cannot manage"},
		{"arbitrary configuration", func(g *Garden) { g.VMs[0].Config = Properties{"security.secureboot": Value("false")} }, "cannot manage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := valid()
			test.change(&g)
			if err := g.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %s", err, test.want)
			}
		})
	}
	g := valid()
	g.VMs[0].Config = Properties{"limits.cpu": Value("2"), "limits.memory": Value("2GiB"), "user.note": nil}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Garden{}).Validate(); err != nil {
		t.Fatal(err)
	}
}
