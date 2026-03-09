package presence

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Service struct {
	redis        *goredis.Client
	deviceTTL    time.Duration
	lastSeenKeep time.Duration
}

func NewService(redis *goredis.Client) *Service {
	return &Service{
		redis:        redis,
		deviceTTL:    90 * time.Second,
		lastSeenKeep: 30 * 24 * time.Hour,
	}
}

func (s *Service) MarkOnline(ctx context.Context, userID, deviceID string) error {
	key := s.deviceKey(userID, deviceID)
	if err := s.redis.Set(ctx, key, "1", s.deviceTTL).Err(); err != nil {
		return err
	}
	return s.redis.Set(ctx, s.lastSeenKey(userID), time.Now().Unix(), s.lastSeenKeep).Err()
}

func (s *Service) Heartbeat(ctx context.Context, userID, deviceID string) error {
	return s.redis.Expire(ctx, s.deviceKey(userID, deviceID), s.deviceTTL).Err()
}

func (s *Service) MarkOffline(ctx context.Context, userID, deviceID string) error {
	if err := s.redis.Del(ctx, s.deviceKey(userID, deviceID)).Err(); err != nil {
		return err
	}
	return s.redis.Set(ctx, s.lastSeenKey(userID), time.Now().Unix(), s.lastSeenKeep).Err()
}

func (s *Service) IsUserOnline(ctx context.Context, userID string) (bool, error) {
	keys, err := s.redis.Keys(ctx, fmt.Sprintf("presence:user:%s:device:*", userID)).Result()
	if err != nil {
		return false, err
	}
	return len(keys) > 0, nil
}

func (s *Service) LastSeen(ctx context.Context, userID string) (*time.Time, error) {
	value, err := s.redis.Get(ctx, s.lastSeenKey(userID)).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	epoch, err := strconv.ParseInt(value, 10, 64)
	if err != nil || epoch <= 0 {
		return nil, nil
	}
	t := time.Unix(epoch, 0).UTC()
	return &t, nil
}

func (s *Service) deviceKey(userID, deviceID string) string {
	return fmt.Sprintf("presence:user:%s:device:%s", userID, deviceID)
}

func (s *Service) lastSeenKey(userID string) string {
	return fmt.Sprintf("presence:user:%s:last_seen", userID)
}
