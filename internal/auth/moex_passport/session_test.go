package moexpassport

import (
	"context"
	"testing"
)

func TestNewSessionValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		login    string
		password string
	}{
		{name: "empty login", login: "", password: "pass"},
		{name: "empty password", login: "user", password: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewSession(context.Background(), tt.login, tt.password)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}
