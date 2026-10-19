package realtime

import "testing"

func TestChannelNaming(t *testing.T) {
	if got := TrackChannel("A1"); got != "angkut:track:A1" {
		t.Errorf("TrackChannel = %q", got)
	}
	if got := OrgShipmentsChannel("O1"); got != "angkut:org:O1:shipments" {
		t.Errorf("OrgShipmentsChannel = %q", got)
	}
	ps := Patterns()
	if len(ps) != 2 {
		t.Fatalf("Patterns = %v", ps)
	}
}
