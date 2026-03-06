package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

const verificationAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func NewVerificationToken() (raw string, hash string, err error) {
	token, err := randomCode(8)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func randomCode(length int) (string, error) {
	if length <= 0 {
		length = 8
	}
	result := make([]byte, length)
	max := big.NewInt(int64(len(verificationAlphabet)))
	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = verificationAlphabet[n.Int64()]
	}
	return string(result), nil
}
