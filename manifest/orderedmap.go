package manifest

// OrderedMap preserves insertion order, matching Python dict behavior.
type OrderedMap[V any] struct {
	keys   []string
	values map[string]V
}

func NewOrderedMap[V any]() OrderedMap[V] {
	return OrderedMap[V]{values: make(map[string]V)}
}

func (m *OrderedMap[V]) Set(key string, value V) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *OrderedMap[V]) Get(key string) (V, bool) {
	v, ok := m.values[key]
	return v, ok
}

func (m *OrderedMap[V]) Delete(key string) {
	if _, ok := m.values[key]; !ok {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *OrderedMap[V]) Keys() []string {
	return m.keys
}

func (m *OrderedMap[V]) Len() int {
	return len(m.keys)
}

func (m *OrderedMap[V]) Range(fn func(key string, value V) bool) {
	for _, k := range m.keys {
		if !fn(k, m.values[k]) {
			break
		}
	}
}
