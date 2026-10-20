// Package garden contains the operator's complete infrastructure definitions.
// Organize workloads in separate Go files and assemble them in Definitions.
package garden

import "github.com/mitschwimmer/gardener/internal/definition"

// Definitions deliberately starts empty. Add actual operator references here;
// an empty garden reports owned orphans and never deletes them.
func Definitions() definition.Garden {
	return definition.Garden{}
}
