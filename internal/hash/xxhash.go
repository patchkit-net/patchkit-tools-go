package hash

import (
	"io"
	"os"
)

// XXH32 constants
const (
	prime32_1 uint32 = 0x9E3779B1
	prime32_2 uint32 = 0x85EBCA77
	prime32_3 uint32 = 0xC2B2AE3D
	prime32_4 uint32 = 0x27D4EB2F
	prime32_5 uint32 = 0x165667B1
)

// XXH32File computes xxHash32 of a file with the given seed.
// This matches the C library's hash_file() with algorithm=32.
func XXH32File(path string, seed uint32) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return XXH32Reader(f, seed)
}

// XXH32Reader computes xxHash32 from an io.Reader.
func XXH32Reader(r io.Reader, seed uint32) (uint32, error) {
	h := newXXH32State(seed)
	buf := make([]byte, 32*1024) // 32 KB read buffer
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.update(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return h.sum(), nil
}

// XXH32Bytes computes xxHash32 of a byte slice.
func XXH32Bytes(data []byte, seed uint32) uint32 {
	h := newXXH32State(seed)
	h.update(data)
	return h.sum()
}

// xxh32State is the streaming state for xxHash32.
type xxh32State struct {
	totalLen uint64
	v1       uint32
	v2       uint32
	v3       uint32
	v4       uint32
	mem      [16]byte
	memSize  int
}

func newXXH32State(seed uint32) *xxh32State {
	return &xxh32State{
		v1: seed + prime32_1 + prime32_2,
		v2: seed + prime32_2,
		v3: seed,
		v4: seed - prime32_1,
	}
}

func (s *xxh32State) update(input []byte) {
	s.totalLen += uint64(len(input))
	p := 0

	// Fill remainder of mem buffer
	if s.memSize > 0 {
		needed := 16 - s.memSize
		if len(input) < needed {
			copy(s.mem[s.memSize:], input)
			s.memSize += len(input)
			return
		}
		copy(s.mem[s.memSize:], input[:needed])
		p = needed

		s.v1 = round32(s.v1, getU32(s.mem[0:]))
		s.v2 = round32(s.v2, getU32(s.mem[4:]))
		s.v3 = round32(s.v3, getU32(s.mem[8:]))
		s.v4 = round32(s.v4, getU32(s.mem[12:]))

		s.memSize = 0
	}

	// Process 16-byte blocks
	for p+16 <= len(input) {
		s.v1 = round32(s.v1, getU32(input[p:]))
		s.v2 = round32(s.v2, getU32(input[p+4:]))
		s.v3 = round32(s.v3, getU32(input[p+8:]))
		s.v4 = round32(s.v4, getU32(input[p+12:]))
		p += 16
	}

	// Store remaining bytes
	if p < len(input) {
		copy(s.mem[:], input[p:])
		s.memSize = len(input) - p
	}
}

func (s *xxh32State) sum() uint32 {
	var h uint32

	if s.totalLen >= 16 {
		h = rotl32(s.v1, 1) + rotl32(s.v2, 7) + rotl32(s.v3, 12) + rotl32(s.v4, 18)
	} else {
		h = s.v3 /* == seed */ + prime32_5
	}

	h += uint32(s.totalLen)

	// Process remaining bytes in mem
	p := 0
	for p+4 <= s.memSize {
		h += getU32(s.mem[p:]) * prime32_3
		h = rotl32(h, 17) * prime32_4
		p += 4
	}

	for p < s.memSize {
		h += uint32(s.mem[p]) * prime32_5
		h = rotl32(h, 11) * prime32_1
		p++
	}

	// Avalanche
	h ^= h >> 15
	h *= prime32_2
	h ^= h >> 13
	h *= prime32_3
	h ^= h >> 16

	return h
}

func round32(acc, input uint32) uint32 {
	acc += input * prime32_2
	acc = rotl32(acc, 13)
	acc *= prime32_1
	return acc
}

func rotl32(x uint32, r uint) uint32 {
	return (x << r) | (x >> (32 - r))
}

func getU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
