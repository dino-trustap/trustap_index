package svc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/trustap/trustap_index/internal/middleware"
	"github.com/trustap/trustap_index/internal/store"
	"github.com/trustap/trustap_index/internal/trustap"
	"github.com/trustap/trustap_index/target/gen/swagger_server/core"
	"github.com/trustap/trustap_index/tools/gen_swagger_server/swagger_rest"
)

func NewAPI() *API {
	return &API{}
}

type API struct{}

// GetHeartbeat is a simple health check endpoint that returns nil (204 No Content)
func (api *API) GetHeartbeat(_ *middleware.Context) error {
	return nil
}

// CreateCheckout runs the full-seller/guest-buyer payment flow: guest user ->
// charge -> transaction on the merchant's behalf -> actions page pay URL.
func (api *API) CreateCheckout(ctx *middleware.Context, body *core.CreateCheckoutRequest) (*core.Checkout, error) {
	if ctx.Store == nil || ctx.Trustap == nil {
		return nil, fmt.Errorf("service is not fully configured (database/trustap missing)")
	}

	merchant, ok := ctx.Merchants[body.MerchantID]
	if !ok {
		return nil, swagger_rest.BadRequest(
			"unknown_merchant", "merchant '%s' is not configured", body.MerchantID,
		)
	}
	if merchant.Trustap.APIKey == "" || merchant.Trustap.Sub == "" {
		return nil, swagger_rest.BadRequest(
			"merchant_not_onboarded",
			"merchant '%s' has no Trustap credentials configured", body.MerchantID,
		)
	}

	checkoutID, err := store.NewCheckoutID()
	if err != nil {
		return nil, err
	}

	// Either a catalog product (price/description derived, stock checked) or
	// raw description+price+currency for catalog-less checkouts.
	quantity := 1
	if body.Quantity != nil && *body.Quantity > 0 {
		quantity = *body.Quantity
	}
	productID, description, price, currency := "", "", 0, ""
	if body.ProductID != nil && *body.ProductID != "" {
		product, err := ctx.Store.GetProduct(*body.ProductID)
		if err != nil {
			return nil, err
		}
		if product == nil || product.Status != store.ProductActive || product.MerchantID != body.MerchantID {
			return nil, swagger_rest.BadRequest(
				"unknown_product", "product '%s' is not available from merchant '%s'", *body.ProductID, body.MerchantID,
			)
		}
		if product.Quantity < quantity {
			return nil, swagger_rest.BadRequestWithContext(
				"insufficient_stock",
				map[string]any{"available": product.Quantity, "requested": quantity},
				"product '%s' has insufficient stock", product.ID,
			)
		}
		productID = product.ID
		description = fmt.Sprintf("%s x%d", product.Title, quantity)
		price = product.PriceMinor * quantity
		currency = strings.ToLower(product.Currency)
	} else {
		if body.Description == nil || body.Price == nil || body.Currency == nil {
			return nil, swagger_rest.BadRequest(
				"missing_pricing", "either product_id or description+price+currency is required",
			)
		}
		description = *body.Description
		price = *body.Price
		currency = strings.ToLower(*body.Currency)
	}

	reqCtx := context.Background()

	// Trustap requires a ToS-acceptance IP; it feeds dispute/chargeback
	// evidence, so callers (agents) SHOULD pass the buyer's real IP. The
	// placeholder keeps agent flows working when they can't.
	// TODO capture the requester IP from the connection as a better default.
	buyerIP := "0.0.0.0"
	if body.BuyerIp != nil && *body.BuyerIp != "" {
		buyerIP = *body.BuyerIp
	}
	guest, err := ctx.Trustap.CreateGuestUser(reqCtx, merchant.Trustap, &trustap.CreateGuestUserRequest{
		CountryCode: body.BuyerCountryCode,
		Email:       body.BuyerEmail,
		FirstName:   body.BuyerFirstName,
		LastName:    body.BuyerLastName,
		TosAcceptance: trustap.TosAcceptance{
			IP:            buyerIP,
			UnixTimestamp: time.Now().Unix(),
		},
	})
	if err != nil {
		return nil, trustapToClientError("create_guest_user", err)
	}

	charge, err := ctx.Trustap.GetCharge(reqCtx, merchant.Trustap, price, currency)
	if err != nil {
		return nil, trustapToClientError("get_charge", err)
	}

	tx, err := ctx.Trustap.CreateTransactionWithGuestUser(reqCtx, merchant.Trustap, &trustap.CreateTransactionRequest{
		BuyerID:                 guest.ID,
		SellerID:                merchant.Trustap.Sub,
		CreatorRole:             "seller",
		Description:             description,
		Currency:                currency,
		DepositPrice:            price,
		DepositCharge:           charge.Charge,
		DepositChargeSeller:     charge.ChargeSeller,
		ChargeCalculatorVersion: charge.ChargeCalculatorVersion,
		SkipRemainder:           true,
	})
	if err != nil {
		return nil, trustapToClientError("create_transaction", err)
	}

	redirectURI := fmt.Sprintf("%s/api/checkouts/%s/return", ctx.PublicBaseURL, checkoutID)
	payURL := ctx.Trustap.PayDepositURL(tx.ID, redirectURI, checkoutID)

	checkout := &store.Checkout{
		ID:               checkoutID,
		MerchantID:       body.MerchantID,
		ProductID:        productID,
		Quantity:         quantity,
		Description:      description,
		PriceMinor:       price,
		ChargeMinor:      charge.Charge,
		Currency:         currency,
		BuyerEmail:       body.BuyerEmail,
		BuyerFirstName:   body.BuyerFirstName,
		BuyerLastName:    body.BuyerLastName,
		BuyerCountryCode: body.BuyerCountryCode,
		BuyerIP:          buyerIP,
		GuestUserID:      guest.ID,
		TransactionID:    tx.ID,
		Status:           store.StatusPendingPayment,
		PayURL:           payURL,
	}
	if err := ctx.Store.CreateCheckout(checkout); err != nil {
		return nil, err
	}

	return checkoutToResponse(checkout), nil
}

