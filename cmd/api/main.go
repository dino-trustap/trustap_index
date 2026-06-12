package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc"

	"github.com/julienschmidt/httprouter"
	rest_api_log "github.com/trustap/rest_api/pkg/log"
	rest_api_middleware "github.com/trustap/rest_api/pkg/middleware"
	"github.com/trustap/rest_api/pkg/middleware/http_log"
	"github.com/trustap/rest_api/pkg/rest"
	"github.com/trustap/rest_api/pkg/rest/responses"
	"github.com/trustap/rest_api/pkg/rest/responses/resputil"
	"github.com/trustap/trustap_index/frontend"
	"github.com/trustap/trustap_index/internal/middleware"
	"github.com/trustap/trustap_index/internal/store"
	"github.com/trustap/trustap_index/internal/surfaces"
	"github.com/trustap/trustap_index/internal/svc"
	"github.com/trustap/trustap_index/internal/trustap"
	trustap_index_http "github.com/trustap/trustap_index/pkg/http"
	"github.com/trustap/trustap_index/target/gen/swagger_server/core"
	"github.com/trustap/trustap_index/tools/gen_swagger_server/swagger_rest"
)

func main() {
	loggers := rest_api_log.NewJSONLoggerFactory(os.Stdout)
	setupLogger, err := loggers.New("setup")
	if err != nil {
		fmt.Printf("couldn't create setup logger: %v", err)
		os.Exit(1)
	}
	_ = setupLogger.Logf(rest_api_log.LevelInfo, "starting")

	argv := os.Args
	if len(argv) < 2 || len(argv) > 3 {
		msg := "usage: %s [config-yaml] <listen-addr>"
		_ = setupLogger.Logf(rest_api_log.LevelError, msg, argv[0])
		os.Exit(1)
	}

	var configPath string
	var listenAddr string

	if len(argv) == 3 {
		configPath = argv[1]
		listenAddr = argv[2]
	} else {
		listenAddr = argv[1]
	}

	err = run(loggers, setupLogger, configPath, listenAddr)
	if err != nil {
		_ = setupLogger.Logf(rest_api_log.LevelError, "%v", err)
		os.Exit(1)
	}
}

