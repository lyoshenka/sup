package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"github.com/topscore/sup/Godeps/_workspace/src/golang.org/x/crypto/pbkdf2"
	"io"
)

var iterations = 4096
var aes256KeySize = 32
var saltLength = 32 // salt length for key derivation fn

type container struct {
	Data []byte
	IV   []byte
	Salt []byte
	Hmac []byte
}

func pkcs5pad(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func pkcs5unpad(src []byte) []byte {
	length := len(src)
	unpadding := int(src[length-1])
	return src[:(length - unpadding)]
}

func deriveKey(key, salt []byte) []byte {
	return pbkdf2.Key(key, salt, iterations, aes256KeySize, sha256.New)
}

func calcMAC(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func checkMAC(message, messageMAC, key []byte) bool {
	expectedMAC := calcMAC(message, key)
	return hmac.Equal(messageMAC, expectedMAC)
}

func decodeData(data []byte) (*container, error) {
	buffer := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buffer)
	var c container
	err := dec.Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func encodeContainer(c *container) ([]byte, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	if err := enc.Encode(*c); err != nil {
		return nil, err
	}
	return []byte(buffer.String()), nil
}

func encrypt(key, plaintext []byte) ([]byte, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("encrypt: %s", err)
	}

	derivedKey := deriveKey(key, salt)

	plaintext = pkcs5pad(plaintext, aes.BlockSize)
	if len(plaintext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("encrypt: plaintext is not a multiple of the block size")
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %s", err)
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("encrypt: %s", err)
	}

	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)

	c := container{ciphertext, iv, salt, calcMAC(append(ciphertext, append(iv, salt...)...), key)}
	data, err := encodeContainer(&c)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %s", err)
	}

	return data, nil
}

func decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < saltLength+aes.BlockSize {
		return nil, fmt.Errorf("decrypt: ciphertext too short")
	}

	data, err := decodeData(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %s", err)
	}

	if !checkMAC(append(data.Data, append(data.IV, data.Salt...)...), data.Hmac, key) {
		return nil, fmt.Errorf("decrypt: invalid key")
	}

	derivedKey := deriveKey(key, data.Salt)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %s", err)
	}

	if len(data.Data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("decrypt: ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, data.IV)

	plaintext := make([]byte, len(data.Data))
	mode.CryptBlocks(plaintext, data.Data)
	plaintext = pkcs5unpad(plaintext)

	return plaintext, nil
}
