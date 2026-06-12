// Package trustap is a client for the Trustap REST API v1, covering the
// full-seller/guest-buyer "p2p" flow (guest user -> p2p charge -> p2p
// transaction -> pay_deposit actions page). The p2p model is deposit-based:
// with skip_remainder=true the deposit covers the full price, which is how
// the Index takes complete payment. Trustap v2 will generalise from p2p, so
// building on it now minimises later refactoring (decision 2026-06-12).
// Reference: docs/reference/trustap-openapi-v1.yaml and the shopify_plugin
// p2p implementation (trustap.p2p.server.js).
//
// Authentication: HTTP basic with the merchant's API key as username and an
// empty password. Requests made on behalf of the merchant's Trustap account
// (the seller) additionally carry a Trustap-User header with the seller's
// user ID (sub), which the seller authorised once via OAuth consent.
package trustap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Environment string

const (
	EnvTest Environment = "test"
	EnvLive Environment = "live"
)

const (
	apiBaseTest     = "https://dev.stage.trustap.com/api/v1"
	apiBaseLive     = "https://dev.trustap.com/api/v1"
	actionsBaseTest = "https://actions.stage.trustap.com"
	actionsBaseLive = "https://actions.trustap.com"
)

// Credentials identify one merchant's Trustap API client.
type Credentials struct {
	// APIKey is used as the basic auth username (empty password).
	APIKey string
	// Sub is the merchant's Trustap user ID, sent as Trustap-User when
	// acting on the seller's behalf.
	Sub string
}

type Client struct {
	apiBase     string
	actionsBase string
	httpClient  *http.Client
}

