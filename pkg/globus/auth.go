package globus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAuthTokenURL = "https://auth.globus.org/v2/oauth2/token"
	TransferScope       = "urn:globus:auth:scope:transfer.api.globus.org:all"
)

type ClientCredentialsTokenSource struct {
	clientID     string
	clientSecret string
	scope        string
	tokenURL     string
	httpClient   *http.Client
	now          func() time.Time

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func NewClientCredentialsTokenSource(clientID, clientSecret string) (*ClientCredentialsTokenSource, error) {
	return NewClientCredentialsTokenSourceWithScopeOptions(clientID, clientSecret, TransferScope, DefaultAuthTokenURL, http.DefaultClient)
}

func NewClientCredentialsTokenSourceWithOptions(clientID, clientSecret, tokenURL string, httpClient *http.Client) (*ClientCredentialsTokenSource, error) {
	return NewClientCredentialsTokenSourceWithScopeOptions(clientID, clientSecret, TransferScope, tokenURL, httpClient)
}

func NewClientCredentialsTokenSourceWithScopeOptions(clientID, clientSecret, scope, tokenURL string, httpClient *http.Client) (*ClientCredentialsTokenSource, error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	scope = strings.TrimSpace(scope)
	if clientID == "" {
		return nil, fmt.Errorf("Globus client ID is required")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("Globus client secret is required")
	}
	if scope == "" {
		return nil, fmt.Errorf("Globus Auth scope is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(tokenURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Globus Auth token URL")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ClientCredentialsTokenSource{
		clientID: clientID, clientSecret: clientSecret, scope: scope, tokenURL: parsed.String(), httpClient: httpClient, now: time.Now,
	}, nil
}

func (s *ClientCredentialsTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Add(30*time.Second).Before(s.expiry) {
		return s.token, nil
	}

	form := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {s.scope},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Globus access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", responseError(resp)
	}
	var result struct {
		AccessToken    string `json:"access_token"`
		ResourceServer string `json:"resource_server"`
		ExpiresIn      any    `json:"expires_in"`
		OtherTokens    []struct {
			AccessToken    string `json:"access_token"`
			ResourceServer string `json:"resource_server"`
			ExpiresIn      any    `json:"expires_in"`
		} `json:"other_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Globus token response: %w", err)
	}
	token := result.AccessToken
	expiresIn := result.ExpiresIn
	if result.ResourceServer != "" && result.ResourceServer != "transfer.api.globus.org" {
		token = ""
	}
	for _, candidate := range result.OtherTokens {
		if candidate.ResourceServer == "transfer.api.globus.org" {
			token = candidate.AccessToken
			expiresIn = candidate.ExpiresIn
			break
		}
	}
	if strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("Globus Auth response missing Transfer API access token")
	}
	lifetime, err := tokenLifetime(expiresIn)
	if err != nil {
		return "", err
	}
	s.token = token
	s.expiry = s.now().Add(lifetime)
	return token, nil
}

func tokenLifetime(raw any) (time.Duration, error) {
	var seconds int64
	switch value := raw.(type) {
	case float64:
		seconds = int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid expires_in in Globus Auth response")
		}
		seconds = parsed
	case nil:
		return 0, fmt.Errorf("Globus Auth response missing expires_in")
	default:
		return 0, fmt.Errorf("invalid expires_in in Globus Auth response")
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("invalid expires_in in Globus Auth response")
	}
	return time.Duration(seconds) * time.Second, nil
}
