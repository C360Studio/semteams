package weatherstation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

// init registers the WeatherReading payload type with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate WeatherReading payloads
// from JSON when the message type is "weather.station.v1".
//
// CRITICAL: Without this registration, JSON deserialization will fail silently.
func init() {
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "weather",
		Category:    "station",
		Version:     "v1",
		Description: "Weather station reading payload with Graphable implementation",
		Factory: func() any {
			return &WeatherReading{}
		},
		Example: map[string]any{
			"StationID":   "ws-001",
			"Temperature": 22.5,
			"Humidity":    65.0,
			"Condition":   "sunny",
		},
	})
	if err != nil {
		panic("failed to register WeatherReading payload: " + err.Error())
	}
}

// WeatherReading represents a weather station measurement.
type WeatherReading struct {
	// Input fields (from incoming JSON)
	StationID   string    `json:"station_id"`
	Temperature float64   `json:"temperature"`
	Humidity    float64   `json:"humidity"`
	WindSpeed   float64   `json:"wind_speed"`
	Condition   string    `json:"condition"`
	City        string    `json:"city"`
	Country     string    `json:"country"`
	ObservedAt  time.Time `json:"observed_at"`

	// Context fields (set by processor from config)
	OrgID    string `json:"org_id"`
	Platform string `json:"platform"`
}

// EntityID returns a deterministic 6-part federated entity ID.
// Format: {org}.{platform}.{domain}.{system}.{type}.{instance}
// Example: "acme.weather.meteorology.station.outdoor.ws-001"
func (w *WeatherReading) EntityID() string {
	return fmt.Sprintf("%s.%s.meteorology.station.outdoor.%s",
		w.OrgID,
		w.Platform,
		w.StationID,
	)
}

// Triples returns semantic facts about this weather reading.
func (w *WeatherReading) Triples() []message.Triple {
	entityID := w.EntityID()

	triples := []message.Triple{
		// Temperature
		{
			Subject:    entityID,
			Predicate:  PredicateWeatherTempCelsius,
			Object:     w.Temperature,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		},
		// Humidity
		{
			Subject:    entityID,
			Predicate:  PredicateWeatherHumidity,
			Object:     w.Humidity,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		},
		// Condition
		{
			Subject:    entityID,
			Predicate:  PredicateWeatherCondition,
			Object:     w.Condition,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		},
		// Observation timestamp
		{
			Subject:    entityID,
			Predicate:  PredicateObservationRecorded,
			Object:     w.ObservedAt,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		},
	}

	// Optional: Wind speed
	if w.WindSpeed > 0 {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateWeatherWindKph,
			Object:     w.WindSpeed,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		})
	}

	// Optional: City
	if w.City != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateLocationCity,
			Object:     w.City,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		})
	}

	// Optional: Country
	if w.Country != "" {
		triples = append(triples, message.Triple{
			Subject:    entityID,
			Predicate:  PredicateLocationCountry,
			Object:     w.Country,
			Source:     "weather_station",
			Timestamp:  w.ObservedAt,
			Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the message type for weather readings.
// This must match the PayloadRegistration in init().
func (w *WeatherReading) Schema() message.Type {
	return message.Type{
		Domain:   "weather",
		Category: "station",
		Version:  "v1",
	}
}

// Validate checks that the weather reading has all required fields.
func (w *WeatherReading) Validate() error {
	if w.StationID == "" {
		return fmt.Errorf("station_id is required")
	}
	if w.Condition == "" {
		return fmt.Errorf("condition is required")
	}
	if w.OrgID == "" {
		return fmt.Errorf("org_id is required")
	}
	if w.Platform == "" {
		return fmt.Errorf("platform is required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (w *WeatherReading) MarshalJSON() ([]byte, error) {
	type Alias WeatherReading
	return json.Marshal((*Alias)(w))
}

// UnmarshalJSON implements json.Unmarshaler.
func (w *WeatherReading) UnmarshalJSON(data []byte) error {
	type Alias WeatherReading
	return json.Unmarshal(data, (*Alias)(w))
}