func NewClient(env Environment) *Client {
	apiBase, actionsBase := apiBaseTest, actionsBaseTest
	if env == EnvLive {
		apiBase, actionsBase = apiBaseLive, actionsBaseLive
	}
	return &Client{
		apiBase:     apiBase,
		actionsBase: actionsBase,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

type TosAcceptance struct {
	IP            string `json:"ip"`
	UnixTimestamp int64  `json:"unix_timestamp"`
}

type CreateGuestUserRequest struct {
	CountryCode   string        `json:"country_code"`
	Email         string        `json:"email"`
	FirstName     string        `json:"first_name"`
	LastName      string        `json:"last_name"`
	TosAcceptance TosAcceptance `json:"tos_acceptance"`
}

type GuestUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type Charge struct {
	Charge                  int    `json:"charge"`
	ChargeCalculatorVersion int    `json:"charge_calculator_version"`
	ChargeSeller            int    `json:"charge_seller"`
	Currency                string `json:"currency"`
	Price                   int    `json:"price"`
}

// CreateTransactionRequest is the p2p create_with_guest_user body. The
// deposit covers the full price and the remainder is skipped, mirroring the
// shopify_plugin p2p model.
type CreateTransactionRequest struct {
	BuyerID                 string `json:"buyer_id"`
	SellerID                string `json:"seller_id"`
	CreatorRole             string `json:"creator_role"`
	Description             string `json:"description"`
	Currency                string `json:"currency"`
	DepositPrice            int    `json:"deposit_price"`
	DepositCharge           int    `json:"deposit_charge"`
	DepositChargeSeller     int    `json:"deposit_charge_seller"`
	ChargeCalculatorVersion int    `json:"charge_calculator_version"`
	SkipRemainder           bool   `json:"skip_remainder"`
}

// Transaction is the subset of the p2p transaction that the Index tracks.
// Timestamp fields are RFC 3339 strings, empty when the event hasn't
// happened. p2p lifecycle: joined -> deposit_paid -> remainder_skipped ->
// seller_handover_confirmed -> funds_released (or cancelled /
// deposit_refunded).
type Transaction struct {
	ID                      int    `json:"id"`
	Status                  string `json:"status"`
	BuyerID                 string `json:"buyer_id"`
	SellerID                string `json:"seller_id"`
	Description             string `json:"description"`
	Currency                string `json:"currency"`
	DepositPrice            int    `json:"deposit_price"`
	DepositCharge           int    `json:"deposit_charge"`
	Created                 string `json:"created"`
	Joined                  string `json:"joined"`
	DepositPaid             string `json:"deposit_paid"`
	ComplaintPeriodDeadline string `json:"complaint_period_deadline"`
	FundsReleased           string `json:"funds_released"`
	Cancelled               string `json:"cancelled"`
	IsPaymentInProgress     bool   `json:"is_payment_in_progress"`
}

// APIError is a non-2xx response from the Trustap API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("trustap api returned %d: %s", e.StatusCode, e.Body)
}

func (c *Client) CreateGuestUser(ctx context.Context, creds Credentials, req *CreateGuestUserRequest) (*GuestUser, error) {
	out := &GuestUser{}
	err := c.do(ctx, creds, "" /* no Trustap-User */, http.MethodPost, "/guest_users", req, out)
	if err != nil {
		return nil, fmt.Errorf("couldn't create guest user: %w", err)
	}
	return out, nil
}

func (c *Client) GetCharge(ctx context.Context, creds Credentials, price int, currency string) (*Charge, error) {
	path := fmt.Sprintf("/p2p/charge?price=%d&currency=%s", price, url.QueryEscape(currency))
	out := &Charge{}
	err := c.do(ctx, creds, "", http.MethodGet, path, nil, out)
	if err != nil {
		return nil, fmt.Errorf("couldn't get charge: %w", err)
	}
	return out, nil
}

func (c *Client) CreateTransactionWithGuestUser(ctx context.Context, creds Credentials, req *CreateTransactionRequest) (*Transaction, error) {
	out := &Transaction{}
	err := c.do(ctx, creds, creds.Sub, http.MethodPost, "/p2p/me/transactions/create_with_guest_user", req, out)
	if err != nil {
		return nil, fmt.Errorf("couldn't create transaction: %w", err)
	}
	return out, nil
}

func (c *Client) GetTransaction(ctx context.Context, creds Credentials, transactionID int) (*Transaction, error) {
	out := &Transaction{}
	err := c.do(ctx, creds, "", http.MethodGet, fmt.Sprintf("/p2p/transactions/%d", transactionID), nil, out)
	if err != nil {
		return nil, fmt.Errorf("couldn't get transaction: %w", err)
	}
	return out, nil
}

// PayDepositURL builds the Trustap actions page URL where the guest buyer
// pays the deposit (full price under skip_remainder). After payment the buyer
// is redirected to redirectURI with the given state.
func (c *Client) PayDepositURL(transactionID int, redirectURI, state string) string {
	return fmt.Sprintf(
		"%s/f2f/transactions/%d/pay_deposit?redirect_uri=%s&state=%s",
		c.actionsBase,
		transactionID,
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
}

func (c *Client) do(ctx context.Context, creds Credentials, trustapUser, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("couldn't marshal request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, body)
	if err != nil {
		return fmt.Errorf("couldn't build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(creds.APIKey, "")
	if trustapUser != "" {
		req.Header.Set("Trustap-User", trustapUser)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("couldn't read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("couldn't decode response: %w", err)
		}
	}
	return nil
}

// AcceptDeposit captures the buyer's deposit on behalf of the seller. With
// skip_remainder transactions the remainder is auto-skipped AFTER this call
// commits, so confirm_handover must wait for status remainder_skipped (see
// ConfirmHandover).
func (c *Client) AcceptDeposit(ctx context.Context, creds Credentials, transactionID int) error {
	err := c.do(ctx, creds, creds.Sub, http.MethodPost,
		fmt.Sprintf("/p2p/transactions/%d/accept_deposit", transactionID), nil, nil)
	if err != nil {
		return fmt.Errorf("couldn't accept deposit: %w", err)
	}
	return nil
}

// ConfirmHandover confirms the seller handed the item over; after the
// complaint window the funds release. Valid only once the transaction is in
// remainder_skipped (calling earlier races the auto-skip and 400s with
// remainder_required).
func (c *Client) ConfirmHandover(ctx context.Context, creds Credentials, transactionID int) error {
	err := c.do(ctx, creds, creds.Sub, http.MethodPost,
		fmt.Sprintf("/p2p/transactions/%d/confirm_handover", transactionID), nil, nil)
	if err != nil {
		return fmt.Errorf("couldn't confirm handover: %w", err)
	}
	return nil
}