func (api *API) GetCheckout(ctx *middleware.Context, checkoutID string) (*core.Checkout, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	checkout, err := ctx.Store.GetCheckout(checkoutID)
	if err != nil {
		return nil, err
	}
	if checkout == nil {
		return nil, swagger_rest.NotFound("checkout '%s' not found", checkoutID)
	}
	return checkoutToResponse(checkout), nil
}

// PaymentReturn is where the Trustap actions page redirects the buyer after
// paying. It only records the return; payment truth arrives via webhook.
func (api *API) PaymentReturn(ctx *middleware.Context, checkoutID string) (*core.PaymentReturn, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}
	checkout, err := ctx.Store.MarkBuyerReturned(checkoutID)
	if err != nil {
		return nil, err
	}
	if checkout == nil {
		return nil, swagger_rest.NotFound("checkout '%s' not found", checkoutID)
	}
	return &core.PaymentReturn{
		CheckoutID: checkout.ID,
		Status:     checkout.Status,
	}, nil
}

// TrustapWebhook stores every event verbatim and best-effort maps it onto the
// matching checkout. The payload format is being confirmed on stage, so
// unknown shapes are accepted (200) and only logged.
func (api *API) TrustapWebhook(ctx *middleware.Context, body *core.TrustapWebhookEvent) (*core.WebhookAck, error) {
	if ctx.Store == nil {
		return nil, fmt.Errorf("service is not fully configured (database missing)")
	}

	event := map[string]any(*body)
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("couldn't re-serialize webhook payload: %w", err)
	}

	transactionID := extractTransactionID(event)
	eventCode := extractEventCode(event)

	if err := ctx.Store.SaveWebhookEvent(&store.WebhookEvent{
		TransactionID: transactionID,
		EventCode:     eventCode,
		Payload:       string(raw),
	}); err != nil {
		return nil, err
	}

	if transactionID != 0 {
		if status, ok := statusFromEvent(event, eventCode); ok {
			checkout, err := ctx.Store.GetCheckoutByTransactionID(transactionID)
			if err != nil {
				return nil, err
			}
			if checkout != nil {
				// First post-purchase action: decrement Index inventory
				// exactly once when the checkout transitions to paid.
				// (Merchant-side sync is the next phase.)
				if status == store.StatusPaid && checkout.Status != store.StatusPaid && checkout.ProductID != "" {
					qty := checkout.Quantity
					if qty < 1 {
						qty = 1
					}
					if err := ctx.Store.DecrementInventory(checkout.ProductID, qty); err != nil {
						return nil, err
					}
				}
				if _, err := ctx.Store.SetStatusByTransactionID(transactionID, status); err != nil {
					return nil, err
				}
			}
		}
	}

	return &core.WebhookAck{Status: "received"}, nil
}

