package hapi

import "github.com/ion-command/ion-command/collector/internal/iso3166"

func LookupCentroid(iso3 string) (lat, lon float64, name string, ok bool) {
	return iso3166.Lookup(iso3)
}
