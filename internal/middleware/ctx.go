package middleware

import (
	"fmt"

	rest_api_log "github.com/trustap/rest_api/pkg/log"
	"github.com/trustap/rest_api/pkg/middleware/http_log"
	"github.com/trustap/rest_api/pkg/rest"
	"github.com/trustap/trustap_index/internal/store"
	"github.com/trustap/trustap_index/internal/trustap"
)

// Merchant is a merchant configured on the Index, with its own Trustap API
// client credentials (per-merchant client model).
type Merchant struct {
	ID      string
	Name    string
	Trustap trustap.Credentials
}

// Context contains any application-specific dependencies that handlers need
type Context struct {
	Store         *store.Store
	Trustap       *trustap.Client
	Merchants     map[string]Merchant
	PublicBaseURL string
}

// MiddlewareContext is used by the middleware chain and holds both
// the application context and middleware-specific data
type MiddlewareContext struct {
	*Context

	endptMeta        *Meta
	middlewareLogger *rest_api_log.FieldLogger
	pathParams       rest.PathParams
}

func (c *MiddlewareContext) SetEndptMetadata(meta *Meta) error {
	c.endptMeta = meta
	return nil
}

func (c *MiddlewareContext) EndptMetadata() (*Meta, error) {
	return c.endptMeta, nil
}

func (c *MiddlewareContext) SetPathParams(pathParams rest.PathParams) error {
	if c.pathParams != nil {
		return fmt.Errorf("path parameters are already set")
	}
	c.pathParams = pathParams
	return nil
}

func (c *MiddlewareContext) PathParams() (rest.PathParams, error) {
	return c.pathParams, nil
}

func (c *MiddlewareContext) SetMiddlewareLogger(logger *rest_api_log.FieldLogger) error {
	if c.middlewareLogger != nil {
		return fmt.Errorf("middleware logger is already set")
	}
	c.middlewareLogger = logger
	return nil
}

func (c *MiddlewareContext) MiddlewareLogger() (*rest_api_log.FieldLogger, error) {
	if c.middlewareLogger == nil {
		return nil, fmt.Errorf("middleware logger is not set")
	}
	return c.middlewareLogger, nil
}

func (c *MiddlewareContext) BodyLogging() (http_log.BodyLogging, error) {
	return http_log.BodyLoggingBoth, nil
}