func run(
	loggers *rest_api_log.JSONLoggerFactory,
	setupLogger rest_api_log.UtilStructuredLogger,
	configPath string,
	listenAddr string,
) error {
	// Load config if provided
	var cfg *config
	if configPath != "" {
		_ = setupLogger.Logf(rest_api_log.LevelInfo, "reading config from '%s'", configPath)
		var err error
		cfg, err = readConfig(configPath)
		if err != nil {
			return fmt.Errorf("couldn't read configuration at '%s': %w", configPath, err)
		}
		_ = setupLogger.Logf(rest_api_log.LevelInfo, "configuration loaded successfully (log level: %s)", cfg.Logging.Level)
	}

	globalCtx := &middleware.Context{}

	var publicSurfaces *surfaces.Handler
	webhookUser, webhookPassword := "", ""
	if cfg != nil {
		if cfg.Database.DSN != "" {
			db, err := store.Open(cfg.Database.DSN)
			if err != nil {
				return fmt.Errorf("couldn't open store: %w", err)
			}
			defer func() { _ = db.Close() }()
			globalCtx.Store = db
			_ = setupLogger.Logf(rest_api_log.LevelInfo, "database connected and migrated")
		}

		env := trustap.EnvTest
		if cfg.Trustap.Environment == string(trustap.EnvLive) {
			env = trustap.EnvLive
		}
		globalCtx.Trustap = trustap.NewClient(env)
		globalCtx.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
		webhookUser = cfg.Trustap.WebhookUser
		webhookPassword = cfg.Trustap.WebhookPassword

		globalCtx.Merchants = map[string]middleware.Merchant{}
		for _, m := range cfg.Merchants {
			globalCtx.Merchants[m.ID] = middleware.Merchant{
				ID:   m.ID,
				Name: m.Name,
				Trustap: trustap.Credentials{
					APIKey: m.TrustapAPIKey,
					Sub:    m.TrustapSub,
				},
			}
		}
		_ = setupLogger.Logf(rest_api_log.LevelInfo, "configured %d merchant(s), trustap env '%s'", len(cfg.Merchants), env)

		merchantNames := map[string]string{}
		for _, m := range cfg.Merchants {
			merchantNames[m.ID] = m.Name
		}
		publicSurfaces = &surfaces.Handler{
			Store:         globalCtx.Store,
			MerchantNames: merchantNames,
			PublicBaseURL: globalCtx.PublicBaseURL,
		}
	}

	// Dashboard SSO: when Keycloak is configured, /api/dashboard/* (except
	// the public bootstrap config) requires a valid bearer ID token. When
	// not configured the dashboard runs open (dev mode).
	var dashboardVerifier *oidc.IDTokenVerifier
	if cfg != nil && cfg.Dashboard.KeycloakAuthority != "" && cfg.Dashboard.KeycloakClientID != "" {
		provider, err := oidc.NewProvider(context.Background(), cfg.Dashboard.KeycloakAuthority)
		if err != nil {
			return fmt.Errorf("couldn't reach keycloak issuer '%s': %w", cfg.Dashboard.KeycloakAuthority, err)
		}
		dashboardVerifier = provider.Verifier(&oidc.Config{ClientID: cfg.Dashboard.KeycloakClientID})
		globalCtx.KeycloakAuthority = cfg.Dashboard.KeycloakAuthority
		globalCtx.KeycloakClientID = cfg.Dashboard.KeycloakClientID
		_ = setupLogger.Logf(rest_api_log.LevelInfo, "dashboard SSO enabled against '%s'", cfg.Dashboard.KeycloakAuthority)
	} else {
		_ = setupLogger.Logf(rest_api_log.LevelInfo, "dashboard SSO not configured; dashboard runs in dev mode")
	}

	dashboardDist, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		return fmt.Errorf("couldn't open embedded dashboard: %w", err)
	}
	dashboardFiles := http.FileServer(http.FS(dashboardDist))

	// Set up endpoints
	coreEndpts := core.Endpoints(svc.NewAPI(), &contextRefiner{})
	endpts := coreEndpts

	routerLogger, err := loggers.New("router")
	if err != nil {
		return fmt.Errorf("couldn't create router logger: %w", err)
	}
	router := rest_api_middleware.NewRouter[*middleware.Meta, *middleware.MiddlewareContext](
		httprouter.New(),
		newParams,
		routerLogger,
	)

	for _, ep := range endpts {
		handle := rest_api_middleware.HandlerFunc[*middleware.MiddlewareContext](
			func(c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				middlewareLogger, err := c.MiddlewareLogger()
				if err != nil {
					return fmt.Errorf("couldn't get middleware logger: %w", err)
				}

				epInfo := map[string]string{
					"method": ep.Method,
					"path":   ep.Path,
				}
				middlewareLogger.Log("endpoint", epInfo)

				err = ep.Handle(c, w, r)
				if err != nil {
					return fmt.Errorf("couldn't handle: %w", err)
				}

				return nil
			},
		)
		router.Handle(ep.Method, pathSwaggerToHTTPRouter("/api"+ep.Path), ep.Meta, handle)
	}

	httpLogger, err := loggers.New("http")
	if err != nil {
		return fmt.Errorf("couldn't create HTTP logger: %w", err)
	}

	middlewareChain := rest_api_middleware.Join(
		rest_api_middleware.NewMaxBytesStep[*middleware.MiddlewareContext](16*rest_api_middleware.Kb, map[string]int64{}),
		rest_api_middleware.NewSimpleStep(
			"webhook basic auth",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				if r.URL.Path == "/api/webhooks/trustap" {
					user, password, ok := r.BasicAuth()
					authorised := ok &&
						webhookUser != "" &&
						subtle.ConstantTimeCompare([]byte(user), []byte(webhookUser)) == 1 &&
						subtle.ConstantTimeCompare([]byte(password), []byte(webhookPassword)) == 1
					if !authorised {
						w.Header().Set("WWW-Authenticate", `Basic realm="trustap-webhooks"`)
						w.WriteHeader(http.StatusUnauthorized)
						return nil
					}
				}
				return next.ServeHTTP(c, w, r)
			},
		),
		rest_api_middleware.NewSimpleStep(
			"dashboard static",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				if r.URL.Path == "/dashboard" {
					http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
					return nil
				}
				if strings.HasPrefix(r.URL.Path, "/dashboard/") {
					name := strings.TrimPrefix(r.URL.Path, "/dashboard/")
					if name == "" {
						name = "index.html"
					}
					if _, err := fs.Stat(dashboardDist, name); err != nil {
						name = "index.html" // SPA fallback for client-side routes
					}
					// FileServer 301-redirects direct index.html requests, so
					// serve the SPA shell by hand.
					if name == "index.html" {
						shell, err := fs.ReadFile(dashboardDist, "index.html")
						if err != nil {
							w.WriteHeader(http.StatusInternalServerError)
							return nil
						}
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						_, _ = w.Write(shell)
						return nil
					}
					req := new(http.Request)
					*req = *r
					req.URL = new(url.URL)
					*req.URL = *r.URL
					req.URL.Path = "/" + name
					dashboardFiles.ServeHTTP(w, req)
					return nil
				}
				return next.ServeHTTP(c, w, r)
			},
		),
		rest_api_middleware.NewSimpleStep(
			"dashboard auth",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				if dashboardVerifier != nil &&
					strings.HasPrefix(r.URL.Path, "/api/dashboard/") &&
					r.URL.Path != "/api/dashboard/config" {
					authz := r.Header.Get("Authorization")
					raw := strings.TrimPrefix(authz, "Bearer ")
					if raw == "" || raw == authz {
						w.WriteHeader(http.StatusUnauthorized)
						return nil
					}
					if _, err := dashboardVerifier.Verify(r.Context(), raw); err != nil {
						w.WriteHeader(http.StatusUnauthorized)
						return nil
					}
				}
				return next.ServeHTTP(c, w, r)
			},
		),
		rest_api_middleware.NewSimpleStep(
			"surface analytics",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				if globalCtx.Store != nil && r.Method == http.MethodGet {
					if merchantID, surface, ok := surfaceFromPath(r.URL.Path); ok {
						userAgent := r.Header.Get("User-Agent")
						go func() {
							_ = globalCtx.Store.RecordSurfaceHit(merchantID, surface, userAgent)
						}()
					}
				}
				return next.ServeHTTP(c, w, r)
			},
		),
		rest_api_middleware.NewSimpleStep(
			"public surfaces",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				if publicSurfaces.Handle(w, r) {
					return nil
				}
				return next.ServeHTTP(c, w, r)
			},
		),
		router.Route,
		rest_api_middleware.NewSimpleStep(
			"log request",
			http_log.NewHTTPLogger[*middleware.MiddlewareContext](httpLogger).Handle,
		),
		rest_api_middleware.NewSimpleStep(
			"add loggers",
			func(next rest_api_middleware.Handler[*middleware.MiddlewareContext], c *middleware.MiddlewareContext, w http.ResponseWriter, r *http.Request) error {
				middlewareLogger, err := c.MiddlewareLogger()
				if err != nil {
					return fmt.Errorf("couldn't get middleware logger: %w", err)
				}

				lcf := rest_api_log.NewLogCollectorFactory()
				defer func() {
					logs := lcf.Logs()
					middlewareLogger.Log("errors", errorsFromLogEvents(logs))
					middlewareLogger.Log("events", logs)
				}()

				err = next.ServeHTTP(c, w, r)
				if err != nil {
					errLogger := lcf.New("error")
					_ = errLogger.Logf(rest_api_log.LevelError, "error: %v", err)

					// Per the swagger_rest contract, a HandleEndpointError
					// means the endpoint already wrote its response (e.g. a
					// client error); writing again would corrupt the body.
					heErr := &swagger_rest.HandleEndpointError{}
					if !errors.As(err, &heErr) {
						resps := &resputil.CommonResponses{
							Responses: responses.NewStandardJSON(w, lcf.New("http")),
						}
						_ = resps.InternalServerError("internal server error")
					}
				}

				return nil
			},
		),
		rest_api_middleware.NewCatchPanicsStep[*middleware.MiddlewareContext](),
		rest_api_middleware.NewLogRequestIDStep[*middleware.MiddlewareContext](),
		rest_api_middleware.CallHandler[*middleware.MiddlewareContext](),
	)

	serverLogger, err := loggers.New("server")
	if err != nil {
		return fmt.Errorf("couldn't create router logger: %w", err)
	}
	server := &http.Server{
		Addr: listenAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := &middleware.MiddlewareContext{
				Context: globalCtx,
			}
			err := middlewareChain.ServeHTTP(c, w, r)
			if err != nil {
				_ = serverLogger.Logf(rest_api_log.LevelError, "couldn't serve: %v", err)
			}
		}),
	}

	_ = setupLogger.Logf(rest_api_log.LevelInfo, "listening on '%s'", listenAddr)
	err = trustap_index_http.ListenAndServe(server, 3*time.Second)
	if err != nil {
		return fmt.Errorf("listening failed: %w", err)
	}
	_ = setupLogger.Logf(rest_api_log.LevelInfo, "shutdown finished gracefully")

	return nil
}

