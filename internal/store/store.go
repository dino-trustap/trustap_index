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
	db = db.AutoMigrate(&Checkout{}, &WebhookEvent{}, &Product{})
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
