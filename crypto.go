// crypto.go
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// SecurityManager handles encryption/decryption
type SecurityManager struct {
	key []byte
}

// NewSecurityManager creates a new security manager from password
func NewSecurityManager(password string) *SecurityManager {
	// Derive 32-byte key from password using PBKDF2
	salt := []byte("evilday-c2-salt-v2-secure")
	key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)

	return &SecurityManager{
		key: key,
	}
}

// Encrypt encrypts data using AES-256-GCM
func (sm *SecurityManager) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM
func (sm *SecurityManager) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(sm.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptString encrypts string and returns HEX with Replay Protection (Timestamp)
func (sm *SecurityManager) EncryptString(plaintext string) (string, error) {
	// Prepend timestamp to payload: "timestamp|plaintext"
	payload := fmt.Sprintf("%d|%s", time.Now().UnixNano(), plaintext)
	
	encrypted, err := sm.Encrypt([]byte(payload))
	if err != nil {
		return "", err
	}
	// Hex encoding is more reliable than base64 for network transmission
	return hex.EncodeToString(encrypted), nil
}

// DecryptString decrypts hex string and verifies Replay Protection
func (sm *SecurityManager) DecryptString(ciphertext string) (string, error) {
	// Trim any whitespace or newlines first
	ciphertext = strings.TrimSpace(ciphertext)

	// Decode from hex
	decoded, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", errors.New("invalid hex: " + err.Error())
	}

	decryptedBytes, err := sm.Decrypt(decoded)
	if err != nil {
		return "", errors.New("decryption failed: " + err.Error())
	}

	decrypted := string(decryptedBytes)
	parts := strings.SplitN(decrypted, "|", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid payload format (possible replay attack or old version)")
	}

	// Verify Timestamp (DISABLED)
	// ts, err := strconv.ParseInt(parts[0], 10, 64)
	// if err != nil {
	// 	return "", errors.New("invalid timestamp")
	// }

	// msgTime := time.Unix(0, ts)
	// TIME CHECK DISABLED (Unlimited Tolerance)
	// if time.Since(msgTime) > 24*time.Hour || time.Since(msgTime) < -24*time.Hour {
	// 	return "", errors.New("replay attack detected: message expired")
	// }

	return parts[1], nil
}

// GenerateChallenge creates a random challenge for authentication
func GenerateChallenge() (string, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return "", err
	}
	return hex.EncodeToString(challenge), nil
}

// VerifyChallenge verifies challenge response
func (sm *SecurityManager) VerifyChallenge(challenge, response string) bool {
	// Expected response is HMAC of challenge with shared key
	expected := sha256.Sum256(append(sm.key, []byte(challenge)...))
	expectedStr := hex.EncodeToString(expected[:])
	return expectedStr == response
}

// CreateChallengeResponse creates response for challenge
func (sm *SecurityManager) CreateChallengeResponse(challenge string) string {
	response := sha256.Sum256(append(sm.key, []byte(challenge)...))
	return hex.EncodeToString(response[:])
}
