package geoyandex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vernal96/go-cms-kernel/modules/core/field"
)

func TestSuggestAndGeocodeUseProviderResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "secret" {
			t.Errorf("unexpected API key")
		}
		switch r.URL.Path {
		case "/suggest":
			if r.URL.Query().Get("attrs") != "uri" || r.URL.Query().Get("results") != "10" {
				t.Errorf("suggest query = %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"results":[{"title":{"text":"Moscow"},"subtitle":{"text":"Russia"},"address":{"formatted_address":"Moscow, Russia"},"uri":"ymapsbm1://geo?data=1"}]}`))
		case "/geocode":
			_, _ = w.Write([]byte(`{"response":{"GeoObjectCollection":{"featureMember":[{"GeoObject":{"metaDataProperty":{"GeocoderMetaData":{"text":"Moscow, Russia"}},"Point":{"pos":"37.618423 55.751244"}}}]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service, err := NewService(Config{SuggestAPIKey: "secret", GeocoderAPIKey: "secret", JSAPIKey: "browser-key", HTTPClient: server.Client(), SuggestURL: server.URL + "/suggest", GeocoderURL: server.URL + "/geocode"})
	if err != nil {
		t.Fatal(err)
	}
	suggest, err := service.Suggest(context.Background(), "Moscow")
	if err != nil {
		t.Fatal(err)
	}
	if len(suggest.Results) != 1 || suggest.Results[0].Address != "Moscow, Russia" || suggest.Results[0].URI == "" {
		t.Fatalf("suggest=%+v", suggest)
	}
	geocode, err := service.Geocode(context.Background(), "Moscow")
	if err != nil {
		t.Fatal(err)
	}
	if geocode.Lat != 55.751244 || geocode.Lon != 37.618423 || geocode.Address != "Moscow, Russia" {
		t.Fatalf("geocode=%+v", geocode)
	}
}

func TestCoordinateTypeNormalizationAndStorage(t *testing.T) {
	typeField, err := (coordinateType{}).Compile(field.CompileContext{}, map[string]any{"multiple": true})
	if err != nil {
		t.Fatal(err)
	}
	storage, ok := typeField.(field.StorageValueType)
	if !ok || storage.StorageKind() != field.StorageJSON || !storage.Multiple() {
		t.Fatalf("storage semantics = %#v", typeField)
	}
	value, err := typeField.Normalize([]any{map[string]any{"lat": float64(55), "lon": float64(37)}, map[string]any{"lat": 56, "lon": 38}})
	if err != nil {
		t.Fatal(err)
	}
	coordinates, ok := value.([]any)
	if !ok || len(coordinates) != 2 {
		t.Fatalf("normalized=%#v", value)
	}
	first, firstOK := coordinates[0].(Coordinate)
	second, secondOK := coordinates[1].(Coordinate)
	if !firstOK || !secondOK || first.Lat != 55 || second.Lon != 38 {
		t.Fatalf("normalized=%#v", value)
	}
	if err := typeField.Validate(value); err != nil {
		t.Fatal(err)
	}
	if _, err := typeField.Normalize([]any{map[string]any{"lat": 91, "lon": 38}}); err == nil {
		t.Fatal("expected out of range latitude rejection")
	}
}

type coordinateResolver struct{}

func (coordinateResolver) FieldType(code field.TypeCode) (field.Type, bool) {
	if code != CoordinateType {
		return nil, false
	}
	return field.DescribedType{Type: coordinateType{}, Presentation: field.Metadata{Label: "Координаты", Editor: CoordinateEditor}}, true
}

func TestMultipleCoordinatesCompileAndPersistInOrder(t *testing.T) {
	definition := field.Definition{Key: "location", Type: CoordinateType, Label: "Location", Options: map[string]any{"multiple": true}}
	schema, err := field.CompilePersistent([]field.Definition{definition}, coordinateResolver{})
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := schema.Validate(map[string]any{"location": []any{
		map[string]any{"lat": float64(55.75), "lon": float64(37.61)},
		map[string]any{"lat": float64(56.0), "lon": float64(38.0)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := schema.StoredValues(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("stored rows=%#v", rows)
	}
	want := []Coordinate{{Lat: 55.75, Lon: 37.61}, {Lat: 56, Lon: 38}}
	for index, row := range rows {
		if row.Key != "location" || row.Position != index || row.Kind != field.StorageJSON || !row.Multiple {
			t.Fatalf("row[%d]=%#v", index, row)
		}
		data, err := json.Marshal(row.Value)
		if err != nil {
			t.Fatal(err)
		}
		var coordinate map[string]float64
		if err := json.Unmarshal(data, &coordinate); err != nil {
			t.Fatal(err)
		}
		if coordinate["lat"] != want[index].Lat || coordinate["lon"] != want[index].Lon || len(coordinate) != 2 {
			t.Fatalf("coordinate JSON=%s", data)
		}
	}
	descriptor, err := field.Describe(definition, coordinateResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Editor != CoordinateEditor {
		t.Fatalf("editor metadata=%q, want %q", descriptor.Editor, CoordinateEditor)
	}
}
