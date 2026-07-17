package models

// BodyMeasurement represents a single body composition measurement
// from a smart scale or manual entry.
type BodyMeasurement struct {
	Date         string  `json:"date"`           // "2006-01-02"
	WeightKg     float64 `json:"weight_kg"`
	BodyFatPct   float64 `json:"body_fat_pct"`
	MuscleMassKg float64 `json:"muscle_mass_kg"`
	BoneMassKg   float64 `json:"bone_mass_kg"`
	WaterPct     float64 `json:"water_pct"`
	BMRKcal      float64 `json:"bmr_kcal"`
	Source       string  `json:"source"` // "manual" | "xiaomi"
}

// Entry represents a single food diary entry.
type Entry struct {
	Timestamp    string  `json:"timestamp"`
	Brand        string  `json:"brand"`         // optional brand name, e.g. 711, 蒙牛
	Food         string  `json:"food"`
	Amount       string  `json:"amount"`
	CaloriesKcal float64 `json:"calories_kcal"`
	ProteinG     float64 `json:"protein_g"`
	FatG         float64 `json:"fat_g"`
	CarbsG       float64 `json:"carbs_g"`
	Notes        string  `json:"notes"`
}
