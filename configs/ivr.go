package configs

type Ivr struct {
	WelcomeMessage       string   `json:"welcome_message"`
	Options              []string `json:"options"`
	Timeout              int      `json:"timeout"`
	TimeoutMessage       string   `json:"timeout_message"`
	InvalidOptionMessage string   `json:"invalid_option_message"`
	RecordDisclaimer     string   `json:"record_disclaimer"`
}

type Options struct {
	OptionNumber string `json:"option_number"`
	OptionAction int    `json:"option_action"`
}
