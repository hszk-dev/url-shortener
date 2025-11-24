package shortener

import (
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	tests := []struct {
		id       uint64
		expected string
	}{
		{0, "a"},
		{1, "b"},
		{61, "9"},
		{62, "ba"},
		{12345, "dnh"},
		{18446744073709551615, "v8QrKbgkrIp"}, // Max Uint64
	}

	for _, test := range tests {
		encoded := Encode(test.id)
		if encoded != test.expected {
			t.Errorf("Encode(%d) = %s; want %s", test.id, encoded, test.expected)
		}

		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode(%s) returned error: %v", encoded, err)
		}
		if decoded != test.id {
			t.Errorf("Decode(%s) = %d; want %d", encoded, decoded, test.id)
		}
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode("invalid_char!")
	if err == nil {
		t.Error("Expected error for invalid character, got nil")
	}
}
