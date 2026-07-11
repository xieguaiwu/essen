package nutrition

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Compiled regexps for fatsecret.cn HTML scraping
// ---------------------------------------------------------------------------

var (
	// fsTableMarkerRe locates the search-results table.
	fsTableMarkerRe = regexp.MustCompile(`class="generic searchResult"`)

	// fsNutritionRe parses the single-line nutrition summary found inside
	// each result row. Format:
	//   "卡路里: 219千卡 | 脂肪: 11.80克 | 碳水物: 16.80克 | 蛋白质: 11.50克"
	// Capture groups: 1=calories 2=fat 3=carbs 4=protein
	fsNutritionRe = regexp.MustCompile(
		`卡路里:\s*(\d+(?:\.?\d+)?)\s*千卡` +
			`.*?脂肪:\s*(\d+(?:\.?\d+)?)\s*克` +
			`.*?碳水(?:物|化合物):\s*(\d+(?:\.?\d+)?)\s*克` +
			`.*?蛋白质:\s*(\d+(?:\.?\d+)?)\s*克`,
	)
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// fatsecretLookup searches fatsecret.cn for nutritional data.
//
// Strategy:
//  1. Search with "brand food". If the first result's brand matches, return it.
//  2. Otherwise, retry with "food" only and return the first result.
//
// Returns:
//   - (*NutritionResult, nil) when a matching result is found.
//   - (nil, nil) when no result is found (caller should fall back).
//   - (nil, error) when the HTTP request itself fails.
func fatsecretLookup(food string, brand string) (*NutritionResult, error) {
	food = strings.TrimSpace(food)
	brand = strings.TrimSpace(brand)

	// 1. Try "brand food" when brand is provided.
	if brand != "" {
		query := brand + " " + food
		result, err := searchFatsecret(query, brand)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
		// Brand not found in results → fall through to food-only search.
	}

	// 2. Search with food only.
	return searchFatsecret(food, "")
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// searchFatsecret performs a single HTTP GET against the fatsecret.cn search
// endpoint, then parses the HTML for matching results.
//
// When matchBrand is non‑empty only results whose embedded brand link
// matches it are returned.
func searchFatsecret(query string, matchBrand string) (*NutritionResult, error) {
	u := &url.URL{
		Scheme:   "https",
		Host:     "www.fatsecret.cn",
		Path:     "/热量营养/search",
		RawQuery: "q=" + url.QueryEscape(query),
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("fatsecret 创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fatsecret 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 404 → no results; treat as non-error so caller falls back silently.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fatsecret 返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB
	if err != nil {
		return nil, fmt.Errorf("读取 fatsecret 响应失败: %w", err)
	}

	return extractFromFatsecretHTML(string(body), query, matchBrand), nil
}

// ---------------------------------------------------------------------------
// HTML scraping
// ---------------------------------------------------------------------------

// extractFromFatsecretHTML locates the search-results table and iterates
// through individual result rows. When matchBrand is non‑empty only rows
// whose embedded brand link matches it are returned. When matchBrand is
// empty the first row whose food name matches the search query is returned.
func extractFromFatsecretHTML(html, query, matchBrand string) *NutritionResult {
	// 1. Find the results table.
	loc := fsTableMarkerRe.FindStringIndex(html)
	if loc == nil {
		return nil
	}

	// 2. Take a generous chunk that covers the whole table.
	chunkStart := loc[0]
	chunkEnd := chunkStart + 20000
	if chunkEnd > len(html) {
		chunkEnd = len(html)
	}
	chunk := html[chunkStart:chunkEnd]

	// 3. Each result row starts with an <a class="prominent"> tag.
	//    Split on that marker; fragments[1:] are individual results.
	fragments := strings.Split(chunk, `<a class="prominent"`)
	if len(fragments) < 2 {
		return nil
	}

	var firstResult *NutritionResult

	for _, frag := range fragments[1:] {
		result, rowBrand := parseFatsecretRow(frag)
		if result == nil {
			continue
		}

		// Remember the first valid result in case brand filtering fails.
		if firstResult == nil {
			firstResult = result
		}

		// Brand filtering.
		if matchBrand != "" {
			if brandMatches(rowBrand, matchBrand) {
				return result
			}
			continue
		}

		// No brand filter — return the first result whose food name matches the query.
		if nameMatches(query, foodName(result)) {
			return result
		}
		continue
	}

	// No match found — return nil so the caller falls through to other sources.
	return nil
}

// parseFatsecretRow extracts food name, brand, and nutrition data from a
// single search‑result HTML fragment (starting after <a class="prominent").
// Returns nil when the fragment does not contain the minimum required data.
func parseFatsecretRow(frag string) (*NutritionResult, string) {
	// --- food name: text between <a ...> and </a> ---
	nameEnd := strings.Index(frag, "</a>")
	if nameEnd < 0 {
		return nil, ""
	}
	nameStart := strings.Index(frag[:nameEnd], ">")
	if nameStart < 0 {
		return nil, ""
	}
	foodName := strings.TrimSpace(frag[nameStart+1 : nameEnd])
	if foodName == "" {
		return nil, ""
	}

	// --- brand: text inside the next <a class="brand"> tag ---
	var rowBrand string
	brandIdx := strings.Index(frag[nameEnd:], `class="brand"`)
	if brandIdx >= 0 {
		brandStart := nameEnd + brandIdx
		brandTagEnd := strings.Index(frag[brandStart:], "</a>")
		if brandTagEnd >= 0 {
			brandChunk := frag[brandStart : brandStart+brandTagEnd]
			if gt := strings.LastIndex(brandChunk, ">"); gt >= 0 {
				raw := strings.TrimSpace(brandChunk[gt+1:])
				// Strip parentheses, e.g. "(7-11)" → "7-11".
				rowBrand = strings.Trim(raw, "()（）")
			}
		}
	}

	// --- nutrition: single‑line regex ---
	nutMatch := fsNutritionRe.FindStringSubmatch(frag)
	if nutMatch == nil {
		return nil, ""
	}

	calories, _ := strconv.ParseFloat(nutMatch[1], 64)
	fat, _ := strconv.ParseFloat(nutMatch[2], 64)
	carbs, _ := strconv.ParseFloat(nutMatch[3], 64)
	protein, _ := strconv.ParseFloat(nutMatch[4], 64)

	// Build notes: food name + optional brand.
	notes := foodName
	if rowBrand != "" {
		notes += " (" + rowBrand + ")"
	}

	return &NutritionResult{
		CaloriesKcal: calories,
		ProteinG:     protein,
		FatG:         fat,
		CarbsG:       carbs,
		Notes:        notes,
	}, rowBrand
}

// foodName extracts the food name from a NutritionResult's Notes field
// (which stores the name during parsing).
func foodName(r *NutritionResult) string {
	parts := strings.SplitN(r.Notes, " (", 2)
	return parts[0]
}

// brandMatches checks whether a brand extracted from the HTML matches the
// user‑supplied brand string. The comparison is case‑insensitive and
// normalises hyphens so that "7-11" and "711" are treated as equal.
func brandMatches(foundBrand, userBrand string) bool {
	fb := strings.ToLower(strings.TrimSpace(foundBrand))
	ub := strings.ToLower(strings.TrimSpace(userBrand))

	if fb == "" || ub == "" {
		return false
	}

	// Direct substring match.
	if strings.Contains(fb, ub) || strings.Contains(ub, fb) {
		return true
	}

	// Normalise: remove hyphens and compare.
	fbNorm := strings.ReplaceAll(fb, "-", "")
	ubNorm := strings.ReplaceAll(ub, "-", "")
	if fbNorm == ubNorm {
		return true
	}

	return false
}
