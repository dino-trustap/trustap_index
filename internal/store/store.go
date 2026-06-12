// Package store is the Postgres persistence layer, built on gorm to match
// the org convention (rest_api/pkg/dbs wraps gorm transactions).
package store

import (
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jinzhu/gorm"

	// Registers the gorm postgres dialect (lib/pq based).
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// Checkout statuses. Webhooks (not the buyer redirect) move a checkout
// through paid and beyond.
const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusCancelled      = "cancelled"
	StatusFailed         = "failed"
)

// Product statuses.
const (
	ProductActive   = "active"
	ProductArchived = "archived"
)

// Product is a catalog item duplicated into the Index from a merchant's
// store. Flat model for v1 (no variants); brand/gtin/mpn feed the GMC and
// JSON-LD identifier fields.
type Product struct {
	ID          string `gorm:"primary_key;size:36"`
	MerchantID  string `gorm:"index;size:100"`
	Title       string
	Description string `gorm:"type:text"`
	PriceMinor  int
	Currency    string `gorm:"size:3"`
	SKU         string `gorm:"size:100"`
	Category    string `gorm:"size:100"`
	ImageURL    string `gorm:"size:2048"`
	Brand       string `gorm:"size:100"`
	GTIN        string `gorm:"size:50"`
	MPN         string `gorm:"size:100"`
	Quantity    int
	Status      string `gorm:"size:20;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Checkout struct {
	ID               string `gorm:"primary_key;size:36"`
	MerchantID       string `gorm:"index;size:100"`
	ProductID        string `gorm:"index;size:36"`
	Quantity         int
	Description      string
	PriceMinor       int
	ChargeMinor      int
	Currency         string `gorm:"size:3"`
	BuyerEmail       string
	BuyerFirstName   string
	BuyerLastName    string
	BuyerCountryCode string `gorm:"size:2"`
	BuyerIP          string
	GuestUserID      string
	TransactionID    int    `gorm:"index"`
	Status           string `gorm:"size:40"`
	PayURL           string `gorm:"size:2048"`
	BuyerReturned    bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// WebhookEvent stores every inbound Trustap webhook verbatim, so status
// mapping can be corrected retroactively while the payload format is still
// being learned on stage.
type WebhookEvent struct {
	ID            uint   `gorm:"primary_key"`
	TransactionID int    `gorm:"index"`
	EventCode     string `gorm:"size:100"`
	Payload       string `gorm:"type:jsonb"`
	CreatedAt     time.Time
}

type Store struct {
	db *gorm.DB
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("couldn't open database: %w", err)
	}
	db = db.AutoMigrate(&Checkout{}, &WebhookEvent{}, &Product{}, &SurfaceHit{})
	if err := db.Error; err != nil {
		return nil, fmt.Errorf("couldn't migrate database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func NewID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("couldn't generate id: %w", err)
	}
	return id.String(), nil
}

// NewCheckoutID is kept for readability at call sites.
func NewCheckoutID() (string, error) { return NewID() }

func (s *Store) CreateCheckout(c *Checkout) error {
	if err := s.db.Create(c).Error; err != nil {
		return fmt.Errorf("couldn't insert checkout: %w", err)
	}
	return nil
}

func (s *Store) GetCheckout(id string) (*Checkout, error) {
	c := &Checkout{}
	res := s.db.Where("id = ?", id).First(c)
	if res.RecordNotFound() {
		return nil, nil
	}
	if err := res.Error; err != nil {
		return nil, fmt.Errorf("couldn't load checkout: %w", err)
	}
	return c, nil
}

func (s *Store) GetCheckoutByTransactionID(transactionID int) (*Checkout, error) {
	c := &Checkout{}
	res := s.db.Where("transaction_id = ?", transactionID).First(c)
	if res.RecordNotFound() {
		return nil, nil
	}
	if err := res.Error; err != nil {
		return nil, fmt.Errorf("couldn't load checkout by transaction: %w", err)
	}
	return c, nil
}

func (s *Store) MarkBuyerReturned(id string) (*Checkout, error) {
	c, err := s.GetCheckout(id)
	if err != nil || c == nil {
		return c, err
	}
	if err := s.db.Model(c).Update("buyer_returned", true).Error; err != nil {
		return nil, fmt.Errorf("couldn't mark buyer returned: %w", err)
	}
	return c, nil
}

// SetStatusByTransactionID updates the checkout for a Trustap transaction.
// Returns false when no checkout matches (event for an unknown transaction).
func (s *Store) SetStatusByTransactionID(transactionID int, status string) (bool, error) {
	res := s.db.Model(&Checkout{}).
		Where("transaction_id = ?", transactionID).
		Update("status", status)
	if err := res.Error; err != nil {
		return false, fmt.Errorf("couldn't update checkout status: %w", err)
	}
	return res.RowsAffected > 0, nil
}

func (s *Store) SaveWebhookEvent(e *WebhookEvent) error {
	if err := s.db.Create(e).Error; err != nil {
		return fmt.Errorf("couldn't insert webhook event: %w", err)
	}
	return nil
}

// --- Products ---

func (s *Store) CreateProduct(p *Product) error {
	if err := s.db.Create(p).Error; err != nil {
		return fmt.Errorf("couldn't insert product: %w", err)
	}
	return nil
}

func (s *Store) GetProduct(id string) (*Product, error) {
	p := &Product{}
	res := s.db.Where("id = ?", id).First(p)
	if res.RecordNotFound() {
		return nil, nil
	}
	if err := res.Error; err != nil {
		return nil, fmt.Errorf("couldn't load product: %w", err)
	}
	return p, nil
}

// ListActiveProducts returns a merchant's active products, newest first.
func (s *Store) ListActiveProducts(merchantID string) ([]Product, error) {
	var products []Product
	err := s.db.
		Where("merchant_id = ? AND status = ?", merchantID, ProductActive).
		Order("created_at desc").
		Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("couldn't list products: %w", err)
	}
	return products, nil
}

// UpdateProduct applies the non-nil fields. Column names are gorm's
// snake_case versions of the struct fields.
func (s *Store) UpdateProduct(id string, fields map[string]any) (*Product, error) {
	if len(fields) > 0 {
		res := s.db.Model(&Product{}).Where("id = ?", id).Updates(fields)
		if err := res.Error; err != nil {
			return nil, fmt.Errorf("couldn't update product: %w", err)
		}
		if res.RowsAffected == 0 {
			return nil, nil
		}
	}
	return s.GetProduct(id)
}

func (s *Store) ArchiveProduct(id string) (bool, error) {
	res := s.db.Model(&Product{}).Where("id = ?", id).Update("status", ProductArchived)
	if err := res.Error; err != nil {
		return false, fmt.Errorf("couldn't archive product: %w", err)
	}
	return res.RowsAffected > 0, nil
}

// DecrementInventory reduces stock after a paid checkout (clamped at zero).
func (s *Store) DecrementInventory(productID string, quantity int) error {
	err := s.db.Model(&Product{}).
		Where("id = ?", productID).
		Update("quantity", gorm.Expr("GREATEST(quantity - ?, 0)", quantity)).Error
	if err != nil {
		return fmt.Errorf("couldn't decrement inventory: %w", err)
	}
	return nil
}

// --- Dashboard: surface analytics, catalog health, recent activity ---

// SurfaceHit records one fetch of a public agent surface (feed, manifest,
// page), so merchants can see whether agents are actually pulling their
// catalog.
type SurfaceHit struct {
	ID         uint   `gorm:"primary_key"`
	MerchantID string `gorm:"index;size:100"`
	Surface    string `gorm:"size:40"`
	UserAgent  string `gorm:"size:400"`
	CreatedAt  time.Time
}

func (s *Store) RecordSurfaceHit(merchantID, surface, userAgent string) error {
	hit := &SurfaceHit{MerchantID: merchantID, Surface: surface, UserAgent: userAgent}
	if err := s.db.Create(hit).Error; err != nil {
		return fmt.Errorf("couldn't record surface hit: %w", err)
	}
	return nil
}

// SurfaceStatus summarises one surface's traffic for a merchant.
type SurfaceStatus struct {
	Surface       string
	LastFetch     *time.Time
	Hits24h       int
	LastUserAgent string
}

func (s *Store) SurfaceStatuses(merchantID string) (map[string]*SurfaceStatus, error) {
	type row struct {
		Surface   string
		LastFetch time.Time
		Hits      int
	}
	var rows []row
	err := s.db.Model(&SurfaceHit{}).
		Select("surface, max(created_at) as last_fetch, count(*) as hits").
		Where("merchant_id = ? AND created_at > ?", merchantID, time.Now().Add(-24*time.Hour)).
		Group("surface").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("couldn't summarise surface hits: %w", err)
	}

	out := map[string]*SurfaceStatus{}
	for i := range rows {
		r := rows[i]
		last := r.LastFetch
		out[r.Surface] = &SurfaceStatus{Surface: r.Surface, LastFetch: &last, Hits24h: r.Hits}
	}

	// Also surface the all-time last fetch for surfaces quiet in the last day.
	var allTime []row
	err = s.db.Model(&SurfaceHit{}).
		Select("surface, max(created_at) as last_fetch, count(*) as hits").
		Where("merchant_id = ?", merchantID).
		Group("surface").
		Scan(&allTime).Error
	if err != nil {
		return nil, fmt.Errorf("couldn't summarise surface hits: %w", err)
	}
	for i := range allTime {
		r := allTime[i]
		if _, ok := out[r.Surface]; !ok {
			last := r.LastFetch
			out[r.Surface] = &SurfaceStatus{Surface: r.Surface, LastFetch: &last, Hits24h: 0}
		}
	}
	return out, nil
}

// ListAllProducts returns every non-archived product including out-of-stock,
// for catalog health reporting.
func (s *Store) ListAllProducts(merchantID string) ([]Product, error) {
	var products []Product
	err := s.db.
		Where("merchant_id = ? AND status != ?", merchantID, ProductArchived).
		Order("created_at desc").
		Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("couldn't list products: %w", err)
	}
	return products, nil
}

func (s *Store) ListRecentCheckouts(merchantID string, limit int) ([]Checkout, error) {
	var checkouts []Checkout
	err := s.db.
		Where("merchant_id = ?", merchantID).
		Order("created_at desc").
		Limit(limit).
		Find(&checkouts).Error
	if err != nil {
		return nil, fmt.Errorf("couldn't list checkouts: %w", err)
	}
	return checkouts, nil
}

// LastWebhookEventTime returns when the most recent Trustap webhook for one
// of the merchant's transactions arrived (zero time when none).
func (s *Store) LastWebhookEventTime(merchantID string) (time.Time, error) {
	type row struct{ Last time.Time }
	var r row
	err := s.db.Model(&WebhookEvent{}).
		Select("max(webhook_events.created_at) as last").
		Joins("JOIN checkouts ON checkouts.transaction_id = webhook_events.transaction_id").
		Where("checkouts.merchant_id = ?", merchantID).
		Scan(&r).Error
	if err != nil {
		return time.Time{}, fmt.Errorf("couldn't query last webhook event: %w", err)
	}
	return r.Last, nil
}

// PaymentsSummary aggregates paid checkout volume for the connection card.
func (s *Store) PaymentsSummary(merchantID string) (paidCount int, revenueMinor int, err error) {
	type row struct {
		Count   int
		Revenue int
	}
	var r row
	err = s.db.Model(&Checkout{}).
		Select("count(*) as count, coalesce(sum(price_minor), 0) as revenue").
		Where("merchant_id = ? AND status = ?", merchantID, StatusPaid).
		Scan(&r).Error
	if err != nil {
		return 0, 0, fmt.Errorf("couldn't summarise payments: %w", err)
	}
	return r.Count, r.Revenue, nil
}
