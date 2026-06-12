package svc

import (
	"fmt"
	"strings"
	"time"

	"github.com/trustap/trustap_index/internal/middleware"
	"github.com/trustap/trustap_index/internal/store"
	"github.com/trustap/trustap_index/target/gen/swagger_server/core"
	"github.com/trustap/trustap_index/tools/gen_swagger_server/swagger_rest"
)

// DashboardConfig is public: the SPA bootstraps from it (whether SSO is
// configured and against which Keycloak realm/client).
func (api *API) DashboardConfig(ctx *middleware.Context) (*core.DashboardConfig, error) {
	configured := ctx.KeycloakAuthority != "" && ctx.KeycloakClientID != ""
	cfg := core.DashboardConfig{
		"keycloak": map[string]any{
			"configured": configured,
			"authority":  ctx.KeycloakAuthority,
			"client_id":  ctx.KeycloakClientID,
		},
		"public_base_url": ctx.PublicBaseURL,
	}
	return &cfg, nil
}

func (api *API) DashboardMerchants(ctx *middleware.Context) (*core.MerchantList, error) {
	merchants := make([]any, 0, len(ctx.Merchants))
	for _, m := range ctx.Merchants {
		merchants = append(merchants, map[string]any{
			"id":                m.ID,
			"name":              m.Name,
			"trustap_connected": m.Trustap.APIKey != "" && m.Trustap.Sub != "",
		})
	}
	list := core.MerchantList{"merchants": merchants}
	return &list, nil
}

// agentSurfaces maps each AI agent to the Index surfaces it consumes.
var agentSurfaces = []struct {
	Key      string
	Name     string
	Surfaces []string
}{
	{"chatgpt", "ChatGPT (OpenAI)", []string{"acp_manifest", "acp_feed"}},
	{"copilot", "Microsoft Copilot", []string{"copilot_manifest", "copilot_feed"}},
	{"google", "Google (Gemini / AI Mode)", []string{"ucp_manifest", "gmc_feed"}},
	{"perplexity", "Perplexity", []string{"gmc_feed", "shop_page", "product_page"}},
}

func (api *API) DashboardOverview(ctx *middleware.Context, merchantID string) (*core.MerchantOverview, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	merchant, ok := ctx.Merchants[merchantID]
	if !ok {
		return nil, swagger_rest.NotFound("merchant '%s' not found", merchantID)
	}

	statuses, err := ctx.Store.SurfaceStatuses(merchantID)
	if err != nil {
		return nil, err
	}
	products, err := ctx.Store.ListAllProducts(merchantID)
	if err != nil {
		return nil, err
	}
	checkouts, err := ctx.Store.ListRecentCheckouts(merchantID, 10)
	if err != nil {
		return nil, err
	}
	lastWebhook, err := ctx.Store.LastWebhookEventTime(merchantID)
	if err != nil {
		return nil, err
	}
	paidCount, revenueMinor, err := ctx.Store.PaymentsSummary(merchantID)
	if err != nil {
		return nil, err
	}
	revenueTrend, err := ctx.Store.DailyPaidRevenue(merchantID, 7)
	if err != nil {
		revenueTrend = make([]int, 7)
	}

	overview := core.MerchantOverview{
		"merchant":         map[string]any{"id": merchant.ID, "name": merchant.Name},
		"agents":           api.agentStatuses(ctx, merchantID, statuses),
		"catalog_health":   catalogHealth(products),
		"connection":       connectionStatus(merchant, lastWebhook, paidCount, revenueMinor, revenueTrend),
		"recent_checkouts": recentCheckouts(checkouts),
	}
	return &overview, nil
}

func (api *API) agentStatuses(ctx *middleware.Context, merchantID string, statuses map[string]*store.SurfaceStatus) []any {
	base := ctx.PublicBaseURL
	surfaceURLs := map[string]string{
		"acp_manifest":     fmt.Sprintf("%s/api/acp/%s/.well-known/agentic-commerce", base, merchantID),
		"acp_feed":         fmt.Sprintf("%s/api/acp/%s/feed", base, merchantID),
		"copilot_manifest": fmt.Sprintf("%s/api/copilot/%s/.well-known/copilot-checkout", base, merchantID),
		"copilot_feed":     fmt.Sprintf("%s/api/copilot/%s/feed", base, merchantID),
		"ucp_manifest":     fmt.Sprintf("%s/api/ucp/%s/.well-known/ucp", base, merchantID),
		"gmc_feed":         fmt.Sprintf("%s/feeds/%s/gmc.csv", base, merchantID),
		"shop_page":        fmt.Sprintf("%s/shop/%s", base, merchantID),
		"product_page":     fmt.Sprintf("%s/shop/%s/products/...", base, merchantID),
	}

	agents := make([]any, 0, len(agentSurfaces))
	for _, agent := range agentSurfaces {
		activity, err := ctx.Store.ActivityBuckets(merchantID, agent.Surfaces, 24, 1)
		if err != nil {
			activity = make([]int, 24)
		}
		var lastFetch *time.Time
		hits24h := 0
		surfaces := make([]any, 0, len(agent.Surfaces))
		for _, key := range agent.Surfaces {
			entry := map[string]any{"surface": key, "url": surfaceURLs[key]}
			if st, ok := statuses[key]; ok {
				entry["last_fetch"] = st.LastFetch.UTC().Format(time.RFC3339)
				entry["hits_24h"] = st.Hits24h
				hits24h += st.Hits24h
				if lastFetch == nil || st.LastFetch.After(*lastFetch) {
					lastFetch = st.LastFetch
				}
			} else {
				entry["last_fetch"] = nil
				entry["hits_24h"] = 0
			}
			surfaces = append(surfaces, entry)
		}

		status := "waiting" // surfaces live, nothing has fetched them yet
		if hits24h > 0 {
			status = "active"
		} else if lastFetch != nil {
			status = "quiet" // fetched in the past but not in the last 24h
		}
		entry := map[string]any{
			"key":      agent.Key,
			"name":     agent.Name,
			"status":   status,
			"hits_24h": hits24h,
			"surfaces": surfaces,
			"activity": activity,
		}
		if lastFetch != nil {
			entry["last_fetch"] = lastFetch.UTC().Format(time.RFC3339)
		}
		agents = append(agents, entry)
	}
	return agents
}

