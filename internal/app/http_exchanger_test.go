package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/go-kit/httpclient"
	"github.com/fromforgesoftware/go-kit/ratelimit"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

type staticSecrets struct {
	id, secret string
	ok         bool
}

func (s staticSecrets) ClientCredentials(string) (string, string, bool) { return s.id, s.secret, s.ok }

func TestHTTPExchanger_RefreshSuccess(t *testing.T) {
	var gotForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotForm = r.Form.Get("grant_type")
		assert.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
		assert.Equal(t, "cid", r.Form.Get("client_id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer srv.Close()

	ex := app.NewHTTPExchanger(httpclient.New(), ratelimit.New(ratelimit.NewInMemoryStore()),
		staticSecrets{id: "cid", secret: "csecret", ok: true},
		app.WithExchangerClock(func() time.Time { return fixedNow }))

	connector := domain.Connector{Slug: "alpaca", TokenURL: srv.URL, Rate: domain.RateProfile{Limit: 100, Window: time.Minute}}
	got, err := ex.Refresh(context.Background(), connector, "old-refresh")
	require.NoError(t, err)
	assert.Equal(t, "refresh_token", gotForm)
	assert.Equal(t, "new-access", got.AccessToken)
	assert.Equal(t, "new-refresh", got.RefreshToken)
	require.NotNil(t, got.ExpiresAt)
	assert.Equal(t, fixedNow.Add(time.Hour), *got.ExpiresAt)
}

func TestHTTPExchanger_InvalidGrantNeedsReauth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token expired"}`))
	}))
	defer srv.Close()

	ex := app.NewHTTPExchanger(httpclient.New(), ratelimit.New(ratelimit.NewInMemoryStore()),
		staticSecrets{ok: false})
	connector := domain.Connector{Slug: "alpaca", TokenURL: srv.URL, Rate: domain.RateProfile{Limit: 100, Window: time.Minute}}

	_, err := ex.Refresh(context.Background(), connector, "dead")
	assert.ErrorIs(t, err, app.ErrNeedsReauth)
}

func TestHTTPExchanger_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","expires_in":3600}`))
	}))
	defer srv.Close()

	ex := app.NewHTTPExchanger(httpclient.New(), ratelimit.New(ratelimit.NewInMemoryStore()),
		staticSecrets{ok: false})
	// Burst of 1: the second refresh within the window is denied.
	connector := domain.Connector{Slug: "alpaca", TokenURL: srv.URL, Rate: domain.RateProfile{Limit: 1, Window: time.Hour}}

	_, err := ex.Refresh(context.Background(), connector, "rt")
	require.NoError(t, err)
	_, err = ex.Refresh(context.Background(), connector, "rt")
	assert.Error(t, err, "second call within the window must be rate limited")
}

func TestHTTPExchanger_NoTokenURL(t *testing.T) {
	ex := app.NewHTTPExchanger(httpclient.New(), ratelimit.New(ratelimit.NewInMemoryStore()), staticSecrets{ok: false})
	_, err := ex.Refresh(context.Background(), domain.Connector{Slug: "binance"}, "rt")
	assert.Error(t, err)
}
