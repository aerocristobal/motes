package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const base36Chars = "0123456789abcdefghijklmnopqrstuvwxyz"

func base36Encode(n int64) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		n = -n
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base36Chars[n%36]
		n /= 36
	}
	return string(buf[i:])
}

// GenerateID creates a unique mote ID: <scope>-<typechar><base36-timestamp><random-suffix>
func GenerateID(scope, moteType string) string {
	typeChar := strings.ToUpper(moteType[:1])
	timestamp := base36Encode(time.Now().UnixNano())
	suffix := base36Encode(int64(cryptoRandN(1679616))) // 4 chars max (36^4)
	return fmt.Sprintf("%s-%s%s%s", scope, typeChar, timestamp, suffix)
}

func cryptoRandN(max int) int {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return int(binary.LittleEndian.Uint32(buf[:])) % max
}
