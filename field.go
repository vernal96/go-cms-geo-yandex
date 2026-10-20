package geoyandex

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/vernal96/go-cms-kernel/modules/core/field"
)

const (
	CoordinateType   field.TypeCode   = "geo-yandex.coordinates"
	CoordinateEditor field.EditorCode = "geo-yandex.coordinates"
)

type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type coordinateType struct{}

func (coordinateType) Code() field.TypeCode { return CoordinateType }
func (coordinateType) Compile(_ field.CompileContext, options any) (field.ValueType, error) {
	var config field.ListOptions
	if options != nil {
		data, err := json.Marshal(options)
		if err != nil {
			return nil, fmt.Errorf("encode coordinate field options: %w", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, errors.New("coordinate field options must contain a boolean multiple property")
		}
	}
	return coordinateValue{multiple: config.Multiple}, nil
}

type coordinateValue struct{ multiple bool }

func (v coordinateValue) Normalize(value any) (any, error) {
	if v.multiple {
		values, ok := coordinateSlice(value)
		if !ok {
			return nil, errors.New("expected an array of coordinates")
		}
		result := make([]any, len(values))
		for i, item := range values {
			coordinate, err := normalizeCoordinate(item)
			if err != nil {
				return nil, fmt.Errorf("coordinate %d: %w", i, err)
			}
			result[i] = coordinate
		}
		return result, nil
	}
	return normalizeCoordinate(value)
}
func (v coordinateValue) Empty(value any) bool {
	if value == nil {
		return true
	}
	if values, ok := value.([]any); ok {
		return len(values) == 0
	}
	return false
}
func (v coordinateValue) Validate(value any) error {
	if v.multiple {
		values, ok := value.([]any)
		if !ok {
			return errors.New("expected an array of coordinates")
		}
		for i, item := range values {
			coordinate, ok := item.(Coordinate)
			if !ok {
				return fmt.Errorf("coordinate %d must be a coordinate object", i)
			}
			if err := validateCoordinate(coordinate); err != nil {
				return fmt.Errorf("coordinate %d: %w", i, err)
			}
		}
		return nil
	}
	coordinate, ok := value.(Coordinate)
	if !ok {
		return errors.New("expected a coordinate")
	}
	return validateCoordinate(coordinate)
}
func (v coordinateValue) Example() any {
	if v.multiple {
		return []any{Coordinate{Lat: 55.751244, Lon: 37.618423}}
	}
	return Coordinate{Lat: 55.751244, Lon: 37.618423}
}
func (v coordinateValue) StorageKind() field.StorageKind { return field.StorageJSON }
func (v coordinateValue) Multiple() bool                 { return v.multiple }

func normalizeCoordinate(value any) (Coordinate, error) {
	switch value := value.(type) {
	case Coordinate:
		if err := validateCoordinate(value); err != nil {
			return Coordinate{}, err
		}
		return value, nil
	case map[string]any:
		if len(value) != 2 {
			return Coordinate{}, errors.New("coordinate must contain only lat and lon")
		}
		latRaw, hasLat := value["lat"]
		lonRaw, hasLon := value["lon"]
		lat, latOK := number(latRaw)
		lon, lonOK := number(lonRaw)
		if !hasLat || !hasLon || !latOK || !lonOK {
			return Coordinate{}, errors.New("lat and lon must be numbers")
		}
		coordinate := Coordinate{Lat: lat, Lon: lon}
		if err := validateCoordinate(coordinate); err != nil {
			return Coordinate{}, err
		}
		return coordinate, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return Coordinate{}, errors.New("expected an object with lat and lon")
		}
		var coordinate Coordinate
		if err := json.Unmarshal(data, &coordinate); err != nil {
			return Coordinate{}, errors.New("expected an object with lat and lon")
		}
		if err := validateCoordinate(coordinate); err != nil {
			return Coordinate{}, err
		}
		return coordinate, nil
	}
}
func coordinateSlice(value any) ([]any, bool) {
	switch values := value.(type) {
	case []any:
		return values, true
	case []Coordinate:
		result := make([]any, len(values))
		for i := range values {
			result[i] = values[i]
		}
		return result, true
	default:
		return nil, false
	}
}
func number(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}
func validateCoordinate(coordinate Coordinate) error {
	if math.IsNaN(coordinate.Lat) || math.IsInf(coordinate.Lat, 0) || coordinate.Lat < -90 || coordinate.Lat > 90 {
		return errors.New("lat must be between -90 and 90")
	}
	if math.IsNaN(coordinate.Lon) || math.IsInf(coordinate.Lon, 0) || coordinate.Lon < -180 || coordinate.Lon > 180 {
		return errors.New("lon must be between -180 and 180")
	}
	return nil
}
