package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const versionPrefix = "v1:"

var ErrKeySize = errors.New("cryptox: 密钥必须是 32 字节，用 base64 或 hex 编码")

type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// ParseKey 接受 base64 或 hex 编码的 32 字节密钥。
func ParseKey(encoded string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if raw, err := hex.DecodeString(encoded); err == nil && len(raw) == 32 {
		return raw, nil
	}
	return nil, ErrKeySize
}

func (c *Cipher) Seal(plaintext []byte) string {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic("cryptox: 生成 nonce 失败: " + err.Error())
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return versionPrefix + base64.StdEncoding.EncodeToString(sealed)
}

func (c *Cipher) Open(encoded string) ([]byte, error) {
	if !strings.HasPrefix(encoded, versionPrefix) {
		return nil, fmt.Errorf("cryptox: 不支持的密文版本 %q", encoded)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, versionPrefix))
	if err != nil {
		return nil, fmt.Errorf("cryptox: 密文不是合法 base64: %w", err)
	}
	size := c.aead.NonceSize()
	if len(raw) < size {
		return nil, errors.New("cryptox: 密文长度不足")
	}
	plaintext, err := c.aead.Open(nil, raw[:size], raw[size:], nil)
	if err != nil {
		return nil, fmt.Errorf("cryptox: 解密失败: %w", err)
	}
	return plaintext, nil
}
