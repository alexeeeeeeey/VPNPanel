package xray

import (
	"encoding/json"
	"testing"
)

func TestCounterValueProtobufJSON(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int64
	}{{"", 0}, {"0", 0}, {`"0"`, 0}, {"123", 123}, {`"123"`, 123}} {
		value, err := counterValue(json.RawMessage(tc.raw))
		if err != nil || value != tc.want {
			t.Fatalf("counter %q = %d, %v; want %d", tc.raw, value, err, tc.want)
		}
	}
	for _, raw := range []string{"-1", `"oops"`, "null"} {
		if _, err := counterValue(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid counter %q", raw)
		}
	}
}