type contextRefiner struct{}

func (*contextRefiner) RefineToCoreContext(c *middleware.MiddlewareContext) (*middleware.Context, error) {
	return c.Context, nil
}

func newParams(ps httprouter.Params) rest.PathParams {
	return &params{Params: ps}
}

type params struct {
	httprouter.Params
}

func (p *params) Get(name string) (string, bool) {
	v := p.ByName(name)
	if v == "" {
		return "", false
	}
	return v, true
}

func pathSwaggerToHTTPRouter(path string) string {
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	return path
}

func errorsFromLogEvents(logEvents []map[string]any) []any {
	errorLogs := []any{}
	for _, log := range logEvents {
		level, ok := log["level"]
		if !ok || level != rest_api_log.LevelError {
			continue
		}

		data, ok := log["data"]
		if !ok {
			continue
		}

		errorLogs = append(errorLogs, data)
	}
	return errorLogs
}

// surfaceFromPath maps a request path onto (merchant, surface) for agent
// surface analytics; ok is false for paths that aren't public surfaces.
func surfaceFromPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "acp" && parts[3] == "feed":
		return parts[2], "acp_feed", true
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "acp" && parts[3] == ".well-known":
		return parts[2], "acp_manifest", true
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "copilot" && parts[3] == "feed":
		return parts[2], "copilot_feed", true
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "copilot" && parts[3] == ".well-known":
		return parts[2], "copilot_manifest", true
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "ucp" && parts[3] == ".well-known":
		return parts[2], "ucp_manifest", true
	case len(parts) == 3 && parts[0] == "feeds" && parts[2] == "gmc.csv":
		return parts[1], "gmc_feed", true
	case len(parts) == 2 && parts[0] == "shop":
		return parts[1], "shop_page", true
	case len(parts) == 4 && parts[0] == "shop" && parts[2] == "products":
		return parts[1], "product_page", true
	}
	return "", "", false
}
