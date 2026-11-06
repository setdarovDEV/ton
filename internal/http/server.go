// Package http exposes the marketplace over REST + WebSocket. Handlers only
// translate between HTTP and domain services; no business rules live here.
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"tonnet/internal/auth"
	"tonnet/internal/chain"
	"tonnet/internal/domain"
)

// Deps are the collaborators the server needs.
type Deps struct {
	Catalog  *domain.CatalogService
	Listings *domain.ListingService
	Txs      *domain.TxService
	Payloads *auth.PayloadIssuer
	Proofs   *auth.Verifier
	Tokens   *auth.TokenIssuer
	Hub      *Hub
	// Addrs normalises inbound addresses and renders outbound ones.
	Addrs chain.AddressCodec
	// Network is the chain network name ("testnet" | "mainnet") the API serves;
	// ton_proof requests for another network are rejected.
	Network string
	Origins []string
	Log     *slog.Logger
	Health  func() error
}

// Server is the HTTP handler tree.
type Server struct {
	deps   Deps
	render addrFormatter
	mux    *chi.Mux
}

// NewServer builds the router.
func NewServer(d Deps) *Server {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := &Server{deps: d, render: addrFormatter{codec: d.Addrs}}
	s.mux = s.routes()
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(s.deps.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.deps.Origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", s.health)
	if s.deps.Hub != nil {
		r.Get("/ws", s.deps.Hub.ServeHTTP)
	}

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/payload", s.authPayload)
		r.Post("/auth/verify", s.authVerify)

		r.Get("/collections", s.listCollections)
		r.Get("/collections/{addr}", s.getCollection)
		r.Get("/collections/{addr}/items", s.listCollectionItems)
		r.Get("/items/{addr}", s.getItem)
		r.Get("/users/{addr}/items", s.listUserItems)
		r.Get("/listings", s.listListings)
		r.Get("/listings/{id}", s.getListing)
		r.Get("/transactions/{hash}", s.getTx)

		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(s.deps.Tokens, writeError))
			r.Get("/auth/me", s.authMe)
			r.Post("/listings/intent", s.listingIntent)
			r.Post("/listings/{id}/buy-intent", s.buyIntent)
			r.Post("/listings/{id}/cancel-intent", s.cancelIntent)
			r.Post("/transactions", s.registerTx)
		})
	})
	return r
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "bytes", ww.BytesWritten(),
				"took", time.Since(start).Round(time.Microsecond), "req_id", middleware.GetReqID(r.Context()))
		})
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.deps.Health != nil {
		if err := s.deps.Health(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "ws_clients": s.wsClients()})
}

func (s *Server) wsClients() int {
	if s.deps.Hub == nil {
		return 0
	}
	return s.deps.Hub.ClientCount()
}
