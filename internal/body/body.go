package body

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"essen/internal/models"
)

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

// DataDir returns ~/.local/share/essen/body/
func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "essen", "body")
}

// DataPath returns the single measurements file path.
func DataPath() string {
	return filepath.Join(DataDir(), "measurements.json")
}

func ensureDir() error {
	return os.MkdirAll(DataDir(), 0755)
}

// LoadMeasurements reads all body measurements from disk.
// Returns empty slice (not error) when file does not exist.
func LoadMeasurements() ([]models.BodyMeasurement, error) {
	data, err := os.ReadFile(DataPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取体测数据失败: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var measurements []models.BodyMeasurement
	if err := json.Unmarshal(data, &measurements); err != nil {
		return nil, fmt.Errorf("解析体测数据失败: %w", err)
	}
	return measurements, nil
}

// SaveMeasurements writes the full slice to disk, creating dirs as needed.
func SaveMeasurements(measurements []models.BodyMeasurement) error {
	if err := ensureDir(); err != nil {
		return err
	}
	if measurements == nil {
		measurements = []models.BodyMeasurement{}
	}
	data, err := json.MarshalIndent(measurements, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化体测数据失败: %w", err)
	}
	if err := os.WriteFile(DataPath(), data, 0600); err != nil {
		return fmt.Errorf("写入体测数据失败: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Manual entry
// ---------------------------------------------------------------------------

// AddManual creates a manual measurement for today with the given weight.
// If a measurement already exists for today, it is updated.
func AddManual(weightKg float64) error {
	today := time.Now().Format("2006-01-02")

	measurements, err := LoadMeasurements()
	if err != nil {
		return err
	}

	// Update existing entry for today, or append.
	found := false
	for i, m := range measurements {
		if m.Date == today {
			measurements[i].WeightKg = weightKg
			measurements[i].Source = "manual"
			found = true
			break
		}
	}
	if !found {
		measurements = append(measurements, models.BodyMeasurement{
			Date:     today,
			WeightKg: weightKg,
			Source:   "manual",
		})
	}

	return SaveMeasurements(measurements)
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

// MergeMeasurements merges fetched measurements into existing, deduplicating
// by date. Source priority: "xiaomi" > "manual" for same-date conflicts.
// Returns the merged slice sorted by date ascending.
func MergeMeasurements(existing, fetched []models.BodyMeasurement) []models.BodyMeasurement {
	byDate := make(map[string]models.BodyMeasurement)

	// Index existing by date.
	sourceRank := map[string]int{"manual": 0, "xiaomi": 1}
	for _, m := range existing {
		byDate[m.Date] = m
	}

	// Merge fetched: if same date, keep the one with higher source rank.
	for _, m := range fetched {
		key := m.Date
		if prev, ok := byDate[key]; ok {
			if sourceRank[m.Source] >= sourceRank[prev.Source] {
				byDate[key] = m
			}
		} else {
			byDate[key] = m
		}
	}

	// Convert to slice and sort by date ascending.
	merged := make([]models.BodyMeasurement, 0, len(byDate))
	for _, m := range byDate {
		merged = append(merged, m)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Date < merged[j].Date
	})
	return merged
}

// ---------------------------------------------------------------------------
// Trend
// ---------------------------------------------------------------------------

// TrendResult holds computed trend data for display.
type TrendResult struct {
	Latest     models.BodyMeasurement
	Count      int
	FirstDate  string
	LastDate   string
	Delta7Day  float64 // kg, positive = gain
	Delta30Day float64
	WeightMin  float64
	WeightMax  float64
	WeightAvg  float64
}

// ComputeTrend computes trend statistics from measurements.
func ComputeTrend(all []models.BodyMeasurement) TrendResult {
	if len(all) == 0 {
		return TrendResult{}
	}

	t := TrendResult{
		Latest:    all[len(all)-1],
		Count:     len(all),
		FirstDate: all[0].Date,
		LastDate:  all[len(all)-1].Date,
	}

	// Min/max/avg (only non-zero weights).
	var sum float64
	n := 0
	t.WeightMin = math.MaxFloat64
	t.WeightMax = 0
	for _, m := range all {
		if m.WeightKg <= 0 {
			continue
		}
		sum += m.WeightKg
		n++
		if m.WeightKg < t.WeightMin {
			t.WeightMin = m.WeightKg
		}
		if m.WeightKg > t.WeightMax {
			t.WeightMax = m.WeightKg
		}
	}
	if n > 0 {
		t.WeightAvg = sum / float64(n)
	}

	// 7-day delta: compare last measurement to 7 days before
	// 30-day delta: compare last measurement to 30 days before
	now := time.Now()
	sevenDaysAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
	thirtyDaysAgo := now.AddDate(0, 0, -30).Format("2006-01-02")

	latestWeight := t.Latest.WeightKg

	for _, m := range all {
		if m.WeightKg > 0 {
			if m.Date >= sevenDaysAgo && m.Date < t.Latest.Date {
				// Use this as the "7 days ago" reference
				t.Delta7Day = latestWeight - m.WeightKg
			}
			if m.Date >= thirtyDaysAgo && m.Date < t.Latest.Date {
				t.Delta30Day = latestWeight - m.WeightKg
			}
		}
	}

	// If no reference found within 7/30 days, find the closest measurement
	if t.Delta7Day == 0 {
		for _, m := range all {
			if m.WeightKg > 0 && m.Date < t.Latest.Date {
				date, _ := time.Parse("2006-01-02", m.Date)
				latestDate, _ := time.Parse("2006-01-02", t.Latest.Date)
				daysDiff := latestDate.Sub(date).Hours() / 24
				if daysDiff >= 6 && daysDiff <= 8 {
					t.Delta7Day = latestWeight - m.WeightKg
					break
				}
			}
		}
	}
	if t.Delta30Day == 0 {
		for _, m := range all {
			if m.WeightKg > 0 && m.Date < t.Latest.Date {
				date, _ := time.Parse("2006-01-02", m.Date)
				latestDate, _ := time.Parse("2006-01-02", t.Latest.Date)
				daysDiff := latestDate.Sub(date).Hours() / 24
				if daysDiff >= 28 && daysDiff <= 32 {
					t.Delta30Day = latestWeight - m.WeightKg
					break
				}
			}
		}
	}

	return t
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListMeasurements returns all measurements sorted by date + computed trend.
func ListMeasurements() ([]models.BodyMeasurement, TrendResult, error) {
	all, err := LoadMeasurements()
	if err != nil {
		return nil, TrendResult{}, err
	}
	if len(all) == 0 {
		return nil, TrendResult{}, nil
	}
	trend := ComputeTrend(all)
	return all, trend, nil
}

// ---------------------------------------------------------------------------
// Formatting helpers (for CLI display)
// ---------------------------------------------------------------------------

// FormatSource returns a display label for the measurement source.
func FormatSource(source string) string {
	switch source {
	case "manual":
		return "📝 手动"
	case "xiaomi":
		return "📱 小米"
	default:
		return source
	}
}

// FormatDelta formats a weight delta with sign and arrow.
func FormatDelta(deltaKg float64) string {
	if deltaKg == 0 {
		return " 0.0 kg →"
	}
	sign := "+"
	if deltaKg < 0 {
		sign = ""
	}
	arrow := "↑"
	if deltaKg < 0 {
		arrow = "↓"
	}
	return fmt.Sprintf("%s%.1f kg %s", sign, deltaKg, arrow)
}

// FormatWeightBar returns a 10-block bar showing weight relative to min/max range.
func FormatWeightBar(weight, min, max float64) string {
	if max <= min {
		return ""
	}
	pct := (weight - min) / (max - min) * 100
	filled := int(pct / 10.0)
	if filled < 0 {
		filled = 0
	}
	if filled > 10 {
		filled = 10
	}
	empty := 10 - filled
	return fmt.Sprintf("%s%s",
		strings.Repeat("█", filled),
		strings.Repeat("░", empty),
	)
}

// FormatWeightChangeBar returns a colored trend bar for weight change.
func FormatWeightChangeBar(deltaKg float64) string {
	// Map delta range (-2kg to +2kg) to 10 blocks
	normalized := (deltaKg + 2.0) / 4.0 * 10
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 10 {
		normalized = 10
	}
	filled := int(normalized)
	empty := 10 - filled
	return fmt.Sprintf("%s%s",
		strings.Repeat("█", filled),
		strings.Repeat("░", empty),
	)
}
