package security

import (
	"bytes"
	"testing"
)

func TestNewAESGCMEncryptor_InvalidKey(t *testing.T) {
	_, err := NewAESGCMEncryptor([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short key, got nil")
	}
	_, err = NewAESGCMEncryptor([]byte("1234567890123456"))
	if err == nil {
		t.Fatal("expected error for 16-byte key, got nil")
	}
	_, err = NewAESGCMEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("32-byte key should be valid, got: %v", err)
	}
}

func TestAESGCMEncryptor_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}
	plaintext := []byte("hello world, this is a secret message!")
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext equals plaintext")
	}
	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestAESGCMEncryptor_UniqueCiphertext(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)
	plaintext := []byte("same plaintext")
	c1, _ := enc.Encrypt(plaintext)
	c2, _ := enc.Encrypt(plaintext)
	if bytes.Equal(c1, c2) {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
	d1, _ := enc.Decrypt(c1)
	d2, _ := enc.Decrypt(c2)
	if !bytes.Equal(d1, d2) || !bytes.Equal(d1, plaintext) {
		t.Error("both should decrypt to plaintext")
	}
}

func TestAESGCMEncryptor_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)
	ciphertext, _ := enc.Encrypt([]byte("sensitive data"))
	ciphertext[len(ciphertext)-1] ^= 0xFF
	_, err := enc.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("decryption of tampered ciphertext should fail")
	}
}

func TestAESGCMEncryptor_RotateKey(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, _ := NewAESGCMEncryptor(key)
	plaintext := []byte("data encrypted before rotation")
	_, _ = enc.Encrypt(plaintext)
	oldKey := make([]byte, len(enc.key))
	copy(oldKey, enc.key)
	err := enc.RotateKey()
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if bytes.Equal(enc.key, oldKey) {
		t.Fatal("key should have been rotated")
	}
	newCiphertext, _ := enc.Encrypt(plaintext)
	newDecrypted, _ := enc.Decrypt(newCiphertext)
	if !bytes.Equal(newDecrypted, plaintext) {
		t.Error("encryption with rotated key should still work")
	}
}

func TestAESGCMEncryptor_EmptyPlaintext(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)
	ciphertext, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(decrypted) != 0 {
		t.Errorf("expected empty decrypted, got %d bytes", len(decrypted))
	}
}

func TestEncryptor_Interface(t *testing.T) {
	var _ Encryptor = (*AESGCMEncryptor)(nil)
}
