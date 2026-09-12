// headingprobe serves four aircraft on known compass courses so the client's
// glyph rotation can be checked against an answer that is not in doubt.
//
// This is a diagnostic tool, not part of the collector. Point the client at
// it with -IonCollectorUrl=ws://127.0.0.1:7899/ws/live and look at which way
// the noses point: north, east, south, west, arranged in that order around
// the centre. Anything else is a bug in the rotation, and the pattern says
// which one - a uniform twist is an offset, a mirrored pattern is a sign.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/motion"
)

type probe struct {
	name    string
	heading float64
	lat     float64
	lon     float64
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7899", "listen address")
	lat := flag.Float64("lat", 47.5, "centre latitude")
	lon := flag.Float64("lon", 9.2, "centre longitude")
	flag.Parse()

	// Spread far enough apart to tell the glyphs apart at close orbit, and
	// labelled with the course they are flying so the screenshot is
	// self-documenting.
	const spread = 0.35
	probes := []probe{
		{"NORTH-000", 0, *lat + spread, *lon},
		{"EAST-090", 90, *lat, *lon + spread*1.5},
		{"SOUTH-180", 180, *lat - spread, *lon},
		{"WEST-270", 270, *lat, *lon - spread*1.5},
	}
	// Extra aircraft for issue #7: a cruising mover, a turning track,
	// and a hover that must hold its last course despite heading noise.
	type mover struct {
		name    string
		heading float64
		speedKt float64
		lat     float64
		lon     float64
		turnDps float64
		hover   bool
	}
	movers := []mover{
		{name: "MOVE-090", heading: 90, speedKt: 420, lat: *lat + spread*0.4, lon: *lon - spread},
		{name: "TURN-045", heading: 45, speedKt: 280, lat: *lat - spread*0.4, lon: *lon + spread, turnDps: 3},
		{name: "HOVER-000", heading: 0, speedKt: 0, lat: *lat, lon: *lon, hover: true},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	http.HandleFunc("/ws/live", func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer connection.Close()
		log.Println("client connected")
		for tick := 0; ; tick++ {
			now := time.Now().UTC()
			for index, p := range probes {
				envelope := events.NewEnvelope(
					fmt.Sprintf("probe-%d-%d", index, tick),
					"aviation", "aviation.aircraft", events.MessageObservation,
					events.SourceRef{PluginID: "headingprobe", InstanceID: "probe", OriginalID: p.name},
					now)
				envelope.EntityID = "aviation:aircraft:" + p.name
				envelope.Geometry = events.Point(p.lon, p.lat, 10000)
				validUntil := now.Add(30 * time.Second)
				envelope.Time.ValidUntilUTC = &validUntil
				envelope.Properties = map[string]any{
					"visual.icon":          "aircraft",
					"visual.markerScale":   3.0,
					"visual.headingDeg":    p.heading,
					"visual.altitudeScale": 1,
					"display.title":        p.name,
					"display.primary":      fmt.Sprintf("course %.0f", p.heading),
				}
				payload, err := json.Marshal(envelope)
				if err != nil {
					continue
				}
				if err := connection.WriteMessage(websocket.TextMessage, payload); err != nil {
					log.Println("write:", err)
					return
				}
			}
			const dt = 2.0
			for index := range movers {
				m := &movers[index]
				if m.hover {
					m.heading = float64((tick * 37) % 360)
				} else if m.turnDps != 0 {
					m.heading = motion.NormalizeDeg(m.heading + m.turnDps*dt)
				}
				if m.speedKt > 1 {
					distanceDeg := (m.speedKt * 0.514444 * dt) / 111320.0
					rad := m.heading * math.Pi / 180
					m.lat += distanceDeg * math.Cos(rad)
					m.lon += distanceDeg * math.Sin(rad) / math.Max(0.2, math.Cos(m.lat*math.Pi/180))
				}
				envelope := events.NewEnvelope(
					fmt.Sprintf("mover-%d-%d", index, tick),
					"aviation", "aviation.aircraft", events.MessageObservation,
					events.SourceRef{PluginID: "headingprobe", InstanceID: "probe", OriginalID: m.name},
					now)
				envelope.EntityID = "aviation:aircraft:" + m.name
				envelope.Geometry = events.Point(m.lon, m.lat, 10000)
				validUntil := now.Add(30 * time.Second)
				envelope.Time.ValidUntilUTC = &validUntil
				props := map[string]any{
					"visual.icon":          "aircraft",
					"visual.markerScale":   3.0,
					"visual.headingDeg":    m.heading,
					"visual.altitudeScale": 1,
					"display.title":        m.name,
					"display.primary":      fmt.Sprintf("course %.0f  //  %.0f KT", m.heading, m.speedKt),
				}
				if m.speedKt > 1 {
					props["visual.speedMps"] = m.speedKt * 0.514444
				}
				envelope.Properties = props
				payload, err := json.Marshal(envelope)
				if err != nil {
					continue
				}
				if err := connection.WriteMessage(websocket.TextMessage, payload); err != nil {
					log.Println("write:", err)
					return
				}
			}
			time.Sleep(2 * time.Second)
		}
	})
	log.Println("heading probe on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
