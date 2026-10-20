package geoyandex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httptransport "github.com/vernal96/go-cms-kernel/transport/http"
)

type testRegistrar struct {
	routes map[string]httptransport.Route
}

func (r *testRegistrar) Route(route httptransport.Route) error {
	if r.routes == nil {
		r.routes = map[string]httptransport.Route{}
	}
	r.routes[route.Pattern] = route
	return nil
}
func (*testRegistrar) Mount(httptransport.Mount) error { return nil }
func (*testRegistrar) Group(string, []httptransport.MiddlewareCode, httptransport.RegisterRoutes) error {
	return nil
}

func TestPublicRoutesReturnDTOs(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/suggest" {
			_, _ = w.Write([]byte(`{"results":[{"title":{"text":"Moscow"},"address":{"formatted_address":"Moscow"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"GeoObjectCollection":{"featureMember":[{"GeoObject":{"metaDataProperty":{"GeocoderMetaData":{"text":"Moscow"}},"Point":{"pos":"37 55"}}}]}}}`))
	}))
	defer provider.Close()
	service, err := NewService(Config{SuggestAPIKey: "s", GeocoderAPIKey: "g", JSAPIKey: "browser-key", HTTPClient: provider.Client(), SuggestURL: provider.URL + "/suggest", GeocoderURL: provider.URL + "/geocode"})
	if err != nil {
		t.Fatal(err)
	}
	builder := publicHTTP(service, "browser-key", 0, nil)
	contribution, err := builder.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	registrar := &testRegistrar{}
	if err := contribution.Routes(registrar); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path, query string
		wantStatus  int
		want        any
	}{
		{path: "/geo-yandex/suggests", query: "text=Moscow", wantStatus: 200, want: map[string]any{"results": []any{map[string]any{"title": "Moscow", "address": "Moscow"}}}},
		{path: "/geo-yandex/geocode", query: "address=Moscow", wantStatus: 200, want: map[string]any{"lat": float64(55), "lon": float64(37), "address": "Moscow"}},
		{path: "/geo-yandex/config", wantStatus: 200, want: map[string]any{"js_api_key": "browser-key"}},
	}
	for _, test := range cases {
		route := registrar.routes[test.path]
		if route.Handler == nil {
			t.Fatalf("route %s not registered", test.path)
		}
		request := httptest.NewRequest(http.MethodGet, test.path+"?"+test.query, nil)
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		route.Handler.ServeHTTP(response, request)
		if response.Code != test.wantStatus {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		var got any
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !jsonEqual(got, test.want) {
			t.Fatalf("%s response=%#v want=%#v", test.path, got, test.want)
		}
	}
}
func jsonEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func TestRateLimiterBoundsClientsAndResetsWindow(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute, 1)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if !limiter.allow("192.0.2.1", now) || !limiter.allow("192.0.2.1", now) {
		t.Fatal("initial requests should pass")
	}
	if limiter.allow("192.0.2.1", now) {
		t.Fatal("request over per-client limit should be rejected")
	}
	if limiter.allow("192.0.2.2", now) {
		t.Fatal("new client should be rejected when table is full")
	}
	if !limiter.allow("192.0.2.2", now.Add(time.Minute)) {
		t.Fatal("expired entries should be removed")
	}
}
