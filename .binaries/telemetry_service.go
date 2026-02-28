//go:build tinygo

package main

import (
	"encoding/json"
	"math/rand"
	"time"
)

type inferenceEvent struct {
	SensorID    string  `json:"sensor_id"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	AltM        int     `json:"alt_m"`
	ThreatClass string  `json:"threat_class"`
	Confidence  float64 `json:"confidence"`
}

//go:wasmexport run
func run() int32 {
	threatClasses := []string{
		"t-90 tank",
		"bm-21 grad artillery",
		"s-400 radar dome",
		"mechanized infantry platoon",
		"hostile uav swarm",
		"forest fire",
		"unidentified supply convoy",
	}

	rand.Seed(time.Now().UnixNano())

	for {
		event := inferenceEvent{
			SensorID:    "watcher-0" + string('1'+rune(rand.Intn(5))),
			Lat:         48.0 + rand.Float64(),
			Lon:         37.0 + rand.Float64(),
			AltM:        rand.Intn(501),
			ThreatClass: threatClasses[rand.Intn(len(threatClasses))],
			Confidence:  0.40 + rand.Float64()*0.59,
		}

		line, _ := json.Marshal(event)
		println(string(line))

		// Much lower event rate to keep rotated files very small.
		sleepMillis := 15000 + rand.Intn(45001)
		time.Sleep(time.Duration(sleepMillis) * time.Millisecond)
	}
}

func main() {}
