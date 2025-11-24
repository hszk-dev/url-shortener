package shortener

import (
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	tests := []struct {
		id       uint64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},        // Last digit
		{10, "a"},       // First lowercase letter
		{35, "z"},       // Last lowercase letter
		{36, "A"},       // First uppercase letter
		{61, "Z"},       // Last character in alphabet (single char max)
		{62, "10"},      // First two-character code
		{3843, "ZZ"},    // Repeated characters
		{12345, "3d7"},
		{18446744073709551615, "lYGhA16ahyf"}, // Max Uint64
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
	tests := []struct {
		input       string
		expectedErr string
	}{
		{"invalid_char!", "invalid character '_' at position 7 in base62 string"},
		{"abc!", "invalid character '!' at position 3 in base62 string"},
		{"123-456", "invalid character '-' at position 3 in base62 string"},
	}

	for _, test := range tests {
		_, err := Decode(test.input)
		if err == nil {
			t.Errorf("Decode(%q) expected error, got nil", test.input)
		}
		if err != nil && err.Error() != test.expectedErr {
			t.Errorf("Decode(%q) error = %q; want %q", test.input, err.Error(), test.expectedErr)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	testCases := []uint64{
		0,
		1,
		62,
		12345,
		1000000,
		18446744073709551615, // Max uint64
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(testCases[i%len(testCases)])
	}
}

func BenchmarkDecode(b *testing.B) {
	codes := []string{
		"0",
		"1",
		"10",
		"3d7",
		"4gfFC3",
		"lYGhA16ahyf",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(codes[i%len(codes)])
	}
}
