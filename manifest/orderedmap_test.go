package manifest

import (
	"testing"
)

func TestOrderedMapInsertionOrder(t *testing.T) {
	m := NewOrderedMap[int]()
	m.Set("c", 3)
	m.Set("a", 1)
	m.Set("b", 2)

	got := m.Keys()
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("Keys() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOrderedMapOverwrite(t *testing.T) {
	m := NewOrderedMap[int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 10)

	if m.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", m.Len())
	}
	v, ok := m.Get("a")
	if !ok || v != 10 {
		t.Errorf("Get(a) = %d, %v, want 10, true", v, ok)
	}
	if m.Keys()[0] != "a" {
		t.Errorf("overwrite should preserve original position")
	}
}

func TestOrderedMapDelete(t *testing.T) {
	m := NewOrderedMap[string]()
	m.Set("x", "X")
	m.Set("y", "Y")
	m.Set("z", "Z")
	m.Delete("y")

	if m.Len() != 2 {
		t.Fatalf("Len() = %d after delete, want 2", m.Len())
	}
	_, ok := m.Get("y")
	if ok {
		t.Error("Get(y) should return false after delete")
	}
	got := m.Keys()
	want := []string{"x", "z"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOrderedMapRange(t *testing.T) {
	m := NewOrderedMap[int]()
	m.Set("first", 1)
	m.Set("second", 2)
	m.Set("third", 3)

	var visited []string
	m.Range(func(key string, _ int) bool {
		visited = append(visited, key)
		return true
	})
	if len(visited) != 3 || visited[0] != "first" || visited[2] != "third" {
		t.Errorf("Range visited %v, want [first second third]", visited)
	}
}

func TestOrderedMapRangeEarlyStop(t *testing.T) {
	m := NewOrderedMap[int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	count := 0
	m.Range(func(_ string, _ int) bool {
		count++
		return count < 2
	})
	if count != 2 {
		t.Errorf("Range stopped after %d, want 2", count)
	}
}
