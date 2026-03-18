package mesh

import (
	"testing"
)

func TestBeacon_Roundtrip(t *testing.T) {
	pubKey := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	d := NewDiscovery(pubKey, 8080, NewPeerList())
	beacon := d.makeBeacon()

	if len(beacon) != BeaconSize {
		t.Fatalf("beacon size = %d, expected %d", len(beacon), BeaconSize)
	}

	parsedPub, port, version, err := parseBeacon(beacon)
	if err != nil {
		t.Fatalf("parseBeacon failed: %v", err)
	}
	if parsedPub != pubKey {
		t.Fatal("pubkey mismatch")
	}
	if port != 8080 {
		t.Fatalf("port = %d, expected 8080", port)
	}
	if version != 1 {
		t.Fatalf("version = %d, expected 1", version)
	}
}

func TestBeacon_InvalidMagic(t *testing.T) {
	data := make([]byte, BeaconSize)
	copy(data[0:6], "WRONG!")
	_, _, _, err := parseBeacon(data)
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestBeacon_TooShort(t *testing.T) {
	_, _, _, err := parseBeacon([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short beacon")
	}
}

func TestPeerList_AddAndGet(t *testing.T) {
	pl := NewPeerList()
	key := [32]byte{42}

	pl.AddOrUpdate(key, "192.168.1.1", 8080)

	peer := pl.Get(key)
	if peer == nil {
		t.Fatal("peer not found")
	}
	if peer.IP != "192.168.1.1" {
		t.Fatalf("IP = %s, expected 192.168.1.1", peer.IP)
	}
	if peer.Port != 8080 {
		t.Fatalf("Port = %d, expected 8080", peer.Port)
	}
	if pl.Count() != 1 {
		t.Fatalf("count = %d, expected 1", pl.Count())
	}
}

func TestPeerList_Update(t *testing.T) {
	pl := NewPeerList()
	key := [32]byte{42}

	pl.AddOrUpdate(key, "192.168.1.1", 8080)
	pl.AddOrUpdate(key, "192.168.1.2", 9090)

	peer := pl.Get(key)
	if peer.IP != "192.168.1.2" {
		t.Fatal("IP not updated")
	}
	if pl.Count() != 1 {
		t.Fatal("should still be 1 peer")
	}
}