// catalogHealth reports per-product issues and per-agent readiness, so
// merchants see exactly which products need extra info for which agent.
func catalogHealth(products []store.Product) map[string]any {
	items := make([]any, 0, len(products))
	readyAll := 0
	withIssues := 0

	for i := range products {
		p := &products[i]

		var issues []string
		if p.ImageURL == "" {
			issues = append(issues, "missing_image")
		}
		if strings.TrimSpace(p.Description) == "" {
			issues = append(issues, "missing_description")
		} else if len(p.Description) < 20 {
			issues = append(issues, "short_description")
		}
		if p.GTIN == "" {
			issues = append(issues, "missing_gtin")
		}
		if p.Brand == "" {
			issues = append(issues, "missing_brand")
		}
		if p.Category == "" {
			issues = append(issues, "missing_category")
		}
		if p.Quantity <= 0 {
			issues = append(issues, "out_of_stock")
		}

		hasDescription := strings.TrimSpace(p.Description) != ""
		hasImage := p.ImageURL != ""
		hasIdentifier := p.GTIN != "" || (p.Brand != "" && p.MPN != "")
		readiness := map[string]any{
			"chatgpt":    hasDescription && hasImage,
			"copilot":    hasDescription && hasImage,
			"google":     hasImage && hasIdentifier,
			"perplexity": hasImage && hasDescription && p.GTIN != "",
		}
		allReady := true
		for _, v := range readiness {
			if v != true {
				allReady = false
			}
		}
		if allReady && p.Quantity > 0 {
			readyAll++
		}
		if len(issues) > 0 {
			withIssues++
		}

		if issues == nil {
			issues = []string{}
		}
		issueList := make([]any, 0, len(issues))
		for _, iss := range issues {
			issueList = append(issueList, iss)
		}
		items = append(items, map[string]any{
			"id":        p.ID,
			"title":     p.Title,
			"price":     p.PriceMinor,
			"currency":  p.Currency,
			"quantity":  p.Quantity,
			"status":    p.Status,
			"issues":    issueList,
			"readiness": readiness,
		})
	}

	return map[string]any{
		"products": items,
		"summary": map[string]any{
			"total":       len(products),
			"ready_all":   readyAll,
			"with_issues": withIssues,
		},
	}
}

func connectionStatus(merchant middleware.Merchant, lastWebhook time.Time, paidCount, revenueMinor int, revenueTrend []int) map[string]any {
	webhook := map[string]any{"last_event": nil}
	if !lastWebhook.IsZero() {
		webhook["last_event"] = lastWebhook.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"trustap": map[string]any{
			"connected": merchant.Trustap.APIKey != "" && merchant.Trustap.Sub != "",
		},
		"webhooks": webhook,
		// Honest placeholder: the store connector (Shopify first) is a
		// later phase; until then inventory sync is one-way (Index only).
		"store_sync": map[string]any{
			"status":   "not_connected",
			"provider": nil,
			"note":     "Store connector coming soon; inventory currently updates on Index sales only.",
		},
		"payments": map[string]any{
			"paid_count":    paidCount,
			"revenue_minor": revenueMinor,
			"trend":         revenueTrend,
		},
	}
}

func recentCheckouts(checkouts []store.Checkout) []any {
	out := make([]any, 0, len(checkouts))
	for i := range checkouts {
		c := &checkouts[i]
		out = append(out, map[string]any{
			"id":             c.ID,
			"description":    c.Description,
			"price":          c.PriceMinor,
			"currency":       c.Currency,
			"status":         c.Status,
			"transaction_id": c.TransactionID,
			"buyer_returned": c.BuyerReturned,
			"created_at":     c.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}
