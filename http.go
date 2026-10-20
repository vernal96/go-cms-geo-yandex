package geoyandex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	httptransport "github.com/vernal96/go-cms-kernel/transport/http"
)

func publicHTTP(service *Service, jsAPIKey string, rateLimitPerMinute int, rateLimitKey func(*http.Request) string) httptransport.Builder {
	var limiter *rateLimiter
	if rateLimitPerMinute > 0 {
		limiter = newRateLimiter(rateLimitPerMinute, time.Minute, 2048)
	}
	return httptransport.BuilderFunc(func(context.Context) (httptransport.Contribution, error) {
		if service == nil {
			return httptransport.Contribution{}, errors.New("geo-yandex service is nil")
		}
		limited := func(handler http.Handler) http.Handler {
			if limiter == nil {
				return handler
			}
			return limiter.wrap(handler, rateLimitKey)
		}
		return httptransport.Contribution{Routes: func(registrar httptransport.Registrar) error {
			if err := registrar.Route(httptransport.Route{Name: "geo-yandex.suggests", Method: http.MethodGet, Pattern: "/geo-yandex/suggests", Handler: limited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handleSuggest(w, r, service) }))}); err != nil {
				return err
			}
			if err := registrar.Route(httptransport.Route{Name: "geo-yandex.geocode", Method: http.MethodGet, Pattern: "/geo-yandex/geocode", Handler: limited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handleGeocode(w, r, service) }))}); err != nil {
				return err
			}
			return registrar.Route(httptransport.Route{Name: "geo-yandex.config", Method: http.MethodGet, Pattern: "/geo-yandex/config", Handler: limited(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, struct {
					JSAPIKey string `json:"js_api_key"`
				}{jsAPIKey})
			}))})
		}}, nil
	})
}

func handleSuggest(w http.ResponseWriter, r *http.Request, service *Service) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["text"]) != 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_query", "text must be provided once")
		return
	}
	result, err := service.Suggest(r.Context(), query.Get("text"))
	if err != nil {
		writeServiceError(w, err, "suggestions could not be loaded")
		return
	}
	writeJSON(w, result)
}

func handleGeocode(w http.ResponseWriter, r *http.Request, service *Service) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["address"]) != 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_query", "address must be provided once")
		return
	}
	result, err := service.Geocode(r.Context(), query.Get("address"))
	if err != nil {
		writeServiceError(w, err, "address could not be geocoded")
		return
	}
	writeJSON(w, result)
}

func writeServiceError(w http.ResponseWriter, err error, generic string) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_query", "query must contain 2 to 512 characters")
	case errors.Is(err, ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "no matching address was found")
	default:
		httptransport.WriteJSONError(w, http.StatusBadGateway, "geolocation_unavailable", generic)
	}
}
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	httptransport.WriteJSONError(w, status, code, message)
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

type rateLimiter struct {
	mu         sync.Mutex
	clients    map[string]rateEntry
	limit      int
	window     time.Duration
	maxClients int
}
type rateEntry struct {
	count   int
	expires time.Time
}

func newRateLimiter(limit int, window time.Duration, maxClients int) *rateLimiter {
	return &rateLimiter{clients: make(map[string]rateEntry), limit: limit, window: window, maxClients: maxClients}
}
func (l *rateLimiter) wrap(next http.Handler, resolveKey func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := remoteAddress(r.RemoteAddr)
		if resolveKey != nil {
			key = strings.TrimSpace(resolveKey(r))
		}
		if key == "" {
			httptransport.WriteJSONError(w, http.StatusTooManyRequests, "rate_limited", "request limit exceeded")
			return
		}
		if !l.allow(key, time.Now()) {
			w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
			httptransport.WriteJSONError(w, http.StatusTooManyRequests, "rate_limited", "request limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, exists := l.clients[key]
	if exists && !now.Before(entry.expires) {
		delete(l.clients, key)
		exists = false
	}
	if !exists {
		for client, candidate := range l.clients {
			if !now.Before(candidate.expires) {
				delete(l.clients, client)
			}
		}
		if len(l.clients) >= l.maxClients {
			return false
		}
		entry = rateEntry{expires: now.Add(l.window)}
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.clients[key] = entry
	return true
}
func remoteAddress(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return strings.TrimSpace(address)
}
