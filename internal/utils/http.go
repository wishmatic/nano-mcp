package utils

import "net/url"

func IsHTTP(s string) bool {
	u, err := url.Parse(s)

	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}
