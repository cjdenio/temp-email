package util

import "math/rand"

func GenerateEmailAddress() string {
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	generated := ""

	for i := 0; i < 6; i++ {
		generated = generated + string(chars[rand.Intn(len(chars))])
	}

	return generated
}
