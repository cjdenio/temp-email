package util

import (
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"mime/quotedprintable"
	"regexp"
	"strings"
)

var encodedHeaderRegexp = regexp.MustCompile(`(?i)=\?(?:.+)\?([BQ])\?(.+)\?=`)

func GenerateEmailAddress() string {
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	generated := ""

	for i := 0; i < 6; i++ {
		generated = generated + string(chars[rand.Intn(len(chars))])
	}

	return generated
}

// Parses a mail header if it's formatted like RFC 1342 (https://www.rfc-editor.org/rfc/rfc1342)
func ParseMailHeader(value string) string {
	result := encodedHeaderRegexp.FindStringSubmatch(value)
	if result == nil {
		return value
	}

	switch result[1] {
	case "B":
		fallthrough
	case "b":
		decoded, err := base64.StdEncoding.DecodeString(result[2])
		if err != nil {
			return value
		}

		return string(decoded)
	case "Q":
		fallthrough
	case "q":
		reader := quotedprintable.NewReader(strings.NewReader(strings.Replace(result[2], "_", " ", -1)))
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return value
		}

		return string(decoded)
	}

	fmt.Printf("result: %v\n", result)

	return value
}
