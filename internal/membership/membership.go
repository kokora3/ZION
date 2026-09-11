// Package membership defines chain-governed membership data, not admission policy.
package membership

import (
	"fmt"
	"github.com/kokora3/zion/internal/identity"
)

type Status string

const (
	Pending   Status = "PENDING"
	Active    Status = "ACTIVE"
	Suspended Status = "SUSPENDED"
	Revoked   Status = "REVOKED"
)

func (s Status) Validate() error {
	if s != Pending && s != Active && s != Suspended && s != Revoked {
		return fmt.Errorf("invalid membership status %q", s)
	}
	return nil
}

type Record struct {
	IdentityID identity.IdentityID `cbor:"1,keyasint"`
	Status     Status              `cbor:"2,keyasint"`
}

func (r Record) Validate() error {
	if e := r.IdentityID.Validate(); e != nil {
		return e
	}
	return r.Status.Validate()
}
