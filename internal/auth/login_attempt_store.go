package auth

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type LoginAttemptStore interface {
	IsLocked(ctx context.Context, email, clientIP string) (bool, error)
	RegisterFailure(ctx context.Context, email, clientIP string) error
	Clear(ctx context.Context, email, clientIP string) error
}

type RedisLoginAttemptStore struct {
	client *goredis.Client
}

func NewRedisLoginAttemptStore(client *goredis.Client) *RedisLoginAttemptStore {
	return &RedisLoginAttemptStore{client: client}
}

func (s *RedisLoginAttemptStore) IsLocked(ctx context.Context, email, clientIP string) (bool, error) {
	emailLocked, err := s.client.Exists(ctx, loginEmailLockKey(email)).Result()
	if err != nil {
		return false, err
	}
	if emailLocked > 0 {
		return true, nil
	}
	if clientIP == "" {
		return false, nil
	}
	ipLocked, err := s.client.Exists(ctx, loginIPLockKey(clientIP)).Result()
	if err != nil {
		return false, err
	}
	return ipLocked > 0, nil
}

func (s *RedisLoginAttemptStore) RegisterFailure(ctx context.Context, email, clientIP string) error {
	if err := s.bumpAndMaybeLock(ctx, loginEmailFailKey(email), loginEmailLockKey(email), 5, 15*time.Minute); err != nil {
		return err
	}
	if clientIP == "" {
		return nil
	}
	return s.bumpAndMaybeLock(ctx, loginIPFailKey(clientIP), loginIPLockKey(clientIP), 20, 15*time.Minute)
}

func (s *RedisLoginAttemptStore) Clear(ctx context.Context, email, clientIP string) error {
	keys := []string{loginEmailFailKey(email), loginEmailLockKey(email)}
	if clientIP != "" {
		keys = append(keys, loginIPFailKey(clientIP), loginIPLockKey(clientIP))
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *RedisLoginAttemptStore) bumpAndMaybeLock(ctx context.Context, failKey, lockKey string, threshold int64, lockTTL time.Duration) error {
	count, err := s.client.Incr(ctx, failKey).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		if err := s.client.Expire(ctx, failKey, 15*time.Minute).Err(); err != nil {
			return err
		}
	}
	if count >= threshold {
		if err := s.client.Set(ctx, lockKey, "1", lockTTL).Err(); err != nil {
			return err
		}
	}
	return nil
}

func loginEmailFailKey(email string) string { return fmt.Sprintf("auth:login:fail:email:%s", email) }
func loginEmailLockKey(email string) string { return fmt.Sprintf("auth:login:lock:email:%s", email) }
func loginIPFailKey(ip string) string       { return fmt.Sprintf("auth:login:fail:ip:%s", ip) }
func loginIPLockKey(ip string) string       { return fmt.Sprintf("auth:login:lock:ip:%s", ip) }
