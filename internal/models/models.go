package models

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
