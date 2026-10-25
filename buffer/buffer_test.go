package buffer

import "testing"

func TestAppendSnapshotAndRemove(t *testing.T) {
	b := New()
	b.Append(100, 10)
	b.Append(101, 11)
	b.Append(102, 12)

	all := b.SnapshotAll()
	if len(all) != 3 {
		t.Fatalf("len=%d", len(all))
	}

	inRange := b.SnapshotRange(101, 102)
	if len(inRange) != 2 || inRange[0].Value != 11 {
		t.Fatalf("range=%v", inRange)
	}

	last, ok := b.Last()
	if !ok || last.Timestamp != 102 {
		t.Fatalf("last=%v ok=%v", last, ok)
	}

	b.RemoveThroughTimestamp(101)
	left := b.SnapshotAll()
	if len(left) != 1 || left[0].Timestamp != 102 {
		t.Fatalf("after remove=%v", left)
	}
}

func TestHealthPending(t *testing.T) {
	b := New()
	pending, age, _, errStr := b.Health()
	if pending != 0 || age != nil || errStr != "" {
		t.Fatalf("empty health: %d %v %q", pending, age, errStr)
	}

	b.Append(1, 1.5)
	pending, age, _, _ = b.Health()
	if pending != 1 || age == nil {
		t.Fatalf("pending health: %d %v", pending, age)
	}
}
