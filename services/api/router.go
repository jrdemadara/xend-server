package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/challenges"
	"xend.chat/m/internal/dailycheckin"
	"xend.chat/m/internal/dailyritual"
	"xend.chat/m/internal/device"
	"xend.chat/m/internal/message"
	"xend.chat/m/internal/presence"
	"xend.chat/m/internal/relationship"
	"xend.chat/m/internal/user"
	"xend.chat/m/pkg/httputil"
	servicerealtime "xend.chat/m/services/realtime"
)

func NewRouter(authHandler *auth.Handler, protectedAuthHandler *ProtectedAuthHandler, userHandler *user.Handler, deviceHandler *device.Handler, relationshipHandler *relationship.Handler, dailyCheckInHandler *dailycheckin.Handler, dailyRitualHandler *dailyritual.Handler, challengeHandler *challenges.Handler, messageHandler *message.Handler, presenceHandler *presence.Handler, realtimeHandler *servicerealtime.Handler, tokens *auth.TokenManager, db *pgxpool.Pool, redisClient *goredis.Client) http.Handler {
	r := chi.NewRouter()
	rl := newAuthRateLimiter(redisClient, 30, time.Minute)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		checks := map[string]string{
			"database": "ok",
			"redis":    "ok",
		}
		if err := db.Ping(ctx); err != nil {
			checks["database"] = "unavailable"
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unavailable"
		}
		if checks["database"] != "ok" || checks["redis"] != "ok" {
			checks["status"] = "not_ready"
			httputil.JSON(w, http.StatusServiceUnavailable, checks)
			return
		}
		checks["status"] = "ready"
		httputil.JSON(w, http.StatusOK, checks)
	})
	r.Get("/v1/ws", realtimeHandler.ServeWS)

	r.Route("/v1", func(v1 chi.Router) {
		v1.Route("/auth", func(ar chi.Router) {
			ar.Use(rl.middlewareHandler)
			ar.Post("/register", authHandler.Register)
			ar.Post("/verify-email", authHandler.VerifyEmail)
			ar.Post("/resend-verification", authHandler.ResendVerification)
			ar.Post("/login", authHandler.Login)
			ar.Post("/google", authHandler.GoogleSignIn)
			ar.Post("/refresh", authHandler.Refresh)
			ar.With(requireAuthMiddleware(tokens)).Post("/logout", protectedAuthHandler.Logout)
			ar.With(requireAuthMiddleware(tokens)).Post("/logout-all", protectedAuthHandler.LogoutAll)
		})

		v1.Group(func(protected chi.Router) {
			protected.Use(requireAuthMiddleware(tokens))
			protected.Get("/ws/connections", realtimeHandler.Connections)

			protected.Route("/daily-rituals", func(dr chi.Router) {
				dr.Get("/templates", dailyRitualHandler.ListTemplates)
			})

			protected.Route("/users", func(ur chi.Router) {
				ur.Get("/me", userHandler.Me)
				ur.Get("/{user_id}/prekeys", deviceHandler.GetPrekeys)
				ur.Get("/{user_id}/presence", presenceHandler.GetUserPresence)
			})

			protected.Route("/devices", func(dr chi.Router) {
				dr.Post("/register", deviceHandler.RegisterDevice)
				dr.Put("/{device_id}/signed-prekey", deviceHandler.UpsertSignedPrekey)
				dr.Put("/{device_id}/kyber-prekey", deviceHandler.UpsertKyberPrekey)
				dr.Post("/{device_id}/one-time-prekeys/batch", deviceHandler.UploadOneTimePrekeys)
				dr.Post("/{device_id}/push-token", deviceHandler.UpsertPushToken)
			})

			protected.Route("/relationship-invites", func(rr chi.Router) {
				rr.Post("/", relationshipHandler.CreateInvite)
				rr.Get("/inbox", relationshipHandler.Inbox)
				rr.Get("/outbox", relationshipHandler.Outbox)
				rr.Post("/{invite_id}/accept", relationshipHandler.Accept)
				rr.Post("/{invite_id}/decline", relationshipHandler.Decline)
			})

			protected.Route("/relationship-spaces", func(sr chi.Router) {
				sr.Get("/", relationshipHandler.ListSpaces)
				sr.Get("/levels", relationshipHandler.ListLevels)
				sr.Post("/unlock", relationshipHandler.UnlockSpace)
				sr.Get("/{space_id}/daily-checkin", dailyCheckInHandler.GetToday)
				sr.Post("/{space_id}/daily-checkin", dailyCheckInHandler.Submit)
				sr.Get("/{space_id}/daily-rituals", dailyRitualHandler.GetOverview)
				sr.Post("/{space_id}/daily-rituals/{assignment_id}/submit", dailyRitualHandler.Submit)
				sr.Patch("/{space_id}/settings", relationshipHandler.UpdateSpaceSettings)
				sr.Post("/{space_id}/cover-photo", relationshipHandler.UploadCoverPhoto)
				sr.Post("/{space_id}/couple-photo", relationshipHandler.UploadCouplePhoto)
				sr.Get("/{space_id}/media/{kind}", relationshipHandler.GetSpaceMedia)
				sr.Get("/{space_id}/moods", relationshipHandler.ListCurrentMoods)
				sr.Post("/{space_id}/moods", relationshipHandler.SetMood)
				sr.Get("/{space_id}/challenges/templates", challengeHandler.ListTemplates)
				sr.Get("/{space_id}/challenges", challengeHandler.GetOverview)
				sr.Post("/{space_id}/challenges", challengeHandler.Create)
				sr.Post("/{space_id}/challenges/{challenge_id}/accept", challengeHandler.Accept)
				sr.Post("/{space_id}/challenges/{challenge_id}/decline", challengeHandler.Decline)
				sr.Post("/{space_id}/challenges/{challenge_id}/complete", challengeHandler.Complete)
				sr.Get("/{space_id}/challenges/{challenge_id}/submission-image", challengeHandler.GetSubmissionImage)
				sr.Get("/{space_id}/level-progress", relationshipHandler.ListLevelProgress)
				sr.Get("/{space_id}/members", relationshipHandler.ListMembers)
				sr.Put("/{space_id}/default", relationshipHandler.SetDefaultSpace)
				sr.Put("/{space_id}/access-lock", relationshipHandler.ConfigureSpaceAccess)
			})

			protected.Route("/conversations", func(cr chi.Router) {
				cr.Get("/{conversation_id}/messages", messageHandler.ListByConversation)
				cr.Post("/{conversation_id}/messages", messageHandler.Send)
				cr.Post("/{conversation_id}/read", messageHandler.MarkConversationRead)
			})

			protected.Route("/messages", func(mr chi.Router) {
				mr.Get("/sync", messageHandler.Sync)
			})

			protected.Route("/presence", func(pr chi.Router) {
				pr.Post("/heartbeat", presenceHandler.Heartbeat)
				pr.Post("/offline", presenceHandler.Offline)
			})
		})
	})

	return r
}
