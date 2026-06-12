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

// acpSchemaVersion is the ACP spec release our feed/manifest shapes follow.
const acpSchemaVersion = "2026-04-17"

// --- Product management ---

func (api *API) CreateProduct(ctx *middleware.Context, merchantID string, body *core.CreateProductRequest) (*core.Product, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	if _, ok := ctx.Merchants[merchantID]; !ok {
		return nil, swagger_rest.BadRequest("unknown_merchant", "merchant '%s' is not configured", merchantID)
	}

	id, err := store.NewID()
	if err != nil {
		return nil, err
	}
	product := &store.Product{
		ID:          id,
		MerchantID:  merchantID,
		Title:       body.Title,
		Description: deref(body.Description),
		PriceMinor:  body.Price,
		Currency:    strings.ToLower(body.Currency),
		SKU:         deref(body.Sku),
		Category:    deref(body.Category),
		ImageURL:    deref(body.ImageUrl),
		Brand:       deref(body.Brand),
		GTIN:        deref(body.Gtin),
		MPN:         deref(body.Mpn),
		Quantity:    body.Quantity,
		Status:      store.ProductActive,

		Condition:        defaultCondition(deref(body.Condition)),
		SalePriceMinor:   derefInt(body.SalePrice),
		AdditionalImages: deref(body.AdditionalImages),
		Color:            deref(body.Color),
		Size:             deref(body.Size),
		Material:         deref(body.Material),
		WeightGrams:      derefInt(body.WeightGrams),
		GoogleCategory:   deref(body.GoogleCategory),
		VideoURL:         deref(body.VideoUrl),
	}
	if err := ctx.Store.CreateProduct(product); err != nil {
		return nil, err
	}
	return productToResponse(product, ctx.PublicBaseURL), nil
}

func (api *API) ListProducts(ctx *middleware.Context, merchantID string) (*core.ProductList, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	products, err := ctx.Store.ListActiveProducts(merchantID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(products))
	for i := range products {
		items = append(items, productMap(&products[i], ctx.PublicBaseURL))
	}
	list := core.ProductList{
		"merchant_id": merchantID,
		"count":       len(items),
		"products":    items,
	}
	return &list, nil
}

func (api *API) GetProduct(ctx *middleware.Context, productID string) (*core.Product, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	product, err := ctx.Store.GetProduct(productID)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, swagger_rest.NotFound("product '%s' not found", productID)
	}
	return productToResponse(product, ctx.PublicBaseURL), nil
}

func (api *API) UpdateProduct(ctx *middleware.Context, productID string, body *core.UpdateProductRequest) (*core.Product, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}

	fields := map[string]any{}
	if body.Title != nil {
		fields["title"] = *body.Title
	}
	if body.Description != nil {
		fields["description"] = *body.Description
	}
	if body.Price != nil {
		fields["price_minor"] = *body.Price
	}
	if body.Currency != nil {
		fields["currency"] = strings.ToLower(*body.Currency)
	}
	if body.Sku != nil {
		fields["sku"] = *body.Sku
	}
	if body.Category != nil {
		fields["category"] = *body.Category
	}
	if body.ImageUrl != nil {
		fields["image_url"] = *body.ImageUrl
	}
	if body.Brand != nil {
		fields["brand"] = *body.Brand
	}
	if body.Gtin != nil {
		fields["gtin"] = *body.Gtin
	}
	if body.Mpn != nil {
		fields["mpn"] = *body.Mpn
	}
	if body.Quantity != nil {
		fields["quantity"] = *body.Quantity
	}
	if body.Condition != nil {
		fields["condition"] = defaultCondition(*body.Condition)
	}
	if body.SalePrice != nil {
		fields["sale_price_minor"] = *body.SalePrice
	}
	if body.AdditionalImages != nil {
		fields["additional_images"] = *body.AdditionalImages
	}
	if body.Color != nil {
		fields["color"] = *body.Color
	}
	if body.Size != nil {
		fields["size"] = *body.Size
	}
	if body.Material != nil {
		fields["material"] = *body.Material
	}
	if body.WeightGrams != nil {
		fields["weight_grams"] = *body.WeightGrams
	}
	if body.GoogleCategory != nil {
		fields["google_category"] = *body.GoogleCategory
	}
	if body.VideoUrl != nil {
		fields["video_url"] = *body.VideoUrl
	}

	product, err := ctx.Store.UpdateProduct(productID, fields)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, swagger_rest.NotFound("product '%s' not found", productID)
	}
	return productToResponse(product, ctx.PublicBaseURL), nil
}

