package platform

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// SystemClock implements usecase.Clock with the wall clock, in UTC.
type SystemClock struct{}

// Now returns the current time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// UUIDGenerator implements usecase.IDGenerator with random (version 4) UUIDs,
// the format of the id columns of the database.
type UUIDGenerator struct{}

// NewID returns a new random UUID such as 3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f10.
func (UUIDGenerator) NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// Unreachable on supported Go versions (the runtime aborts first). A
		// predictable id would be worse than stopping, so never fall back.
		panic(fmt.Sprintf("system entropy source failed: %v", err))
	}
	raw[6] = raw[6]&0x0f | 0x40 // version 4
	raw[8] = raw[8]&0x3f | 0x80 // RFC 4122 variant
	encoded := hex.EncodeToString(raw[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
