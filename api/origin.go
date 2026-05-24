package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

func allowedOrigin(origin string, r *http.Request) bool {
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	requestHost := strings.ToLower(r.Host)
	if host == requestHost {
		return true
	}

	for _, candidate := range configuredOrigins() {
		if strings.EqualFold(origin, candidate) {
			return true
		}
	}

	return false
}

func configuredOrigins() []string {
	env := strings.TrimSpace(os.Getenv("FS_ENGINE_ALLOWED_ORIGINS"))
	if env == "" {
		return []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:4173",
			"http://127.0.0.1:4173",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
		}
	}

	parts := strings.Split(env, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
