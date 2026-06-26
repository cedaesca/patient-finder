package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/cedaesca/patient-finder/internal/config"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := chi.NewRouter()

	// ==========================================================
	// 1. GLOBAL MIDDLEWARES
	// ==========================================================
	r.Use(middleware.RequestID)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(request.LoggingMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(traceRouteName)
	r.Use(request.ExtractRequestInfo)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   resolveCORSAllowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   utils.RateLimitHeaderNames,
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Use(request.Authenticate(os.Getenv(config.JWTAccessTokenSecretKey)))
	r.Use(request.ResolveLocale)

	// ==========================================================
	// 2. PUBLIC ROUTES
	// ==========================================================
	r.Group(func(public chi.Router) {
		s.registerPublicRoutes(public)
	})

	// ==========================================================
	// 3. PRIVATE ROUTES
	// ==========================================================
	r.Group(func(private chi.Router) {
		private.Use(request.RequireAuthenticated)

		s.registerPrivateRoutes(private)
	})

	return r
}

func resolveCORSAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv(config.CORSAllowedOriginsKey))
	if raw == "" {
		return []string{"https://*", "http://*"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if o := strings.TrimSpace(p); o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		return []string{"https://*", "http://*"}
	}
	return origins
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResp, _ := json.Marshal(s.db.Health())
	_, _ = w.Write(jsonResp)
}

func (s *Server) registerPublicRoutes(r chi.Router) {
	r.Get("/", s.HelloWorldHandler)
	r.Get("/health", s.healthHandler)

	s.app.Handlers.Auth.RegisterRoutes(r)
}

func (s *Server) registerPrivateRoutes(r chi.Router) {
	s.app.Handlers.Users.RegisterRoutes(r)
	s.app.Handlers.Audit.RegisterRoutes(r)
}
