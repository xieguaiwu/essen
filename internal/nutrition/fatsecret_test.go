package nutrition

import (
	"fmt"
	"os"
	"testing"
)

// TestFatsecretParsingLive uses a previously downloaded fatsecret.cn HTML
// page to verify the extraction logic end-to-end.
//
// To refresh the cached HTML:
//   curl -s -H 'User-Agent: Mozilla/5.0' \
//     'https://www.fatsecret.cn/%E7%83%AD%E9%87%8F%E8%90%A5%E5%85%BB/search?q=711+%E7%81%AB%E8%85%BF%E4%B8%89%E6%98%8E%E6%B2%BB' \
//     > /tmp/fatsecret_test.html
func TestFatsecretParsingLive(t *testing.T) {
	cachePath := "/tmp/fatsecret_test.html"
	html, err := os.ReadFile(cachePath)
	if err != nil {
		t.Skipf("skipping live test: cannot read %s (run: curl ... > %s)", cachePath, cachePath)
	}

	tests := []struct {
		name       string
		matchBrand string
		wantFood   string
		wantCal    float64
		wantMinFat float64
	}{
		{
			name:       "first result without brand filter",
			matchBrand: "",
			wantFood:   "鸡蛋火腿三明治",
			wantCal:    219,
			wantMinFat: 0, // just verify it parses
		},
		{
			name:       "brand filter 7-11",
			matchBrand: "7-11",
			wantFood:   "鸡蛋火腿三明治",
			wantCal:    219,
			wantMinFat: 0,
		},
		{
			name:       "brand filter 711 (normalised)",
			matchBrand: "711",
			wantFood:   "鸡蛋火腿三明治",
			wantCal:    219,
			wantMinFat: 0,
		},
		{
			name:       "non-matching brand returns first result",
			matchBrand: "蒙牛",
			wantFood:   "鸡蛋火腿三明治",
			wantCal:    219,
			wantMinFat: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFromFatsecretHTML(string(html), "", tt.matchBrand)
			if result == nil {
				if tt.wantFood != "" {
					t.Fatalf("expected result, got nil")
				}
				return
			}

			if result.CaloriesKcal != tt.wantCal {
				t.Errorf("calories = %.0f, want %.0f", result.CaloriesKcal, tt.wantCal)
			}

			if !contains(result.Notes, tt.wantFood) {
				t.Errorf("notes = %q, want containing %q", result.Notes, tt.wantFood)
			}

			t.Logf("food=%q cal=%.0f prot=%.1f fat=%.1f carbs=%.1f",
				result.Notes, result.CaloriesKcal,
				result.ProteinG, result.FatG, result.CarbsG)
		})
	}
}

func TestBrandMatches(t *testing.T) {
	tests := []struct {
		found, user string
		want        bool
	}{
		{"7-11", "7-11", true},
		{"7-11", "711", true},
		{"711", "7-11", true},
		{"7-11", "蒙牛", false},
		{"蒙牛", "蒙牛", true},
		{"蒙牛", "mengniu", false},
		{"", "711", false},
		{"7-11", "", false},

	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.found, tt.user), func(t *testing.T) {
			got := brandMatches(tt.found, tt.user)
			if got != tt.want {
				t.Errorf("brandMatches(%q, %q) = %v, want %v", tt.found, tt.user, got, tt.want)
			}
		})
	}
}

func TestParseFatsecretRow(t *testing.T) {
	// Simulated fragment from a real search result row.
	frag := `" href="/热量营养/7-11/鸡蛋火腿三明治/1份">鸡蛋火腿三明治</a>&nbsp;&nbsp;<a class="brand" href="/热量营养/7-11">(7-11)</a>
									<div class="smallText greyText greyLink">
										每1份 (114克) - 卡路里: 219千卡 | 脂肪: 11.80克 | 碳水物: 16.80克 | 蛋白质: 11.50克
										
										&nbsp;&nbsp;&nbsp;<a href="/热量营养/7-11/鸡蛋火腿三明治/1份">营养成分</a>`

	result, rowBrand := parseFatsecretRow(frag)
	if result == nil {
		t.Fatal("parseFatsecretRow returned nil")
	}

	if result.CaloriesKcal != 219 {
		t.Errorf("calories = %.0f, want 219", result.CaloriesKcal)
	}
	if result.ProteinG != 11.50 {
		t.Errorf("protein = %.2f, want 11.50", result.ProteinG)
	}
	if result.FatG != 11.80 {
		t.Errorf("fat = %.2f, want 11.80", result.FatG)
	}
	if result.CarbsG != 16.80 {
		t.Errorf("carbs = %.2f, want 16.80", result.CarbsG)
	}
	if rowBrand != "7-11" {
		t.Errorf("brand = %q, want 7-11", rowBrand)
	}
	if !contains(result.Notes, "鸡蛋火腿三明治") {
		t.Errorf("notes = %q, want containing 鸡蛋火腿三明治", result.Notes)
	}
	if !contains(result.Notes, "7-11") {
		t.Errorf("notes = %q, want containing 7-11", result.Notes)
	}
}

func TestParseFatsecretRowPer100g(t *testing.T) {
	// Per‑100 g variant from a different search result.
	frag := `" href="/热量营养/7-11/火腿鸡蛋三文治/100克">火腿鸡蛋三文治</a>&nbsp;&nbsp;<a class="brand" href="/热量营养/7-11">(7-11)</a>
									<div class="smallText greyText greyLink">
										每100克 - 卡路里: 289千卡 | 脂肪: 11.30克 | 碳水物: 35.50克 | 蛋白质: 11.20克
										
										&nbsp;&nbsp;&nbsp;<a href="/热量营养/7-11/火腿鸡蛋三文治/100克">营养成分</a>`

	result, rowBrand := parseFatsecretRow(frag)
	if result == nil {
		t.Fatal("parseFatsecretRow returned nil")
	}

	if result.CaloriesKcal != 289 {
		t.Errorf("calories = %.0f, want 289", result.CaloriesKcal)
	}
	if result.ProteinG != 11.20 {
		t.Errorf("protein = %.2f, want 11.20", result.ProteinG)
	}
	if result.FatG != 11.30 {
		t.Errorf("fat = %.2f, want 11.30", result.FatG)
	}
	if result.CarbsG != 35.50 {
		t.Errorf("carbs = %.2f, want 35.50", result.CarbsG)
	}
	if rowBrand != "7-11" {
		t.Errorf("brand = %q, want 7-11", rowBrand)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
