package portableupdate

import "fmt"

// PathError identifies a rejected boundary without weakening its validation.
type PathError struct {
	Code   string
	Path   string
	Reason string
}

func (e *PathError) Error() string {
	return fmt.Sprintf("%s at %s; use a private user-owned folder", e.Reason, e.Path)
}
