package config

import (
	"encoding/json"
	"log"
	"os"
)

type QueueConfig struct {
	Queues map[int]*Queue `json:"queues"`
}

type Queue struct {
	OptionId        int    `json:"option_id"`
	HoldMusic       string `json:"hold_music"`
	Timeout         int    `json:"timeout"`
	TimeoutMessage  string `json:"timeout_message"`
	TimeoutAction   int    `json:"timeout_action"`
	AnnounceTime    int    `json:"announce_time"`
	AnnounceMessage string `json:"announce_message"`
}

// LoadQueueConfig Loads all the Queue configurations from the JSON file
func LoadQueueConfig() *QueueConfig {
	file, err := os.Open("./configs/queue.json")
	if err != nil {
		log.Fatalf("error opening config file: %v", err)
	}
	defer file.Close()

	var config QueueConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		log.Fatalf("error decoding config JSON: %v", err)
	}

	return &config
}
