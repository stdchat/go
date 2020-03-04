package service

import (
	"math/bits"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
)

const digits64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// Uint64ID writes an ID string to dest based on n.
// Returns the number of bytes written.
func Uint64ID(dest *strings.Builder, n uint64) int {
	if n < 64 {
		dest.WriteByte(digits64[n])
		return 1
	}
	var buf [16]byte
	buflen := 0
	for n > 0 {
		buf[buflen] = digits64[n%64]
		buflen++
		n /= 64
	}
	for i := buflen - 1; i >= 0; i-- {
		dest.WriteByte(buf[i])
	}
	return buflen
}

var prevID uint32 // atomic

var xorID = func() uint64 {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() ^ 3594538))
	return uint64(rng.Int63()) >> 24
}()

// reqID can be an ID that the new ID will be in response to, or empty.
func MakeID(reqID string) string {
	idnum := uint64(atomic.AddUint32(&prevID, 1))
	idtime := uint64(bits.ReverseBytes32(uint32(time.Now().Unix())))
	id := idtime<<8 ^ idnum ^ xorID // obfuscate
	sb := &strings.Builder{}
	if reqID != "" {
		sb.WriteString(reqID)
		sb.WriteByte('@')
	}
	Uint64ID(sb, id)
	return sb.String()
}
