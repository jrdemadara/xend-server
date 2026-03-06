package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"xend.chat/m/internal/config"
	"xend.chat/m/internal/db"
	"xend.chat/m/internal/jobs"
	"xend.chat/m/internal/logging"
	"xend.chat/m/internal/notify"
	"xend.chat/m/internal/queue"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}
	logger := logging.New()

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	mailer := notify.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFromEmail, cfg.AppBaseURL)
	rotator := jobs.NewIdentifierRotationJob(pool, cfg.IdentifierRotationEvery)

	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB}
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 10})
	mux := asynq.NewServeMux()

	mux.HandleFunc(queue.TaskTypeSendVerificationEmail, func(ctx context.Context, task *asynq.Task) error {
		var payload queue.SendVerificationEmailPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		if err := mailer.SendVerificationEmail(ctx, payload.Email, payload.Token); err != nil {
			logger.Error("send verification email failed", "error", err, "email", payload.Email)
			return err
		}
		logger.Info("verification email sent", "email", payload.Email)
		return nil
	})

	mux.HandleFunc(queue.TaskTypeSendRelationshipInviteEmail, func(ctx context.Context, task *asynq.Task) error {
		var payload queue.SendRelationshipInviteEmailPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		if err := mailer.SendRelationshipInviteEmail(ctx, payload.Email, payload.InviterDisplayName, payload.InviterIdentifier, payload.Note); err != nil {
			logger.Error("send relationship invite email failed", "error", err, "email", payload.Email)
			return err
		}
		logger.Info("relationship invite email sent", "email", payload.Email)
		return nil
	})

	mux.HandleFunc(queue.TaskTypeRotateIdentifiers, func(ctx context.Context, _ *asynq.Task) error {
		count, err := rotator.RotateDueUsers(ctx)
		if err != nil {
			logger.Error("identifier rotation failed", "error", err)
			return err
		}
		if count > 0 {
			logger.Info("identifier rotation completed", "rotated_users", count)
		}
		return nil
	})

	// Periodically enqueue identifier rotation scan job.
	go func() {
		ticker := time.NewTicker(cfg.IdentifierRotationTick)
		defer ticker.Stop()

		enqueue := func() {
			_, err := client.Enqueue(asynq.NewTask(queue.TaskTypeRotateIdentifiers, nil), asynq.MaxRetry(3))
			if err != nil {
				logger.Error("enqueue identifier rotation job failed", "error", err)
			}
		}

		enqueue()
		for range ticker.C {
			enqueue()
		}
	}()

	go func() {
		logger.Info("worker started")
		if err := srv.Run(mux); err != nil {
			logger.Error("worker crashed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	srv.Shutdown()
	logger.Info("worker stopped")
}
