package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	SIPProtocol             string `json:"sip_protocol"`
	SIPPort                 int    `json:"sip_port"`
	SIPListenAddress        string `json:"sip_listen_address"`
	InitialOptionId         int    `json:"initial_option_id"`
	RecordDisclaimerMessage string `json:"record_disclaimer_message"`
}

func LoadConfig() (*Config, error) {
	// Load configuration from config.json
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "./configs/config.json")

	configData, err := os.ReadFile(configPath)

	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
