package membership

import "testing"

func TestStatuses(t *testing.T) {
	for _, s := range []Status{Pending, Active, Suspended, Revoked} {
		if s.Validate() != nil {
			t.Fatal(s)
		}
	}
	if Status("bad").Validate() == nil {
		t.Fatal("bad status")
	}
}
