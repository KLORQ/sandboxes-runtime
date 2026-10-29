package runtime

import "testing"

func TestCapabilitiesZeroValue(t *testing.T) {
	var caps Capabilities
	if caps.IPv6 {
		t.Fatal("IPv6 must be opt-in")
	}
}
