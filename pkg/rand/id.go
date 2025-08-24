package rand

import (
	cr "crypto/rand"
	"encoding/base32"
	"io"
)

func ID16() string {
	var b [10]byte // 10 raw bytes → 16 base32 chars
	_, _ = cr.Read(b[:])
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

// Password generates a random alphanumeric password of a given length.
func Password(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := io.ReadFull(cr.Reader, b); err != nil {
		// This should not happen in a healthy system
		panic("failed to read from crypto/rand: " + err.Error())
	}

	for i, v := range b {
		b[i] = chars[int(v)%len(chars)]
	}
	return string(b)
}
