package nutrition

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ---------------------------------------------------------------------------
// OpenFoodFacts API response types
// ---------------------------------------------------------------------------

// offProduct mirrors a single product entry in the OFF search response.
type offProduct struct {
	ProductName    string                 `json:"product_name"`
	Brands         string                 `json:"brands"`
	Nutriments     map[string]interface{} `json:"nutriments"`
	ServingQuantity json.Number           `json:"serving_quantity"`
	ServingSize    string                 `json:"serving_size"`
}

// offResponse is the top-level JSON returned by the OFF search endpoint.
type offResponse struct {
	Products []offProduct `json:"products"`
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// openFoodFactsLookup queries the OpenFoodFacts database for the given food.
// brand is optional; when empty it is omitted from the request.
//
// Returns:
//   - (*NutritionResult, nil) when a matching product is found.
//   - (nil, nil) when no matching product is found (caller should fall back).
//   - (nil, error) when the API call itself fails.
func openFoodFactsLookup(food string, brand string) (*NutritionResult, error) {
	q := buildQuery(food, brand)
	reqURL := "https://world.openfoodfacts.org/cgi/search.pl?" + q

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("OpenFoodFacts 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenFoodFacts 返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB
	if err != nil {
		return nil, fmt.Errorf("读取 OpenFoodFacts 响应失败: %w", err)
	}

	var sr offResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("解析 OpenFoodFacts 响应失败: %w", err)
	}

	if len(sr.Products) == 0 {
		return nil, nil
	}

	// Iterate products and pick the first one whose name is a reasonable
	// match for the search term. A pure keyword search can return noise.
	for _, p := range sr.Products {
		if p.ProductName == "" {
			continue
		}
		if !nameMatches(food, p.ProductName) {
			continue
		}

		result, err := productToResult(p)
		if err != nil {
			continue // try next product
		}
		return result, nil
	}

	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildQuery constructs a URL-encoded query string for the OFF search API.
func buildQuery(food string, brand string) string {
	v := url.Values{}
	v.Set("search_terms", food)
	v.Set("json", "1")
	v.Set("page_size", "3")

	if brand != "" {
		v.Set("brands", brand)
	}

	return v.Encode()
}

// productToResult converts an OpenFoodFacts product into a NutritionResult.
// Values are returned per 100 g / 100 ml (the native OFF unit).
func productToResult(p offProduct) (*NutritionResult, error) {
	r := &NutritionResult{}

	// --- energy ---
	if v, ok := p.Nutriments["energy-kcal_100g"]; ok {
		r.CaloriesKcal = toFloat(v)
	} else if v, ok := p.Nutriments["energy-kcal_value"]; ok {
		r.CaloriesKcal = toFloat(v)
	}

	// --- macros ---
	if v, ok := p.Nutriments["proteins_100g"]; ok {
		r.ProteinG = toFloat(v)
	}
	if v, ok := p.Nutriments["fat_100g"]; ok {
		r.FatG = toFloat(v)
	}
	if v, ok := p.Nutriments["carbohydrates_100g"]; ok {
		r.CarbsG = toFloat(v)
	}

	// Build a human-readable notes string.
	var notesParts []string
	notesParts = append(notesParts, "OpenFoodFacts")

	if p.ServingSize != "" {
		notesParts = append(notesParts, "份量: "+p.ServingSize)
	}

	// Mention when we have a serving quantity that differs from 100.
	if sq := servingQuantityFloat(p.ServingQuantity); sq > 0 && sq != 100 {
		notesParts = append(notesParts, fmt.Sprintf("每%.0fg", sq))
	}

	r.Notes = strings.Join(notesParts, " | ")

	return r, nil
}

// servingQuantityFloat parses the serving_quantity field into a float64.
// Returns 0 when parsing fails or the field is empty.
func servingQuantityFloat(sq json.Number) float64 {
	if sq == "" {
		return 0
	}
	v, err := sq.Float64()
	if err != nil {
		return 0
	}
	return v
}

// toFloat converts a nutriment value (float64, json.Number, or string)
// into float64. Returns 0 for unrecognised types.
func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

// nameMatches returns true when the search term is a reasonable match for
// the product name. Uses case-insensitive substring and Levenshtein as a
// final fallback to avoid noise from keyword-based search.
func nameMatches(query, productName string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	p := strings.ToLower(strings.TrimSpace(productName))

	if q == "" {
		return false
	}

	// Direct substring match (brand+food often appear inside product name).
	if strings.Contains(p, q) {
		return true
	}

	// For short queries, allow near-exact matches.
	if len([]rune(q)) <= 3 {
		return editDistance([]rune(q), []rune(p)) <= len([]rune(q))
	}

	// For longer queries, require the edit distance not to exceed half the
	// query length — otherwise we are probably looking at an unrelated product.
	return editDistance([]rune(q), []rune(p)) <= len([]rune(q))/2
}

// editDistance computes the Levenshtein distance between two rune slices.
func editDistance(a, b []rune) int {
	// Optimisation: if one is empty, distance is the length of the other.
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use two rows to save memory.
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(
				prev[j]+1,      // deletion
				cur[j-1]+1,     // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// ---------------------------------------------------------------------------
// scaleResultForAmount – adjusts a NutritionResult based on the user's
// amount description.
// ---------------------------------------------------------------------------

// scaleResultForAmount extracts a numeric quantity from the amount string
// and scales the nutrition values proportionally. OFF values are per 100 g/ml
// so if a user writes "250ml" the values are multiplied by 250/100 = 2.5.
//
// Amounts like "1份", "1碗", or "各一份" contain no parseable unit and are
// left unchanged.
func scaleResultForAmount(result *NutritionResult, amount string) {
	n := extractNumber(amount)
	if n <= 0 {
		return
	}

	factor := n / 100.0
	result.CaloriesKcal *= factor
	result.ProteinG *= factor
	result.FatG *= factor
	result.CarbsG *= factor
}

// extractNumber pulls the first contiguous numeric value from a string.
// Examples:
//
//	"250ml"   → 250
//	"200g"    → 200
//	"2个"     → 2
//	"1份"     → 0  ("份" is not a recognised unit)
//	"1碗"     → 0
//	"各一份"   → 0
//
// The heuristic: only digits followed (or preceded) by a recognised unit
// ("g", "ml", "l", "kg", "个", "瓶", "杯", "盒", "碗", "包", "袋", "罐", "听")
// or a pure number are treated as a quantity.
func extractNumber(s string) float64 {
	s = strings.TrimSpace(s)

	// Strip common Chinese quantity prefixes.
	s = strings.TrimPrefix(s, "约")

	// Try to parse as a pure float (e.g. "250").
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Known unit suffixes (Chinese + metric).
	// Note: "份" and "碗" are intentionally excluded — they represent
	// vague serving sizes whose weight cannot be inferred.
	units := []string{"ml", "ML", "mL", "l", "L", "g", "G", "kg", "KG", "Kg",
		"个", "瓶", "杯", "盒", "包", "袋", "罐", "听"}

	for _, u := range units {
		if strings.HasSuffix(s, u) {
			numPart := strings.TrimSuffix(s, u)
			numPart = strings.TrimSpace(numPart)
			if f, err := strconv.ParseFloat(numPart, 64); err == nil {
				return f
			}
		}
	}

	// Fallback: find the first run of digits (including decimal point).
	var digits []rune
	for _, r := range s {
		if unicode.IsDigit(r) || r == '.' {
			digits = append(digits, r)
		} else if len(digits) > 0 {
			// Stop at the first non-digit after we've started collecting.
			break
		}
	}
	if len(digits) == 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(string(digits), 64)
	return f
}
