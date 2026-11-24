package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// AssetStatsFile is where the assets window keeps its daily counts, beside
// the settings in Dir.
const AssetStatsFile = "asset_stats.json"

// assetStatsDays is how many days the file keeps: about half a year, as the
// window's ranges need.
const assetStatsDays = 180

// AssetDay is one day's counts, for the assets window: the logins that
// worked and those that did not, the commands sent, and the risky ones.
type AssetDay struct {
	Day       string `json:"d"`
	Login     int    `json:"lg"`
	LoginFail int    `json:"lf"`
	Cmd       int    `json:"cm"`
	Risk      int    `json:"rk"`
}

// LoadAssetStats reads the daily counts, the oldest first: none when the
// file is missing or unreadable.
func LoadAssetStats() []AssetDay {
	data, err := os.ReadFile(filepath.Join(Dir(), AssetStatsFile))
	if err != nil {
		return nil
	}
	var days []AssetDay
	if json.Unmarshal(data, &days) != nil {
		return nil
	}
	return days
}

// SaveAssetStats writes the daily counts, keeping the last assetStatsDays.
func SaveAssetStats(days []AssetDay) error {
	if len(days) > assetStatsDays {
		days = days[len(days)-assetStatsDays:]
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(days)
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(Dir(), AssetStatsFile), data)
}

// DayKey is the key of t's day in AssetDay.Day: the local date.
func DayKey(t time.Time) string { return t.Format("2006-01-02") }
