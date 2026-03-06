package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	issuer        string
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

type AccessClaims struct {
	UserID   string
	DeviceID string
}

func NewTokenManager(issuer, accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{issuer: issuer, accessSecret: []byte(accessSecret), refreshSecret: []byte(refreshSecret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (tm *TokenManager) CreateAccessToken(userID, deviceID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(tm.accessTTL)
	claims := jwt.MapClaims{"sub": userID, "device_id": deviceID, "scope": "api", "type": "access", "iss": tm.issuer, "iat": time.Now().Unix(), "exp": expiresAt.Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(tm.accessSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (tm *TokenManager) ParseAccessToken(raw string) (AccessClaims, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return tm.accessSecret, nil
	})
	if err != nil || !token.Valid {
		return AccessClaims{}, fmt.Errorf("invalid access token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return AccessClaims{}, fmt.Errorf("invalid access claims")
	}
	if claims["type"] != "access" || claims["iss"] != tm.issuer {
		return AccessClaims{}, fmt.Errorf("invalid access token payload")
	}

	u, _ := claims["sub"].(string)
	d, _ := claims["device_id"].(string)
	if u == "" || d == "" {
		return AccessClaims{}, fmt.Errorf("invalid access token payload")
	}
	return AccessClaims{UserID: u, DeviceID: d}, nil
}

func (tm *TokenManager) CreateRefreshToken(userID, deviceID, sessionID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(tm.refreshTTL)
	claims := jwt.MapClaims{"sub": userID, "device_id": deviceID, "session_id": sessionID, "type": "refresh", "iss": tm.issuer, "iat": time.Now().Unix(), "exp": expiresAt.Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(tm.refreshSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (tm *TokenManager) ParseRefreshToken(raw string) (userID, deviceID, sessionID string, err error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return tm.refreshSecret, nil
	})
	if err != nil || !token.Valid {
		return "", "", "", fmt.Errorf("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", "", fmt.Errorf("invalid refresh claims")
	}
	if claims["type"] != "refresh" || claims["iss"] != tm.issuer {
		return "", "", "", fmt.Errorf("invalid refresh token payload")
	}

	u, _ := claims["sub"].(string)
	d, _ := claims["device_id"].(string)
	s, _ := claims["session_id"].(string)
	if u == "" || d == "" || s == "" {
		return "", "", "", fmt.Errorf("invalid refresh token payload")
	}
	return u, d, s, nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
