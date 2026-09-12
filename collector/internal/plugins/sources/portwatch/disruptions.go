// Copyright (c) World Monitor contributors and ION COMMAND contributors.
// SPDX-License-Identifier: AGPL-3.0-only
//
// PortWatch disruption query wiring (ArcGIS FeatureServer URL, SQL
// timestamp literal, and todate-null-means-active filter) was adapted
// from koala73/worldmonitor scripts/seed-portwatch-disruptions.mjs,
// licensed under the GNU Affero General Public License v3.0. This Go
// file is a covered adaptation and is licensed AGPL-3.0-only; see
// licenses/AGPL-3.0.txt. ION COMMAND does not vendor World Monitor's
// TypeScript application.
//
// The FeatureServer is IMF PortWatch public data. That provider's terms
// apply independently of this file's AGPL.

package portwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultDisruptions = "https://services9.arcgis.com/weJ1QsnbMYJlCHdG/arcgis/rest/services/portwatch_disruptions_database/FeatureServer/0/query"
	disruptionDaysBack = 30
	maxDisruptions     = 40
)

// arcgisTimestamp formats a UTC instant the way this FeatureServer's SQL
// parser accepts. Bare epoch milliseconds are rejected ("Cannot perform
// query. Invalid query parameters.").
func arcgisTimestamp(when time.Time) string {
	return when.UTC().Format("2006-01-02 15:04:05")
}

func disruptionsURL(now time.Time) string {
	since := arcgisTimestamp(now.Add(-disruptionDaysBack * 24 * time.Hour))
	values := url.Values{}
	values.Set("where", fmt.Sprintf("todate > timestamp '%s' OR todate IS NULL", since))
	values.Set("outFields", "eventid,eventtype,eventname,alertlevel,country,fromdate,todate,severitytext,lat,long,affectedports,n_affectedports")
	values.Set("orderByFields", "fromdate DESC")
	values.Set("resultRecordCount", "2000")
	values.Set("outSR", "4326")
	values.Set("f", "json")
	return defaultDisruptions + "?" + values.Encode()
}

func (s *Source) sampleDisruptions(ctx context.Context, now time.Time) []plugins.RawRecord {
	body, err := s.fetch(ctx, disruptionsURL(now))
	if err != nil {
		s.logger.Warn("portwatch disruptions fetch failed", "source", s.id, "error", err)
		return nil
	}
	var response arcGIS
	if err := json.Unmarshal(body, &response); err != nil {
		s.logger.Warn("portwatch disruptions decode failed", "source", s.id, "error", err)
		return nil
	}
	records := make([]plugins.RawRecord, 0, len(response.Features))
	for _, feature := range response.Features {
		id := attrString(feature.Attributes, "eventid", "eventId")
		if id == "" || id == "0" {
			continue
		}
		lat, latOK := attrFloat(feature.Attributes, "lat", "latitude")
		lon, lonOK := attrFloat(feature.Attributes, "long", "lon", "longitude")
		if (!latOK || !lonOK) && feature.Geometry != nil {
			lon, lat = feature.Geometry.X, feature.Geometry.Y
			latOK, lonOK = true, true
		}
		if !latOK || !lonOK || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		alert := attrString(feature.Attributes, "alertlevel", "alertLevel")
		payload, err := json.Marshal(map[string]any{
			"kind":       "disruption",
			"eventId":    id,
			"title":      attrString(feature.Attributes, "eventname", "eventName"),
			"category":   attrString(feature.Attributes, "eventtype", "eventType"),
			"alertLevel": alert,
			"country":    attrString(feature.Attributes, "country"),
			"latitude":   lat,
			"longitude":  lon,
			"severity":   attrString(feature.Attributes, "severitytext", "severityText"),
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "portwatch", SourceInstanceID: s.id,
			OriginalID:  "pw-disruption-" + id,
			Domain:      "maritime",
			ObservedUTC: now,
			Payload:     payload,
		})
		if len(records) >= maxDisruptions {
			break
		}
	}
	return records
}
