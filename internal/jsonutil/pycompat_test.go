package jsonutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestMarshalScalars(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{"nil", nil, "null"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"empty_string", "", `""`},
		{"string", "hello", `"hello"`},
		{"module_name", "org.osbuild.rpm", `"org.osbuild.rpm"`},
		{"int", 42, "42"},
		{"int64", int64(1659397331), "1659397331"},
		{"float_whole", float64(42), "42"},
		{"float_frac", float64(3.14), "3.14"},
		{"json_number_int", json.Number("42"), "42"},
		{"json_number_large", json.Number("1659397331"), "1659397331"},
		{"nil_ptr_string", (*string)(nil), "null"},
		{"nil_ptr_int", (*int)(nil), "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.val)
			if err != nil {
				t.Fatalf("Marshal(%v) error: %v", tt.val, err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%v) = %q, want %q", tt.val, string(got), tt.want)
			}
		})
	}
}

func TestMarshalPtrString(t *testing.T) {
	s := "abc123"
	got, err := Marshal(&s)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"abc123"` {
		t.Errorf("got %q, want %q", string(got), `"abc123"`)
	}
}

func TestMarshalContainers(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want string
	}{
		{"empty_map", map[string]interface{}{}, "{}"},
		{"empty_slice", []interface{}{}, "[]"},
		{
			"map_sorted_keys",
			map[string]interface{}{"b": json.Number("1"), "a": []interface{}{json.Number("2"), json.Number("3")}},
			`{"a": [2, 3], "b": 1}`,
		},
		{
			"nested_map",
			map[string]interface{}{
				"disable_dracut": true,
				"gpgkeys":        []interface{}{"key1"},
			},
			`{"disable_dracut": true, "gpgkeys": ["key1"]}`,
		},
		{
			"slice_of_strings",
			[]interface{}{"a", "b", "c"},
			`["a", "b", "c"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.val)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestStageIDSimple(t *testing.T) {
	h := sha256.New()
	write := func(v interface{}) {
		b, err := Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(b)
	}

	write("org.osbuild.rpm") // name
	write(nil)               // build (None)
	write(nil)               // base (None)
	write(map[string]interface{}{}) // options ({})

	got := hex.EncodeToString(h.Sum(nil))
	want := "ffa14005965372e389f88a29b29cb6ee52dc991e5cc9751c8e647a0f2d5a0c27"
	if got != want {
		t.Errorf("simple stage ID = %s, want %s", got, want)
	}
}

func TestStageIDWithOptions(t *testing.T) {
	h := sha256.New()
	write := func(v interface{}) {
		b, err := Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(b)
	}

	write("org.osbuild.rpm")
	write(nil)
	write(nil)
	write(map[string]interface{}{
		"disable_dracut": true,
		"gpgkeys":        []interface{}{"key1"},
	})

	got := hex.EncodeToString(h.Sum(nil))
	want := "f7061077d5fa8fd51cc6b4d2c42613ae31016c65dce8335c6ede36b2f8b722ba"
	if got != want {
		t.Errorf("options stage ID = %s, want %s", got, want)
	}
}

func TestStageIDWithEpoch(t *testing.T) {
	h := sha256.New()
	write := func(v interface{}) {
		b, err := Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(b)
	}

	write("org.osbuild.rpm")
	write(nil)
	write(nil)
	write(map[string]interface{}{})
	write(1659397331)

	got := hex.EncodeToString(h.Sum(nil))
	want := "067e33d437c838388dcddbdc4cc14d64be3517db6c31541de207a5622b98f2f3"
	if got != want {
		t.Errorf("epoch stage ID = %s, want %s", got, want)
	}
}
