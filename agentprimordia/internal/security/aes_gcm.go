package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

// deriveDEK 使用 HKDF-SHA256 从 KEK 和版本号派生数据加密密钥。
// HKDF 是 NIST SP 800-56C 推荐的标准密钥派生函数，
// 相比 XOR 方案可抵抗已知明文攻击和密钥相关性攻击。
// 实现基于 crypto/hmac + sha256，遵循 RFC 5869。
func deriveDEK(kek []byte, version uint32) []byte {
	// HKDF-Extract: PRK = HMAC-Hash(salt, IKM)
	// salt 为 nil 时使用 Hash 输出长度的零值（SHA-256 为 32 字节零值）
	prkHMAC := hmac.New(sha256.New, nil)
	prkHMAC.Write(kek)
	prk := prkHMAC.Sum(nil)

	// HKDF-Expand: OKM = T(1) || T(2) || ...
	// T(i) = HMAC-Hash(PRK, T(i-1) || info || i)
	var okm []byte
	var prev []byte
	info := make([]byte, 8)
	binary.BigEndian.PutUint32(info[:4], version)
	binary.BigEndian.PutUint32(info[4:], uint32(aesKeySize))

	for i := byte(1); len(okm) < aesKeySize; i++ {
		h := hmac.New(sha256.New, prk)
		h.Write(prev)
		h.Write(info)
		h.Write([]byte{i})
		prev = h.Sum(nil)
		okm = append(okm, prev...)
	}
	return okm[:aesKeySize]
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
