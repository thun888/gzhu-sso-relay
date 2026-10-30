package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

const keyBits = 2048

type Keys struct {
	private *rsa.PrivateKey
	kid     string
}

func LoadOrCreateKeys(path string) (*Keys, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		return parseKeys(raw)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("oidc: 读取签名密钥 %s: %w", path, err)
	}
	return generateKeys(path)
}

func parseKeys(raw []byte) (*Keys, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("oidc: 签名密钥不是 PEM")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return newKeys(rsaKey), nil
		}
		return nil, errors.New("oidc: 签名密钥不是 RSA")
	}

	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oidc: 解析签名密钥: %w", err)
	}
	return newKeys(rsaKey), nil
}

func generateKeys(path string) (*Keys, error) {
	private, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("oidc: 生成签名密钥: %w", err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, fmt.Errorf("oidc: 编码签名密钥: %w", err)
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("oidc: 创建密钥目录: %w", err)
		}
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, fmt.Errorf("oidc: 写入签名密钥 %s: %w", path, err)
	}

	return newKeys(private), nil
}

func newKeys(private *rsa.PrivateKey) *Keys {
	sum := sha256.Sum256(private.PublicKey.N.Bytes())
	return &Keys{
		private: private,
		kid:     base64.RawURLEncoding.EncodeToString(sum[:])[:16],
	}
}

func (k *Keys) JWK() map[string]any {
	pub := &k.private.PublicKey
	return map[string]any{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": k.kid,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}
