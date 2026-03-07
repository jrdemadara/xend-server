package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"
	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
	servicerealtime "xend.chat/m/services/realtime"
)

func NewRouter(authHandler *auth.Handler, protectedAuthHandler *ProtectedAuthHandler, deviceHandler *DeviceHandler, relationshipHandler *RelationshipHandler, presenceHandler *PresenceHandler, realtimeHandler *servicerealtime.Handler, tokens *auth.TokenManager, redisClient *goredis.Client) http.Handler {
	r := chi.NewRouter()
	rl := newAuthRateLimiter(redisClient, 30, time.Minute)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

			protected.Route("/users", func(ur chi.Router) {
				ur.Get("/me", func(w http.ResponseWriter, r *http.Request) {
					claims, ok := claimsFromContext(r.Context())
					if !ok {
						httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
						return
					}
					profile, err := deviceHandler.repo.GetUserProfileByID(r.Context(), claims.UserID)
					if err != nil {
						httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
						return
					}
					httputil.JSON(w, http.StatusOK, auth.UserProfileResponse{
						UserID:      profile.ID,
						DeviceID:    claims.DeviceID,
						DisplayName: profile.DisplayName,
						Email:       profile.Email,
						AvatarURL:   profile.AvatarURL,
						Identifier:  profile.Identifier,
					})
				})
				ur.Get("/{user_id}/prekeys", deviceHandler.GetPrekeys)
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
				sr.Get("/{space_id}/level-progress", relationshipHandler.ListLevelProgress)
				sr.Get("/{space_id}/members", relationshipHandler.ListMembers)
			})

			protected.Route("/presence", func(pr chi.Router) {
				pr.Post("/heartbeat", presenceHandler.Heartbeat)
				pr.Post("/offline", presenceHandler.Offline)
			})
		})
	})

	return r
}