func (api *API) ArchiveProduct(ctx *middleware.Context, productID string) (*core.Product, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	ok, err := ctx.Store.ArchiveProduct(productID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, swagger_rest.NotFound("product '%s' not found", productID)
	}
	product, err := ctx.Store.GetProduct(productID)
	if err != nil || product == nil {
		return nil, fmt.Errorf("couldn't reload archived product: %w", err)
	}
	return productToResponse(product, ctx.PublicBaseURL), nil
}

// --- Agent manifests ---

func (api *API) AcpManifest(ctx *middleware.Context, merchantID string) (*core.AgentManifest, error) {
	merchant, ok := ctx.Merchants[merchantID]
	if !ok {
		return nil, swagger_rest.NotFound("merchant '%s' not found", merchantID)
	}
	base := ctx.PublicBaseURL
	manifest := core.AgentManifest{
		"schema_version": acpSchemaVersion,
		"protocol":       "acp",
		"merchant":       map[string]any{"id": merchant.ID, "name": merchant.Name},
		"feed_url":       fmt.Sprintf("%s/api/acp/%s/feed", base, merchant.ID),
		"endpoints": map[string]any{
			// ACP checkout_sessions adapter is the next phase; until then
			// agents use the native checkout, which returns a Trustap
			// pay_url for the buyer.
			"native_checkout": map[string]any{"method": "POST", "url": base + "/api/checkouts"},
		},
		"capabilities": map[string]any{
			"payment": map[string]any{
				"handlers": []any{
					map[string]any{"id": "trustap", "name": "Trustap buyer protection", "flow": "redirect"},
				},
			},
		},
	}
	return &manifest, nil
}

func (api *API) CopilotManifest(ctx *middleware.Context, merchantID string) (*core.AgentManifest, error) {
	merchant, ok := ctx.Merchants[merchantID]
	if !ok {
		return nil, swagger_rest.NotFound("merchant '%s' not found", merchantID)
	}
	base := ctx.PublicBaseURL
	manifest := core.AgentManifest{
		"schema_version":  acpSchemaVersion,
		"protocol":        "copilot",
		"compatible_with": "acp",
		"merchant":        map[string]any{"id": merchant.ID, "name": merchant.Name},
		"feed_url":        fmt.Sprintf("%s/api/copilot/%s/feed", base, merchant.ID),
		"endpoints": map[string]any{
			"native_checkout": map[string]any{"method": "POST", "url": base + "/api/checkouts"},
		},
	}
	return &manifest, nil
}

func (api *API) UcpManifest(ctx *middleware.Context, merchantID string) (*core.AgentManifest, error) {
	merchant, ok := ctx.Merchants[merchantID]
	if !ok {
		return nil, swagger_rest.NotFound("merchant '%s' not found", merchantID)
	}
	base := ctx.PublicBaseURL
	manifest := core.AgentManifest{
		"schema_version": "1.0",
		"protocol":       "ucp",
		"merchant":       map[string]any{"id": merchant.ID, "name": merchant.Name},
		"capabilities": map[string]any{
			"catalog": map[string]any{
				"product_feed_url": fmt.Sprintf("%s/api/acp/%s/feed", base, merchant.ID),
				"gmc_feed_url":     fmt.Sprintf("%s/feeds/%s/gmc.csv", base, merchant.ID),
				"product_pages":    fmt.Sprintf("%s/shop/%s", base, merchant.ID),
			},
		},
		"payment_handlers": []any{
			map[string]any{"id": "trustap", "name": "Trustap buyer protection", "flow": "redirect"},
		},
	}
	return &manifest, nil
}

// --- Agent product feeds (ACP shape; Copilot reuses it) ---

func (api *API) AcpFeed(ctx *middleware.Context, merchantID string) (*core.ProductFeed, error) {
	return api.productFeed(ctx, merchantID)
}

func (api *API) CopilotFeed(ctx *middleware.Context, merchantID string) (*core.ProductFeed, error) {
	return api.productFeed(ctx, merchantID)
}

