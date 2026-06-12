// Package surfaces serves the public, crawlable discovery surfaces that
// produce non-JSON output and therefore live outside the swagger codegen:
//
//	GET /feeds/{merchant_id}/gmc.csv         Google-Merchant-Center-spec CSV
//	GET /shop/{merchant_id}                  HTML product listing
//	GET /shop/{merchant_id}/products/{id}    HTML product page with JSON-LD
//
// These exist because feed-driven discovery (ChatGPT post-pivot, Microsoft
// Merchant Center, the Perplexity Merchant Program) requires a GMC feed plus
// public product pages carrying Schema.org Product/Offer JSON-LD; a headless
// API alone fails their ingestion.
package surfaces

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/trustap/trustap_index/internal/store"
)

type Handler struct {
	Store         *store.Store
	MerchantNames map[string]string
	PublicBaseURL string
}

// Handle serves the request when its path matches a public surface and
// reports whether it did. Non-matching requests fall through to the API
// router.
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.Store == nil || r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path

	if strings.HasPrefix(path, "/feeds/") && strings.HasSuffix(path, "/gmc.csv") {
		merchantID := strings.TrimSuffix(strings.TrimPrefix(path, "/feeds/"), "/gmc.csv")
		h.gmcCSV(w, merchantID)
		return true
	}

	if strings.HasPrefix(path, "/shop/") {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/shop/"), "/"), "/")
		switch {
		case len(parts) == 1 && parts[0] != "":
			h.shopPage(w, parts[0])
			return true
		case len(parts) == 3 && parts[1] == "products":
			h.productPage(w, parts[0], parts[2])
			return true
		}
	}

	return false
}

func (h *Handler) merchantName(merchantID string) (string, bool) {
	name, ok := h.MerchantNames[merchantID]
	return name, ok
}

func (h *Handler) gmcCSV(w http.ResponseWriter, merchantID string) {
	if _, ok := h.merchantName(merchantID); !ok {
		http.Error(w, "merchant not found", http.StatusNotFound)
		return
	}
	products, err := h.Store.ListActiveProducts(merchantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", merchantID+"-gmc.csv"))

	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"id", "title", "description", "link", "image_link", "condition",
		"availability", "price", "brand", "gtin", "mpn", "identifier_exists", "product_type",
	})
	for i := range products {
		p := &products[i]
		availability := "in_stock"
		if p.Quantity <= 0 {
			availability = "out_of_stock"
		}
		identifierExists := "no"
		if p.Brand != "" || p.GTIN != "" || p.MPN != "" {
			identifierExists = "yes"
		}
		description := p.Description
		if description == "" {
			description = p.Title
		}
		_ = writer.Write([]string{
			p.ID,
			p.Title,
			description,
			h.productURL(p),
			p.ImageURL,
			"new",
			availability,
			price(p),
			p.Brand,
			p.GTIN,
			p.MPN,
			identifierExists,
			p.Category,
		})
	}
	writer.Flush()
}

