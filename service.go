package geoyandex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSuggestURL  = "https://suggest-maps.yandex.ru/v1/suggest"
	defaultGeocoderURL = "https://geocode-maps.yandex.ru/v1/"
	maxInputLength     = 512
	maxResponseBytes   = 1 << 20
)

var ErrInvalidInput = errors.New("invalid geolocation input")

type Service struct {
	suggestKey  string
	geocoderKey string
	client      *http.Client
	suggestURL  string
	geocoderURL string
}

func NewService(config Config) (*Service, error) {
	if strings.TrimSpace(config.SuggestAPIKey) == "" || strings.TrimSpace(config.GeocoderAPIKey) == "" {
		return nil, errors.New("Yandex Suggest and Geocoder API keys are required")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	suggestURL := config.SuggestURL
	if suggestURL == "" {
		suggestURL = defaultSuggestURL
	}
	geocoderURL := config.GeocoderURL
	if geocoderURL == "" {
		geocoderURL = defaultGeocoderURL
	}
	for _, endpoint := range []string{suggestURL, geocoderURL} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" {
			return nil, errors.New("Yandex API endpoint URL is invalid")
		}
	}
	return &Service{suggestKey: config.SuggestAPIKey, geocoderKey: config.GeocoderAPIKey, client: client, suggestURL: suggestURL, geocoderURL: geocoderURL}, nil
}

type Suggestion struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Address  string `json:"address,omitempty"`
	URI      string `json:"uri,omitempty"`
}
type SuggestResult struct {
	Results []Suggestion `json:"results"`
}
type GeocodeResult struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Address string  `json:"address"`
}

func (s *Service) Suggest(ctx context.Context, text string) (SuggestResult, error) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || len(text) > maxInputLength {
		return SuggestResult{}, ErrInvalidInput
	}
	query := url.Values{"text": {text}, "apikey": {s.suggestKey}, "results": {"10"}, "print_address": {"1"}, "attrs": {"uri"}}
	var raw struct {
		Results []struct {
			Title struct {
				Text string `json:"text"`
			} `json:"title"`
			Subtitle struct {
				Text string `json:"text"`
			} `json:"subtitle"`
			Address struct {
				Formatted string `json:"formatted_address"`
			} `json:"address"`
			URI string `json:"uri"`
		} `json:"results"`
	}
	if err := s.getJSON(ctx, s.suggestURL, query, &raw); err != nil {
		return SuggestResult{}, err
	}
	result := SuggestResult{Results: make([]Suggestion, 0, min(len(raw.Results), 10))}
	for _, item := range raw.Results {
		if len(result.Results) == 10 {
			break
		}
		result.Results = append(result.Results, Suggestion{Title: item.Title.Text, Subtitle: item.Subtitle.Text, Address: item.Address.Formatted, URI: item.URI})
	}
	return result, nil
}

func (s *Service) Geocode(ctx context.Context, address string) (GeocodeResult, error) {
	address = strings.TrimSpace(address)
	if len(address) < 2 || len(address) > maxInputLength {
		return GeocodeResult{}, ErrInvalidInput
	}
	query := url.Values{"geocode": {address}, "apikey": {s.geocoderKey}, "format": {"json"}, "results": {"1"}}
	var raw struct {
		Response struct {
			Collection struct {
				Members []struct {
					GeoObject struct {
						Meta struct {
							Geocoder struct {
								Text string `json:"text"`
							} `json:"GeocoderMetaData"`
						} `json:"metaDataProperty"`
						Point struct {
							Position string `json:"pos"`
						} `json:"Point"`
					} `json:"GeoObject"`
				} `json:"featureMember"`
			} `json:"GeoObjectCollection"`
		} `json:"response"`
	}
	if err := s.getJSON(ctx, s.geocoderURL, query, &raw); err != nil {
		return GeocodeResult{}, err
	}
	if len(raw.Response.Collection.Members) == 0 {
		return GeocodeResult{}, ErrNotFound
	}
	item := raw.Response.Collection.Members[0].GeoObject
	parts := strings.Fields(item.Point.Position)
	if len(parts) != 2 {
		return GeocodeResult{}, errors.New("Yandex Geocoder returned an invalid coordinate")
	}
	lon, lonErr := strconv.ParseFloat(parts[0], 64)
	lat, latErr := strconv.ParseFloat(parts[1], 64)
	if lonErr != nil || latErr != nil {
		return GeocodeResult{}, errors.New("Yandex Geocoder returned an invalid coordinate")
	}
	return GeocodeResult{Lat: lat, Lon: lon, Address: item.Meta.Geocoder.Text}, nil
}

var ErrNotFound = errors.New("geolocation result not found")

func (s *Service) getJSON(ctx context.Context, endpoint string, query url.Values, destination any) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("Yandex API request failed")
	}
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return errors.New("Yandex API request failed")
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("Yandex API request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Yandex API returned status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes+1))
	if err := decoder.Decode(destination); err != nil {
		return errors.New("Yandex API returned an invalid response")
	}
	return nil
}
