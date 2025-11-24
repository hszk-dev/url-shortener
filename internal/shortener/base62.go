package shortener

import (
	"fmt"
	"strings"
)

const (
	alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base     = uint64(len(alphabet))
)

// Encode converts a unique integer ID to a Base62 string.
func Encode(id uint64) string {
	if id == 0 {
		return string(alphabet[0])
	}

	var sb strings.Builder
	for id > 0 {
		remainder := id % base
		sb.WriteByte(alphabet[remainder])
		id = id / base
	}

	// Reverse the string because we constructed it backwards
	chars := []byte(sb.String())
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}

	return string(chars)
}

// Decode converts a Base62 string back to a unique integer ID.
func Decode(encoded string) (uint64, error) {
	var id uint64

	for i, char := range encoded {
		index := strings.IndexRune(alphabet, char)
		if index == -1 {
			return 0, fmt.Errorf("invalid character '%c' at position %d in base62 string", char, i)
		}
		id = id*base + uint64(index)
	}

	return id, nil
}
