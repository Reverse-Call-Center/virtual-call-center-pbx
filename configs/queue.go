package configs

type Queue struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	HoldMusic       string `json:"hold_music"`
	Timeout         int    `json:"timeout"`
	TimeoutMessage  string `json:"timeout_message"`
	AnnounceTime    int    `json:"announce_time"`
	AnnounceMessage string `json:"announce_message"`
}
