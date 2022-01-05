package util

import (
	"math/rand"
	"strings"
)

func GenerateEmailAddress() string {
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	generated := ""

	for i := 0; i < 6; i++ {
		generated = generated + string(chars[rand.Intn(len(chars))])
	}

	return generated
}

// Removes @everyone, @channel, and @here
func SanitizeInput(input string) string {
	input = strings.ReplaceAll(input, "@channel", "[redacted]")
	input = strings.ReplaceAll(input, "<!channel>", "[redacted]")

	input = strings.ReplaceAll(input, "@everyone", "[redacted]")
	input = strings.ReplaceAll(input, "<!everyone>", "[redacted]")

	input = strings.ReplaceAll(input, "@here", "[redacted]")
	input = strings.ReplaceAll(input, "<!here>", "[redacted]")

	return input
}