func checkoutToResponse(c *store.Checkout) *core.Checkout {
	createdAt := c.CreatedAt.UTC().Format(time.RFC3339)
	return &core.Checkout{
		ID:            c.ID,
		MerchantID:    c.MerchantID,
		Description:   &c.Description,
		Price:         &c.PriceMinor,
		Charge:        &c.ChargeMinor,
		Currency:      &c.Currency,
		Status:        c.Status,
		TransactionID: &c.TransactionID,
		PayUrl:        &c.PayURL,
		BuyerReturned: &c.BuyerReturned,
		CreatedAt:     &createdAt,
	}
}

// trustapToClientError surfaces Trustap 4xx rejections as 502s with context
// instead of opaque 500s; transport errors stay internal.
func trustapToClientError(step string, err error) error {
	var apiErr *trustap.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return swagger_rest.NewClientErrorWithContext(
			502,
			"trustap_rejected_"+step,
			map[string]any{"trustap_status": apiErr.StatusCode},
			"trustap rejected %s", step,
		)
	}
	return fmt.Errorf("%s failed: %w", step, err)
}

// extractTransactionID finds the Trustap transaction ID in a webhook payload.
// Real stage events (verified 2026-06-12, e.g. basic_tx.paid) carry it as the
// string "target_id" and inside "target_preview".id; older guesses are kept
// as fallbacks.
func extractTransactionID(event map[string]any) int {
	if s, ok := event["target_id"].(string); ok {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	if preview, ok := event["target_preview"].(map[string]any); ok {
		if f, ok := preview["id"].(float64); ok {
			return int(f)
		}
	}
	for _, key := range []string{"transaction_id", "id"} {
		if f, ok := event[key].(float64); ok {
			return int(f)
		}
	}
	return 0
}

func extractEventCode(event map[string]any) string {
	for _, key := range []string{"code", "event", "event_type", "type"} {
		if v, ok := event[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// statusFromEvent maps a Trustap webhook event onto a checkout status. The
// authoritative source is target_preview.status (the transaction's own state,
// verified against real stage events); the event code is the fallback.
// Unknown states change nothing: the raw event is stored either way.
func statusFromEvent(event map[string]any, code string) (string, bool) {
	if preview, ok := event["target_preview"].(map[string]any); ok {
		if s, ok := preview["status"].(string); ok && s != "" {
			switch s {
			case "joined", "created":
				return "", false // pre-payment states; no change
			case "paid", "deposit_paid", "remainder_skipped":
				// p2p: the deposit covers the full price (skip_remainder),
				// so deposit_paid means the buyer has paid in full.
				return store.StatusPaid, true
			case "cancelled":
				return store.StatusCancelled, true
			default:
				// Post-payment lifecycle states (handover confirmed, funds
				// released, deposit refunded) get mapped when the
				// post-purchase phase lands; until then, store only.
				return "", false
			}
		}
	}

	c := strings.ToLower(code)
	switch {
	case strings.Contains(c, "paid") || strings.Contains(c, "payment_accepted"):
		return store.StatusPaid, true
	case strings.Contains(c, "cancelled") || strings.Contains(c, "canceled") || strings.Contains(c, "rejected"):
		return store.StatusCancelled, true
	default:
		return "", false
	}
}
