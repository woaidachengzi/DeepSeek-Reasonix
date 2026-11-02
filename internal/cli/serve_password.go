package cli

import (
	"fmt"
	"os"

	"reasonix/internal/i18n"
	"reasonix/internal/serve"
)

// A private file takes precedence over argv for both server startup and hash
// generation. Reuse the token-file single-line, size and permission boundary.
func servePassword(password, file string) (string, error) {
	if file == "" {
		return password, nil
	}
	return readServeTokenFile(file)
}

func printServePasswordHash(password string) int {
	if password == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "--hash-password requires --password-file or --password")
		return 1
	}
	hash, err := serve.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	fmt.Println(hash)
	return 0
}
