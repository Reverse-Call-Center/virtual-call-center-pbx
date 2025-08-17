package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

func ExtractCallerPhone(headers []sip.Header) string {
	for _, header := range headers {
		if header.Name() == "From" {
			from := header.Value()
			if after, ok := strings.CutPrefix(from, "<sip:"); ok {
				trimmed := after
				parts := strings.Split(strings.TrimSuffix(trimmed, ">"), "@")
				return parts[0]
			}
		}
	}
	return "unknown"
}

func GenerateCallID() string {
	return fmt.Sprintf("call_%d", time.Now().UnixNano())
}
