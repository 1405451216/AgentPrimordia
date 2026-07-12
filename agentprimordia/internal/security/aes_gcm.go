package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

const (
	aesKeySize     = 32
	nonceSize      = 12
	keyVersionSize = 4
)

var (
	ErrInvalidKeySize    = errors.New("invalid AES key size: must be 32 bytes for AES-256")
	ErrInvalidCiphertext = errors.New("ciphertext too short")
	ErrKeyMismatch       = errors.New("wrong key: authentication failed")
)

type AESGCMEncryptor struct {
	mu     sync.RWMutex
	key    []byte
	keyVer uint32
	kek    []byte
}

func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	if len(key) != aesKeySize {
		return nil, ErrInvalidKeySize
	}
	kek := make([]byte, len(key))
	copy(kek, key)
	dek := deriveDEK(kek, 1)
	return &AESGCMEncryptor{
		key:    dek,
		keyVer: 1,
		kek:    kek,
	}, nil
}

func deriveDEK(kek []byte, version uint32) []byte {
	dek := make([]byte, aesKeySize)
	copy(dek, kek)
	bb := make([]byte, 4)
	binary.BigEndian.PutUint32(bb, version)
	for i := 0; i < aesKeySize; i++ {
		dek[i] ^= bb[i%4]
	}
	return dek
}

func (e *AESGCMEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	e.mu.RLock()
	key := make([]byte, len(e.key))
	copy(key, e.key)
	ver := e.keyVer
	e.mu.RUnlock()

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, 0, keyVersionSize+nonceSize+len(ct))
	bv := make([]byte, keyVersionSize)
	binary.BigEndian.PutUint32(bv, ver)
	result = append(result, bv...)
	result = append(result, nonce...)
	result = append(result, ct...)
	return result, nil
}

func (e *AESGCMEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < keyVersionSize+nonceSize {
		return nil, ErrInvalidCiphertext
	}
	ver := binary.BigEndian.Uint32(ciphertext[:keyVersionSize])
	nonce := ciphertext[keyVersionSize : keyVersionSize+nonceSize]
	ct := ciphertext[keyVersionSize+nonceSize:]

	e.mu.RLock()
	kek := make([]byte, len(e.kek))
	copy(kek, e.kek)
	currentVer := e.keyVer
	e.mu.RUnlock()

	var key []byte
	if ver == currentVer {
		e.mu.RLock()
		key = make([]byte, len(e.key))
		copy(key, e.key)
		e.mu.RUnlock()
	} else {
		key = deriveDEK(kek, ver)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func (e *AESGCMEncryptor) RotateKey() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	newVer := e.keyVer + 1
	newDEK := deriveDEK(e.kek, newVer)
	subtle.ConstantTimeCopy(len(e.key), e.key, make([]byte, len(e.key)))
	e.key = newDEK
	e.keyVer = newVer
	return nil
}

func (e *AESGCMEncryptor) KeyVersion() uint32 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.keyVer
}
