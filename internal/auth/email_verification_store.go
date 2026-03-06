package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type EmailVerificationStore interface {
	Save(ctx context.Context, email, tokenHash string, ttl time.Duration) error
	Consume(ctx context.Context, email, tokenHash string) (bool, error)
	AllowSend(ctx context.Context, email, clientIP string) (bool, error)
}

type RedisEmailVerificationStore struct {
	client *goredis.Client
}

func NewRedisEmailVerificationStore(client *goredis.Client) *RedisEmailVerificationStore {
	return &RedisEmailVerificationStore{client: client}
}

func (s *RedisEmailVerificationStore) Save(ctx context.Context, email, tokenHash string, ttl time.Duration) error {
	key := verificationKey(email)
	return s.client.Set(ctx, key, tokenHash, ttl).Err()
}

func (s *RedisEmailVerificationStore) Consume(ctx context.Context, email, tokenHash string) (bool, error) {
	key := verificationKey(email)
	val, err := s.client.GetDel(ctx, key).Result()
	if err == goredis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if subtle.ConstantTimeCompare([]byte(val), []byte(tokenHash)) != 1 {
		return false, nil
	}
	return true, nil
}

func (s *RedisEmailVerificationStore) AllowSend(ctx context.Context, email, clientIP string) (bool, error) {
	cooldownKey := fmt.Sprintf("auth:email_verify:cooldown:%s", email)
	ok, err := s.client.SetNX(ctx, cooldownKey, "1", 60*time.Second).Result()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	emailHourlyKey := fmt.Sprintf("auth:email_verify:hour:%s", email)
	emailCount, err := s.client.Incr(ctx, emailHourlyKey).Result()
	if err != nil {
		return false, err
	}
	if emailCount == 1 {
		if err := s.client.Expire(ctx, emailHourlyKey, time.Hour).Err(); err != nil {
			return false, err
		}
	}
	if emailCount > 5 {
		return false, nil
	}

	if clientIP == "" {
		return true, nil
	}

	ipHourlyKey := fmt.Sprintf("auth:email_verify:ip:%s:hour", clientIP)
	ipCount, err := s.client.Incr(ctx, ipHourlyKey).Result()
	if err != nil {
		return false, err
	}
	if ipCount == 1 {
		if err := s.client.Expire(ctx, ipHourlyKey, time.Hour).Err(); err != nil {
			return false, err
		}
	}
	if ipCount > 20 {
		return false, nil
	}

	return true, nil
}

func verificationKey(email string) string {
	return fmt.Sprintf("auth:email_verify:%s", email)
}
