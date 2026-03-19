package throughput

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"
)

// sensorTypes and their measurement units
var sensorTypes = []struct {
	Type string
	Unit string
}{
	{"temperature", "celsius"},
	{"humidity", "percent"},
	{"pressure", "hpa"},
	{"power", "kw"},
	{"vibration", "ips"},
	{"flow", "gpm"},
	{"combustible_gas", "percent_lel"},
	{"illumination", "foot_candles"},
	{"level", "percent"},
	{"noise", "db"},
}

// GenerateSyntheticMessages creates count unique sensor messages with realistic
// entity relationships. Each message produces a unique entity in the graph with
// ~6 triples including one relationship edge to a zone hub node.
//
// Zone distribution: 15 zones, entities distributed evenly across them.
// This concentrates INCOMING_INDEX CAS writes on 15 hot keys.
func GenerateSyntheticMessages(count int) [][]byte {
	rng := rand.New(rand.NewPCG(42, 0)) // deterministic for reproducibility
	messages := make([][]byte, count)
	numZones := 15

	baseLat := 37.77
	baseLon := -122.42
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := range count {
		st := sensorTypes[i%len(sensorTypes)]
		zone := i % numZones

		msg := map[string]any{
			"device_id": fmt.Sprintf("synth-%05d", i),
			"type":      st.Type,
			"reading":   rng.Float64() * 100,
			"unit":      st.Unit,
			"location":  fmt.Sprintf("zone-%02d", zone),
			"timestamp": baseTime.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"serial":    fmt.Sprintf("SN-SYNTH-%05d", i),
			"latitude":  baseLat + (rng.Float64()-0.5)*0.1,
			"longitude": baseLon + (rng.Float64()-0.5)*0.1,
		}

		data, _ := json.Marshal(msg)
		messages[i] = data
	}

	return messages
}
