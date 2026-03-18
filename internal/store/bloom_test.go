package store

import (
	"testing"
)

func TestBloom_AddContains(t *testing.T) {
	b := NewBloom(10000, 0.001)

	id1 := [32]byte{1, 2, 3}
	id2 := [32]byte{4, 5, 6}

	b.Add(id1)

	if !b.Contains(id1) {
		t.Fatal("added ID not found")
	}
	if b.Contains(id2) {
		t.Fatal("non-added ID found (unlikely false positive with empty filter)")
	}
}

func TestBloom_ManyItems(t *testing.T) {
	b := NewBloom(10000, 0.001)

	// Add 1000 items
	for i := 0; i < 1000; i++ {
		var id [32]byte
		id[0] = byte(i)
		id[1] = byte(i >> 8)
		b.Add(id)
	}

	// Check all 1000 are found
	for i := 0; i < 1000; i++ {
		var id [32]byte
		id[0] = byte(i)
		id[1] = byte(i >> 8)
		if !b.Contains(id) {
			t.Fatalf("item %d not found", i)
		}
	}

	// Check false positive rate with 1000 non-added items
	falsePositives := 0
	for i := 1000; i < 2000; i++ {
		var id [32]byte
		id[0] = byte(i)
		id[1] = byte(i >> 8)
		id[2] = 0xFF // Different from added items
		if b.Contains(id) {
			falsePositives++
		}
	}
	rate := float64(falsePositives) / 1000.0
	t.Logf("false positive rate: %.4f (%d/1000)", rate, falsePositives)
	if rate > 0.05 { // Allow up to 5% for this small test
		t.Fatalf("false positive rate too high: %.4f", rate)
	}
}

func TestBloom_Export(t *testing.T) {
	b := NewBloom(1000, 0.001)
	id := [32]byte{42}
	b.Add(id)

	exported := b.Export()
	if len(exported) == 0 {
		t.Fatal("export returned empty data")
	}
}
