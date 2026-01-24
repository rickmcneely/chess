package models

import (
	"chess-server/database"
)

func GetSetting(key string) (string, error) {
	var value string
	err := database.DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func SetSetting(key, value string) error {
	_, err := database.DB.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		key, value,
	)
	return err
}

func IsProfanityFilterEnabled() bool {
	value, err := GetSetting("profanity_filter")
	if err != nil {
		return true
	}
	return value == "true"
}

func SetProfanityFilter(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return SetSetting("profanity_filter", value)
}

func IsProfanityAutoWarnEnabled() bool {
	value, err := GetSetting("profanity_auto_warn")
	if err != nil {
		return true // default to enabled
	}
	return value == "true"
}

func SetProfanityAutoWarn(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return SetSetting("profanity_auto_warn", value)
}
