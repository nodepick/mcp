package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	const secret = "test-secret-key"

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := authMiddleware(secret, okHandler)

	tests := []struct {
		name       string
		method     string
		target     string
		headerName string
		headerVal  string
		wantStatus int
	}{
		{
			name:       "accepts X-MCP-Key header",
			method:     http.MethodPost,
			target:     "/",
			headerName: "X-MCP-Key",
			headerVal:  secret,
			wantStatus: http.StatusOK,
		},
		{
			name:       "rejects legacy X-API-Key header",
			method:     http.MethodPost,
			target:     "/",
			headerName: "X-API-Key",
			headerVal:  secret,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "accepts Bearer authorization header",
			method:     http.MethodPost,
			target:     "/",
			headerName: "Authorization",
			headerVal:  "Bearer " + secret,
			wantStatus: http.StatusOK,
		},
		{
			name:       "accepts api_key query param",
			method:     http.MethodGet,
			target:     "/?api_key=" + secret,
			wantStatus: http.StatusOK,
		},
		{
			name:       "accepts token query param",
			method:     http.MethodGet,
			target:     "/?token=" + secret,
			wantStatus: http.StatusOK,
		},
		{
			name:       "rejects missing credentials",
			method:     http.MethodPost,
			target:     "/",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "allows OPTIONS preflight without credentials",
			method:     http.MethodOptions,
			target:     "/",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.headerName != "" {
				req.Header.Set(tc.headerName, tc.headerVal)
			}
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
