// Package gui provides shared HTTP machinery for app GUIs: the MASS connection
// settings control (endpoint/token/custom CA) that validates against the
// gateway and persists per endpoint via the connstore package.
package gui

import "context"

// ValidatorInterface is the minimal interface the connection settings need from
// whatever client an app uses to talk to its gateway: call a known endpoint and
// return nil iff the configured endpoint+token are accepted.
type ValidatorInterface interface {
	Validate(ctx context.Context) error
}
