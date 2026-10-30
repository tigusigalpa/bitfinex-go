package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestJSONValueExactAndDetached(t *testing.T) {
	v, e := ParseJSONValue([]byte(` {"secret":900719925474099312345,"fraction":0.123456789012345678901,"list":[true,null,"x"]} `))
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(v)
	if e != nil || !strings.Contains(string(b), "900719925474099312345") || !strings.Contains(string(b), "0.123456789012345678901") {
		t.Fatalf("%s %v", b, e)
	}
	obj, ok := v.Object()
	if !ok {
		t.Fatal("object")
	}
	d, ok := obj["fraction"].Number()
	if !ok || d.String() != "0.123456789012345678901" {
		t.Fatal(d)
	}
	a, ok := obj["list"].Array()
	if !ok || len(a) != 3 || !a[1].IsNull() {
		t.Fatal(a)
	}
	if yes, ok := a[0].Boolean(); !ok || !yes {
		t.Fatal("bool")
	}
	if text, ok := a[2].Text(); !ok || text != "x" {
		t.Fatal("text")
	}
	delete(obj, "secret")
	b, _ = json.Marshal(v)
	if !strings.Contains(string(b), "secret") {
		t.Fatal("alias")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", v, v), "900719") {
		t.Fatal("format exposed settings")
	}
	for _, raw := range []string{"", `1 2`, `{"a":}`, strings.Repeat(" ", 1048577)} {
		if _, e := ParseJSONValue([]byte(raw)); e == nil {
			t.Fatalf("accepted invalid JSON length %d", len(raw))
		}
	}
	var zero JSONValue
	b, e = json.Marshal(zero)
	if e != nil || string(b) != "null" {
		t.Fatal(string(b), e)
	}
}

func FuzzJSONValue(f *testing.F) {
	for _, s := range []string{`null`, `{"x":9007199254740993}`, `[1,"x"]`, `{"x":`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, e := ParseJSONValue([]byte(s))
		if e != nil {
			return
		}
		b, e := json.Marshal(v)
		if e != nil || !json.Valid(b) {
			t.Fatalf("invalid roundtrip: %v", e)
		}
	})
}
