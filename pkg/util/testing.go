package util

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func RandStr(t *testing.T, n int) string {
	s := ""
	for len(s) < n {
		buf := make([]byte, 16)
		m, err := rand.Read(buf)
		buf = buf[:m]
		assert.Nil(t, err)
		for _, b := range buf {
			if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') {
				s = s + string(b)
				if len(s) == n {
					break
				}
			}
		}
	}
	return s
}
