# go-cms-geo-yandex

External GO CMS module that contributes the `geo-yandex.coordinates` field and public Yandex geolocation API.

## Add to a profile

```go
import geoyandex "github.com/vernal96/go-cms-geo-yandex"

modules := []kernel.Module{
	core.New(),
	admin.New(),
	geoyandex.New(geoyandex.Config{
		SuggestAPIKey:  os.Getenv("YANDEX_SUGGEST_API_KEY"),
		GeocoderAPIKey: os.Getenv("YANDEX_GEOCODER_API_KEY"),
		JSAPIKey:       os.Getenv("YANDEX_MAPS_JS_API_KEY"),
	}),
}
```

`SuggestAPIKey` and `GeocoderAPIKey` are required. `JSAPIKey` is optional and public; when configured it is returned by `/api/geo-yandex/config`. Restrict it in Yandex Developer Console by allowed referrers/domains. Without it, the API returns an empty `js_api_key`, so the admin editor can use its manual coordinate input fallback. Never use a server key as the JavaScript key.

## Coordinate field

Use field type and editor code `geo-yandex.coordinates`. Its value is `{ "lat": 55.751244, "lon": 37.618423 }`. Set type options to `{ "multiple": true }` for ordered arrays of coordinate objects. The field stores each value as JSON (`lat` and `lon`) and validates WGS84 latitude/longitude ranges.

## Public API

All routes are public and are mounted below the kernel `/api` prefix.

* `GET /api/geo-yandex/suggests?text=Moscow` returns `{ "results": [{ "title": "...", "subtitle": "...", "address": "...", "uri": "..." }] }`.
* `GET /api/geo-yandex/geocode?address=Moscow` returns `{ "lat": 55.75, "lon": 37.61, "address": "..." }`.
* `GET /api/geo-yandex/config` returns `{ "js_api_key": "..." }`.

Requests are limited to 2–512 UTF-8 bytes; the provider call has a five-second default timeout and suggests return at most 10 results. Since the module cannot safely infer a visitor IP behind every deployment's reverse proxy, its optional per-process limiter is disabled by default. Set `RateLimitPerMinute` to enable it; it caps its client table at 2048 entries and uses `RemoteAddr` unless `RateLimitKey` is supplied. Only supply a resolver based on proxy metadata that the application has already validated against its trusted proxy list. The module does not trust forwarded IP headers directly. For multi-instance deployments, enforce a shared edge/API-gateway quota.

Yandex provider errors and keys are never included in HTTP error bodies. The server keys are sent only to the corresponding Yandex server APIs.

## Development

Run tests with `go test ./...`.