func (api *API) productFeed(ctx *middleware.Context, merchantID string) (*core.ProductFeed, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	merchant, ok := ctx.Merchants[merchantID]
	if !ok {
		return nil, swagger_rest.NotFound("merchant '%s' not found", merchantID)
	}
	products, err := ctx.Store.ListActiveProducts(merchantID)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(products))
	for i := range products {
		p := &products[i]
		availability := "in_stock"
		if p.Quantity <= 0 {
			availability = "out_of_stock"
		}
		item := map[string]any{
			"id":                 p.ID,
			"title":              p.Title,
			"description":        p.Description,
			"link":               productPageURL(p, ctx.PublicBaseURL),
			"image_link":         p.ImageURL,
			"price":              feedPrice(p),
			"availability":       availability,
			"available_quantity": p.Quantity,
			"sku":                p.SKU,
			"category":           p.Category,
			"brand":              p.Brand,
			"gtin":               p.GTIN,
			"mpn":                p.MPN,
			"condition":          defaultCondition(p.Condition),
		}
		if p.SalePriceMinor > 0 {
			item["sale_price"] = fmt.Sprintf("%.2f %s", float64(p.SalePriceMinor)/100, strings.ToUpper(p.Currency))
		}
		if imgs := splitImages(p.AdditionalImages); len(imgs) > 0 {
			item["additional_image_links"] = imgs
		}
		if p.Color != "" {
			item["color"] = p.Color
		}
		if p.Size != "" {
			item["size"] = p.Size
		}
		if p.Material != "" {
			item["material"] = p.Material
		}
		if p.WeightGrams > 0 {
			item["shipping_weight"] = fmt.Sprintf("%d g", p.WeightGrams)
		}
		if p.GoogleCategory != "" {
			item["google_product_category"] = p.GoogleCategory
		}
		if p.VideoURL != "" {
			item["video_link"] = p.VideoURL
		}
		items = append(items, item)
	}

	feed := core.ProductFeed{
		"version":  acpSchemaVersion,
		"merchant": map[string]any{"id": merchant.ID, "name": merchant.Name},
		"count":    len(items),
		"products": items,
	}
	return &feed, nil
}

// --- Helpers ---

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func productPageURL(p *store.Product, base string) string {
	return fmt.Sprintf("%s/shop/%s/products/%s", base, p.MerchantID, p.ID)
}

// feedPrice renders the GMC-style price string, e.g. "9.99 EUR".
func feedPrice(p *store.Product) string {
	return fmt.Sprintf("%.2f %s", float64(p.PriceMinor)/100, strings.ToUpper(p.Currency))
}

func productToResponse(p *store.Product, base string) *core.Product {
	createdAt := p.CreatedAt.UTC().Format(time.RFC3339)
	pageURL := productPageURL(p, base)
	return &core.Product{
		ID:          p.ID,
		MerchantID:  p.MerchantID,
		Title:       p.Title,
		Description: optional(p.Description),
		Price:       p.PriceMinor,
		Currency:    p.Currency,
		Sku:         optional(p.SKU),
		Category:    optional(p.Category),
		ImageUrl:    optional(p.ImageURL),
		Brand:       optional(p.Brand),
		Gtin:        optional(p.GTIN),
		Mpn:         optional(p.MPN),
		Quantity:    p.Quantity,
		Status:      p.Status,
		PageUrl:     &pageURL,
		CreatedAt:   &createdAt,

		Condition:        optional(p.Condition),
		SalePrice:        optionalInt(p.SalePriceMinor),
		AdditionalImages: optional(p.AdditionalImages),
		Color:            optional(p.Color),
		Size:             optional(p.Size),
		Material:         optional(p.Material),
		WeightGrams:      optionalInt(p.WeightGrams),
		GoogleCategory:   optional(p.GoogleCategory),
		VideoUrl:         optional(p.VideoURL),
	}
}

func defaultCondition(c string) string {
	switch c {
	case "refurbished", "used":
		return c
	default:
		return "new"
	}
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func optionalInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func splitImages(csv string) []string {
	var out []string
	for _, part := range strings.Split(csv, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func productMap(p *store.Product, base string) map[string]any {
	return map[string]any{
		"id":          p.ID,
		"merchant_id": p.MerchantID,
		"title":       p.Title,
		"description": p.Description,
		"price":       p.PriceMinor,
		"currency":    p.Currency,
		"sku":         p.SKU,
		"category":    p.Category,
		"image_url":   p.ImageURL,
		"brand":       p.Brand,
		"gtin":        p.GTIN,
		"mpn":         p.MPN,
		"quantity":    p.Quantity,
		"status":      p.Status,
		"page_url":    productPageURL(p, base),
	}
}
