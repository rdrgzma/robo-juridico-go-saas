package main

import (
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	u := os.Getenv("HEALTHCHECK_URL")
	if u == "" {
		u = "http://127.0.0.1:8080/health/ready"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	r, err := c.Get(u)
	if err != nil {
		os.Exit(1)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	if r.StatusCode != 200 {
		os.Exit(1)
	}
}
