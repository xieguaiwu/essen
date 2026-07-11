package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"essen/internal/models"
)

// DataDir returns the base data directory: ~/.local/share/essen/.
func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "essen")
}

// EnsureDataDir creates the data directory if it does not exist.
func EnsureDataDir() error {
	return os.MkdirAll(DataDir(), 0755)
}

// DayPath returns the JSON file path for a given date.
// Example: ~/.local/share/essen/2026-07-11.json
func DayPath(date time.Time) string {
	return filepath.Join(DataDir(), date.Format("2006-01-02")+".json")
}

// LoadDay reads all entries for a given date. Returns an empty slice
// (not an error) when the file does not exist.
func LoadDay(date time.Time) ([]models.Entry, error) {
	path := DayPath(date)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取数据文件失败: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	var entries []models.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("解析数据文件失败: %w", err)
	}
	return entries, nil
}

// SaveDay writes a day's entries to its JSON file, creating directories
// as needed. A nil slice is written as an empty JSON array.
func SaveDay(date time.Time, entries []models.Entry) error {
	if err := EnsureDataDir(); err != nil {
		return err
	}

	path := DayPath(date)
	if entries == nil {
		entries = []models.Entry{}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入数据文件失败: %w", err)
	}
	return nil
}

// DeleteEntry removes a single entry by its 1‑based index from the
// given date's file and saves the result.
func DeleteEntry(date time.Time, index int) error {
	entries, err := LoadDay(date)
	if err != nil {
		return err
	}

	if index < 1 || index > len(entries) {
		return fmt.Errorf("序号 %d 超出范围 (1-%d)", index, len(entries))
	}

	entries = append(entries[:index-1], entries[index:]...)
	return SaveDay(date, entries)
}
