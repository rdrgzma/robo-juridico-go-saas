package transport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func TestReadinessAndLivenessIndependent(t *testing.T) {
	for _, ready := range []bool{false, true} {
		h := New([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(ctx context.Context) error {
			if !ready {
				return errors.New("unavailable")
			}
			return nil
		})
		if w := request(h, "GET", "/health/live", "", ""); w.Code != 200 {
			t.Fatal(w.Code)
		}
		expected := 503
		if ready {
			expected = 200
		}
		if w := request(h, "GET", "/health/ready", "", ""); w.Code != expected {
			t.Fatal(w.Code)
		}
		if w := request(h, "GET", "/docs", "", ""); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
}
