// Package jsonutil provides JSON marshaling compatible with Python's
// json.dumps(sort_keys=True). Stage and pipeline IDs are SHA-256 hashes of
// their JSON description, so byte-identical output is required for the Go
// and Python implementations to agree on cache keys.
package jsonutil

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Marshal produces JSON byte-identical to Python's json.dumps(v, sort_keys=True).encode().
// Key difference from Go's json.Marshal: uses ", " and ": " separators (with spaces)
// instead of Go's compact "," and ":".
func Marshal(v interface{}) ([]byte, error) {
	var b strings.Builder
	if err := marshalValue(&b, v); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func marshalValue(b *strings.Builder, v interface{}) error {
	switch val := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if val {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case json.Number:
		b.WriteString(val.String())
	case int:
		b.WriteString(strconv.Itoa(val))
	case int64:
		b.WriteString(strconv.FormatInt(val, 10))
	case float64:
		if math.IsInf(val, 0) || math.IsNaN(val) {
			return fmt.Errorf("json: unsupported value: %v", val)
		}
		if val == math.Trunc(val) && !math.IsInf(val, 0) && math.Abs(val) < 1e18 {
			b.WriteString(strconv.FormatInt(int64(val), 10))
		} else {
			b.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		}
	case string:
		marshalString(b, val)
	case map[string]interface{}:
		if err := marshalMap(b, val); err != nil {
			return err
		}
	case []interface{}:
		if err := marshalSlice(b, val); err != nil {
			return err
		}
	case *string:
		if val == nil {
			b.WriteString("null")
		} else {
			marshalString(b, *val)
		}
	case *int:
		if val == nil {
			b.WriteString("null")
		} else {
			b.WriteString(strconv.Itoa(*val))
		}
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return err
		}
		b.Write(data)
	}
	return nil
}

func marshalString(b *strings.Builder, s string) {
	data, _ := json.Marshal(s)
	b.Write(data)
}

func marshalMap(b *strings.Builder, m map[string]interface{}) error {
	b.WriteByte('{')
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		marshalString(b, k)
		b.WriteString(": ")
		if err := marshalValue(b, m[k]); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func marshalSlice(b *strings.Builder, s []interface{}) error {
	b.WriteByte('[')
	for i, v := range s {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := marshalValue(b, v); err != nil {
			return err
		}
	}
	b.WriteByte(']')
	return nil
}