func (h *Handler) shopPage(w http.ResponseWriter, merchantID string) {
	name, ok := h.merchantName(merchantID)
	if !ok {
		http.Error(w, "merchant not found", http.StatusNotFound)
		return
	}
	products, err := h.Store.ListActiveProducts(merchantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var cards strings.Builder
	for i := range products {
		p := &products[i]
		fmt.Fprintf(
			&cards, `<div class="card">
<h2><a href="%s">%s</a></h2>
<p>%s</p>
<p><span class="price">%s</span> · %s</p>
</div>
`,
			html.EscapeString(h.productURL(p)),
			html.EscapeString(p.Title),
			html.EscapeString(p.Description),
			html.EscapeString(price(p)),
			stockLabel(p),
		)
	}
	if len(products) == 0 {
		cards.WriteString("<p>No products available.</p>")
	}

	body := fmt.Sprintf("<h1>%s</h1>\n<p>%d products</p>\n%s",
		html.EscapeString(name), len(products), cards.String())
	writePage(w, name, body)
}

func (h *Handler) productPage(w http.ResponseWriter, merchantID, productID string) {
	name, ok := h.merchantName(merchantID)
	if !ok {
		http.Error(w, "merchant not found", http.StatusNotFound)
		return
	}
	p, err := h.Store.GetProduct(productID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil || p.MerchantID != merchantID || p.Status != store.ProductActive {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	jsonLD, err := json.MarshalIndent(h.productJSONLD(p, name), "", " ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	imageHTML := ""
	if p.ImageURL != "" {
		imageHTML = fmt.Sprintf(`<img src="%s" alt="%s">`,
			html.EscapeString(p.ImageURL), html.EscapeString(p.Title))
	}
	skuHTML := ""
	if p.SKU != "" {
		skuHTML = fmt.Sprintf("<p>SKU: %s</p>", html.EscapeString(p.SKU))
	}

	buyHTML := ""
	if p.Quantity > 0 {
		buyHTML = fmt.Sprintf(`<div class="buy">
<h3>Buy now</h3>
<form id="buy-form">
<div class="buy-grid">
<input required type="email" id="b-email" placeholder="Email" autocomplete="email">
<input required id="b-first" placeholder="First name" autocomplete="given-name">
<input required id="b-last" placeholder="Last name" autocomplete="family-name">
<select id="b-country" aria-label="Country">
<option value="IE">Ireland</option><option value="GB">United Kingdom</option>
<option value="DE">Germany</option><option value="FR">France</option>
<option value="HR">Croatia</option><option value="NL">Netherlands</option>
<option value="ES">Spain</option><option value="IT">Italy</option>
<option value="US">United States</option>
</select>
<input type="number" id="b-qty" min="1" max="%d" value="1" aria-label="Quantity">
</div>
<button type="submit" class="buy-btn" id="buy-btn">Buy now &middot; <span id="buy-total">%s</span></button>
<p id="buy-error" class="buy-error" hidden></p>
<p class="buy-note">Secure payment with buyer protection by Trustap.</p>
</form>
</div>
<script>
(function(){
var unit=%d, cur=%q, maxQ=%d;
var qty=document.getElementById('b-qty'), total=document.getElementById('buy-total');
var btn=document.getElementById('buy-btn'), err=document.getElementById('buy-error');
function fmt(m){return (m/100).toFixed(2)+' '+cur.toUpperCase();}
function current(){return Math.min(maxQ, Math.max(1, parseInt(qty.value||'1',10)));}
qty.addEventListener('input', function(){ total.textContent = fmt(unit*current()); });
document.getElementById('buy-form').addEventListener('submit', function(ev){
ev.preventDefault();
btn.disabled = true; btn.textContent = 'Preparing secure payment...'; err.hidden = true;
fetch('/api/checkouts', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({
merchant_id: %q,
product_id: %q,
quantity: current(),
buyer_email: document.getElementById('b-email').value,
buyer_first_name: document.getElementById('b-first').value,
buyer_last_name: document.getElementById('b-last').value,
buyer_country_code: document.getElementById('b-country').value
})}).then(function(r){ return r.json().then(function(d){ return {ok: r.ok, d: d}; }); })
.then(function(res){
if (res.ok && res.d.pay_url) { location.href = res.d.pay_url; return; }
err.textContent = (res.d && res.d.message) || 'Could not start checkout.';
err.hidden = false; btn.disabled = false;
btn.innerHTML = 'Buy now &middot; ' + fmt(unit*current());
}).catch(function(){
err.textContent = 'Network error, please try again.';
err.hidden = false; btn.disabled = false;
btn.innerHTML = 'Buy now &middot; ' + fmt(unit*current());
});
});
})();
</script>`,
			p.Quantity, price(p), p.PriceMinor, p.Currency, p.Quantity, merchantID, p.ID)
	}

	body := fmt.Sprintf(
		`<script type="application/ld+json">
%s
</script>
<p><a href="/shop/%s">&larr; %s</a></p>
<h1>%s</h1>
%s
<p>%s</p>
<p><span class="price">%s</span> · %s</p>
%s
%s`,
		jsonLD,
		html.EscapeString(merchantID),
		html.EscapeString(name),
		html.EscapeString(p.Title),
		imageHTML,
		html.EscapeString(p.Description),
		html.EscapeString(price(p)),
		stockLabel(p),
		skuHTML,
		buyHTML,
	)
	writePage(w, p.Title+" · "+name, body)
}

func (h *Handler) productJSONLD(p *store.Product, merchantName string) map[string]any {
	availability := "https://schema.org/InStock"
	if p.Quantity <= 0 {
		availability = "https://schema.org/OutOfStock"
	}
	data := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"name":        p.Title,
		"description": p.Description,
		"offers": map[string]any{
			"@type":         "Offer",
			"url":           h.productURL(p),
			"price":         fmt.Sprintf("%.2f", float64(p.PriceMinor)/100),
			"priceCurrency": strings.ToUpper(p.Currency),
			"availability":  availability,
			"seller":        map[string]any{"@type": "Organization", "name": merchantName},
		},
	}
	if p.SKU != "" {
		data["sku"] = p.SKU
	}
	if p.ImageURL != "" {
		data["image"] = p.ImageURL
	}
	if p.Brand != "" {
		data["brand"] = map[string]any{"@type": "Brand", "name": p.Brand}
	}
	if p.GTIN != "" {
		data["gtin"] = p.GTIN
	}
	if p.MPN != "" {
		data["mpn"] = p.MPN
	}
	return data
}

func (h *Handler) productURL(p *store.Product) string {
	return fmt.Sprintf("%s/shop/%s/products/%s", h.PublicBaseURL, p.MerchantID, p.ID)
}

func price(p *store.Product) string {
	return fmt.Sprintf("%.2f %s", float64(p.PriceMinor)/100, strings.ToUpper(p.Currency))
}

func stockLabel(p *store.Product) string {
	if p.Quantity > 0 {
		return `<span class="stock-in">In stock</span>`
	}
	return `<span class="stock-out">Out of stock</span>`
}

func writePage(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
body { font-family: -apple-system, system-ui, sans-serif; max-width: 720px; margin: 2rem auto; padding: 0 1rem; color: #1a1a1a; }
a { color: #0a5c99; }
.price { font-size: 1.4rem; font-weight: 600; }
.stock-in { color: #1a7f37; }
.stock-out { color: #b91c1c; }
.card { border: 1px solid #e0e0e0; border-radius: 8px; padding: 1rem; margin: 1rem 0; }
img { max-width: 100%%; border-radius: 8px; }
footer { margin-top: 3rem; font-size: 0.85rem; color: #666; }
.buy { border: 1px solid #e3e8ee; border-radius: 10px; padding: 1.1rem 1.2rem; margin-top: 1.6rem; background: #fafbfc; }
.buy h3 { margin: 0 0 0.8rem; font-size: 1rem; }
.buy-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.55rem; margin-bottom: 0.8rem; }
.buy-grid input[type=email] { grid-column: 1 / -1; }
.buy-grid input, .buy-grid select { font: inherit; padding: 0.55rem 0.7rem; border: 1px solid #d5dbe3; border-radius: 7px; background: #fff; }
.buy-grid input:focus, .buy-grid select:focus { outline: none; border-color: #2949ce; box-shadow: 0 0 0 3px #eef2ff; }
.buy-btn { font: inherit; font-weight: 600; width: 100%; background: #2949ce; color: #fff; border: none; border-radius: 8px; padding: 0.7rem 1rem; cursor: pointer; }
.buy-btn:hover { background: #1f3cab; }
.buy-btn:disabled { opacity: 0.7; cursor: wait; }
.buy-error { color: #cd3d64; font-size: 0.85rem; }
.buy-note { color: #8792a2; font-size: 0.78rem; margin: 0.6rem 0 0; }
</style>
</head>
<body>
%s
<footer>Buyer-protected checkout powered by Trustap.</footer>
</body>
</html>`, html.EscapeString(title), body)
}
