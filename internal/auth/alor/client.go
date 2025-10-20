package alor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Credentials описывают параметры авторизации для АЛОР OpenAPI.
type Credentials struct {
	ClientID     string
	RefreshToken string
}

// Client отвечает за получение и обновление access token для работы с API.
type Client struct {
	credentials Credentials

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewClient создаёт клиента авторизации, проверяя наличие обязательных полей.
func NewClient(credentials Credentials) (*Client, error) {
	if credentials.ClientID == "" {
		return nil, errors.New("alor: client id не указан")
	}
	if credentials.RefreshToken == "" {
		return nil, errors.New("alor: refresh token не указан")
	}

	return &Client{credentials: credentials}, nil
}

// AccessToken возвращает валидный токен. При необходимости выполняется обновление.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.expiresAt) && c.accessToken != "" {
		return c.accessToken, nil
	}

	if err := c.refreshLocked(ctx); err != nil {
		return "", err
	}

	return c.accessToken, nil
}

func (c *Client) refreshLocked(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// В реальной реализации здесь был бы HTTP-запрос к OpenAPI АЛОР.
	// Для прототипа генерируем псевдо-токен, валидный в течение часа.
	c.accessToken = fmt.Sprintf("mock-token-%d", time.Now().Unix())
	c.expiresAt = time.Now().Add(time.Hour)

	return nil
}
