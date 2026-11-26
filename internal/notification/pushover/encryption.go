package pushover

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"unicode/utf8"
)

func parseEncryptionKey(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	// Never include the supplied key or hex decoding errors in diagnostics.
	key, err := hex.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("PUSHOVER_ENCRYPTION_KEY must contain exactly 64 hexadecimal characters")
	}
	return key, nil
}

// encryptField follows https://pushover.net/api#e2ee. Only the plaintext may
// be shortened to fit the final Base64 wire limit, never the ciphertext.
func encryptField(plaintext string, key []byte, limit int) (string, error) {
	compressed, err := compressLimitedField(plaintext, limit)
	if err != nil {
		return "", err
	}
	return encryptCompressedField(compressed, key)
}

// encryptExactField preserves URL destinations and fails rather than truncating.
func encryptExactField(plaintext string, key []byte, limit int) (string, error) {
	compressed, err := compressField(plaintext)
	if err != nil {
		return "", err
	}
	if encryptedFieldLength(len(compressed)) > limit {
		return "", fmt.Errorf("encrypted Pushover field cannot fit within %d characters without truncation", limit)
	}
	return encryptCompressedField(compressed, key)
}

func encryptCompressedField(compressed, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("Pushover encryption requires a 256-bit key")
	}
	padding := aes.BlockSize - len(compressed)%aes.BlockSize
	padded := append(compressed, bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialize Pushover encryption: %w", err)
	}

	payload := make([]byte, aes.BlockSize+len(padded), aes.BlockSize+len(padded)+sha256.Size)
	iv := payload[:aes.BlockSize]
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate Pushover encryption IV: %w", err)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(payload[aes.BlockSize:], padded)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	payload = append(payload, mac.Sum(nil)...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func compressField(plaintext string) ([]byte, error) {
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("initialize Pushover compression: %w", err)
	}
	if _, err := writer.Write([]byte(plaintext)); err != nil {
		return nil, fmt.Errorf("compress Pushover field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish Pushover compression: %w", err)
	}
	return buffer.Bytes(), nil
}

func encryptedFieldLength(compressedLength int) int {
	paddedLength := (compressedLength/aes.BlockSize + 1) * aes.BlockSize
	return base64.StdEncoding.EncodedLen(aes.BlockSize + paddedLength + sha256.Size)
}

func compressLimitedField(plaintext string, limit int) ([]byte, error) {
	compressed, err := compressField(plaintext)
	if err != nil {
		return nil, err
	}
	if encryptedFieldLength(len(compressed)) <= limit {
		return compressed, nil
	}

	// GZIP size is not strictly monotonic. Keep only candidates verified to fit
	// while finding a UTF-8 prefix in logarithmic compression attempts.
	var best []byte
	low, high := 1, utf8.RuneCountInString(plaintext)-1
	for low <= high {
		mid := low + (high-low)/2
		candidate, err := compressField(truncateField(plaintext, mid))
		if err != nil {
			return nil, err
		}
		if encryptedFieldLength(len(candidate)) <= limit {
			best = candidate
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	if best == nil {
		return nil, fmt.Errorf("encrypted Pushover field cannot fit within %d characters", limit)
	}
	return best, nil
}
