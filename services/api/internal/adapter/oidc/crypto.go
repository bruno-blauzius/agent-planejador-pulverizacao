package oidc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// Keyring seals refresh tokens and the login flow cookie with AES-256-GCM.
// Several keys may be loaded at once so a rotation never logs anyone out:
// new data uses the current key, old data is read with the key id stored next
// to it. The flow cookie key is derived (HKDF) from the same material, so one
// secret never serves two purposes directly.
type Keyring struct {
	currentID int16
	sessions  map[int16]cipher.AEAD
	flows     map[int16]cipher.AEAD
}

// NewKeyring loads the current key and, optionally, older ones. Every key must
// have exactly 32 bytes and ids must be unique and positive.
func NewKeyring(currentID int16, current []byte, old map[int16][]byte) (*Keyring, error) {
	if currentID < 1 {
		return nil, errors.New("encryption key id must be positive")
	}
	ring := &Keyring{currentID: currentID, sessions: map[int16]cipher.AEAD{}, flows: map[int16]cipher.AEAD{}}
	if err := ring.add(currentID, current); err != nil {
		return nil, err
	}
	for id, key := range old {
		if id == currentID {
			return nil, fmt.Errorf("encryption key id %d is both current and old", id)
		}
		if id < 1 {
			return nil, errors.New("encryption key id must be positive")
		}
		if err := ring.add(id, key); err != nil {
			return nil, err
		}
	}
	return ring, nil
}

func (k *Keyring) add(id int16, key []byte) error {
	if len(key) != encryptionKeyBytes {
		return fmt.Errorf("encryption key %d must have %d bytes (base64 of 32 random bytes)", id, encryptionKeyBytes)
	}
	session, err := newGCM(key)
	if err != nil {
		return err
	}
	flowKey, err := hkdf.Key(sha256.New, key, nil, "agentes-agro/oidc-flow-cookie/v1", encryptionKeyBytes)
	if err != nil {
		return fmt.Errorf("derive flow key: %w", err)
	}
	flow, err := newGCM(flowKey)
	if err != nil {
		return err
	}
	k.sessions[id], k.flows[id] = session, flow
	return nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return aead, nil
}

// Seal encrypts plain with the current key, binding it to aad (the session id),
// so a ciphertext copied to another session row does not open.
func (k *Keyring) Seal(plain, aad []byte) ([]byte, int16, error) {
	sealed, err := seal(k.sessions[k.currentID], plain, aad)
	if err != nil {
		return nil, 0, err
	}
	return sealed, k.currentID, nil
}

// Open decrypts data sealed with the key identified by keyID.
func (k *Keyring) Open(sealed []byte, keyID int16, aad []byte) ([]byte, error) {
	aead, ok := k.sessions[keyID]
	if !ok {
		return nil, fmt.Errorf("unknown encryption key id %d", keyID)
	}
	return open(aead, sealed, aad)
}

// sealFlow encrypts the login flow state into a cookie value. The first two
// bytes carry the key id.
func (k *Keyring) sealFlow(plain []byte) (string, error) {
	sealed, err := seal(k.flows[k.currentID], plain, nil)
	if err != nil {
		return "", err
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(k.currentID&0x7fff))
	out = append(out, sealed...)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func (k *Keyring) openFlow(value string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < 2 {
		return nil, errors.New("malformed flow cookie")
	}
	id := int16(binary.BigEndian.Uint16(raw) & 0x7fff)
	aead, ok := k.flows[id]
	if !ok {
		return nil, errors.New("flow cookie sealed with an unknown key")
	}
	return open(aead, raw[2:], nil)
}

func seal(aead cipher.AEAD, plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read random nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plain, aad), nil
}

func open(aead cipher.AEAD, sealed, aad []byte) ([]byte, error) {
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("sealed data too short")
	}
	nonce, body := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, body, aad)
	if err != nil {
		return nil, errors.New("sealed data does not open") // never include crypto detail
	}
	return plain, nil
}

// randomToken returns n random bytes as unpadded base64url (256 bits for n=32).
func randomToken(n int) (string, error) {
	buffer := make([]byte, n)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
