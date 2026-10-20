// Package definition contains the typed, deliberately small infrastructure model.
package definition

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const OwnerKey = "user.garden.owner"
const Owner = "garden"

// PendingPowerKey records unfinished stop/update/start work in Incus itself.
const PendingPowerKey = "user.garden.pending-power"

// Properties manages only present keys. A nil value explicitly removes a key.
type Properties map[string]*string

func Value(s string) *string { return &s }

type Project struct {
	Name        string
	Description *string
}

type Power string

const (
	Unmanaged Power = ""
	Running   Power = "running"
	Stopped   Power = "stopped"
)

type VM struct {
	Project, Name string
	Description   *string
	// Image is a full fingerprint of an existing image in the default project.
	Image         string
	Pool, Network string
	Config        Properties
	Power         Power
	Disks         []Disk
}

type Garden struct {
	Projects []Project
	VMs      []VM
	Volumes  []Volume
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var imagePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var memoryPattern = regexp.MustCompile(`^[1-9][0-9]*(MiB|GiB)$`)

// Validate runs without contacting Incus. Empty definitions are allowed so that
// removing the last definition still reports orphans without deleting anything.
func (g Garden) Validate() error {
	if err := g.validateVolumes(); err != nil {
		return err
	}
	projects := map[string]bool{}
	for _, p := range g.Projects {
		if !namePattern.MatchString(p.Name) || p.Name == "default" {
			return fmt.Errorf("invalid managed project name %q (default is reserved)", p.Name)
		}
		if projects[p.Name] {
			return fmt.Errorf("duplicate project %q", p.Name)
		}
		projects[p.Name] = true
	}
	vms := map[string]bool{}
	for _, v := range g.VMs {
		id := v.Project + "/" + v.Name
		if !projects[v.Project] {
			return fmt.Errorf("VM %s references an undeclared project", id)
		}
		if !namePattern.MatchString(v.Name) {
			return fmt.Errorf("invalid VM name %q", v.Name)
		}
		if vms[id] {
			return fmt.Errorf("duplicate VM %s", id)
		}
		vms[id] = true
		if !imagePattern.MatchString(v.Image) {
			return fmt.Errorf("VM %s needs a full lowercase image fingerprint", id)
		}
		if v.Pool == "" || v.Network == "" {
			return fmt.Errorf("VM %s needs an existing pool and managed network", id)
		}
		if v.Power != Unmanaged && v.Power != Running && v.Power != Stopped {
			return fmt.Errorf("VM %s has invalid power state %q", id, v.Power)
		}
		for _, key := range Keys(v.Config) {
			value := v.Config[key]
			switch {
			case key == "limits.cpu":
				if value != nil {
					n, err := strconv.Atoi(*value)
					if err != nil || n < 1 {
						return fmt.Errorf("VM %s limits.cpu must be a positive CPU count", id)
					}
				}
			case key == "limits.memory":
				if value != nil && !memoryPattern.MatchString(*value) {
					return fmt.Errorf("VM %s limits.memory must be a positive MiB or GiB value", id)
				}
			case strings.HasPrefix(key, "user.") && len(key) > len("user.") && !strings.HasPrefix(key, "user.garden."):
			default:
				return fmt.Errorf("VM %s cannot manage config key %q", id, key)
			}
		}
	}
	return nil
}

func Keys(p Properties) []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
