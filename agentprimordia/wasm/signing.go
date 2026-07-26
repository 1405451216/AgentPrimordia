package wasm

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
)

// VerifySignature 验证 WASM 字节码的 Ed25519 签名
//
// 签名是对 WASM 字节码的 SHA-256 哈希的 Ed25519 签名。
// 验证流程：
// 1. 计算 WASM 字节码的 SHA-256 哈希
// 2. 使用公钥验证签名
func VerifySignature(wasmBytes, signature, publicKey []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d (expected %d)", len(publicKey), ed25519.PublicKeySize)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: %d (expected %d)", len(signature), ed25519.SignatureSize)
	}

	// 计算哈希
	hash := sha256.Sum256(wasmBytes)

	// 验证签名
	if !ed25519.Verify(ed25519.PublicKey(publicKey), hash[:], signature) {
		return fmt.Errorf("signature verification failed: invalid signature")
	}

	return nil
}

// SignWASM 使用 Ed25519 私钥对 WASM 字节码签名
//
// 签名是对 WASM 字节码的 SHA-256 哈希的 Ed25519 签名。
// 返回签名和对应的公钥。
func SignWASM(wasmBytes []byte, privateKey ed25519.PrivateKey) ([]byte, []byte, error) {
	hash := sha256.Sum256(wasmBytes)
	signature := ed25519.Sign(privateKey, hash[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return signature, []byte(publicKey), nil
}

// GenerateKeyPair 生成 Ed25519 密钥对
func GenerateKeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	return priv, pub, err
}

// KeyFingerprint 计算公钥的指纹（SHA-256 前 8 字节的十六进制）
func KeyFingerprint(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return fmt.Sprintf("%x", hash[:4])
}
