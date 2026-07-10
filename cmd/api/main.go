package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/config"
	"xend.chat/m/internal/dailycheckin"
	"xend.chat/m/internal/dailyritual"
	"xend.chat/m/internal/db"
	"xend.chat/m/internal/logging"
	"xend.chat/m/internal/notify"
	"xend.chat/m/internal/presence"
	"xend.chat/m/internal/queue"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/internal/redis"
	"xend.chat/m/services/api"
	servicerealtime "xend.chat/m/services/realtime"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}
	logger := logging.New()
	slog.SetDefault(logger)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	redisClient, err := redis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Error("redis connect failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer asynqClient.Close()

	repo := auth.NewRepository(pool)
	tm := auth.NewTokenManager(cfg.JWTIssuer, cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	googleVerifier := &auth.IDTokenGoogleVerifier{Audience: cfg.GoogleOAuthClientID}
	emailVerifyStore := auth.NewRedisEmailVerificationStore(redisClient)
	loginAttempts := auth.NewRedisLoginAttemptStore(redisClient)
	emailEnqueuer := queue.NewVerificationEmailEnqueuer(asynqClient)
	svc := auth.NewService(repo, tm, googleVerifier, emailVerifyStore, loginAttempts, emailEnqueuer)
	h := auth.NewHandler(svc)
	protectedAuthHandler := api.NewProtectedAuthHandler(svc)
	deviceHandler := api.NewDeviceHandler(repo)
	presenceSvc := presence.NewService(redisClient)
	presenceHandler := api.NewPresenceHandler(presenceSvc, repo)
	hub := realtime.NewHub()
	realtimeHandler := servicerealtime.NewHandler(tm, hub, presenceSvc, repo)
	var pushNotifier notify.PushNotifier = notify.NoopPushNotifier{}
	if cfg.FirebaseCredentialsFile != "" {
		fcmNotifier, fcmErr := notify.NewFCMNotifier(cfg.FirebaseCredentialsFile)
		if fcmErr != nil {
			logger.Error("fcm notifier disabled", "error", fcmErr)
		} else {
			pushNotifier = fcmNotifier
			logger.Info("fcm notifier enabled", "credentials_file", cfg.FirebaseCredentialsFile)
		}
	} else {
		logger.Info("fcm notifier disabled", "reason", "FIREBASE_CREDENTIALS_FILE is empty")
	}
	relationshipHandler := api.NewRelationshipHandler(repo, emailEnqueuer, hub, pushNotifier)
	dailyCheckInRepo := dailycheckin.NewRepository(pool)
	dailyCheckInHandler := api.NewDailyCheckInHandler(dailyCheckInRepo, hub)
	dailyRitualRepo := dailyritual.NewRepository(pool)
	dailyRitualStore := dailyritual.NewSubmissionStore("storage/daily-rituals")
	dailyRitualHandler := api.NewDailyRitualHandler(dailyRitualRepo, dailyRitualStore)
	messageHandler := api.NewMessageHandler(repo, hub, pushNotifier)
	router := api.NewRouter(h, protectedAuthHandler, deviceHandler, relationshipHandler, dailyCheckInHandler, dailyRitualHandler, messageHandler, presenceHandler, realtimeHandler, tm, redisClient)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("api server started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server crashed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("api server stopped")
}
