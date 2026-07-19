// Package cty resolves amateur radio callsigns to their DXCC entity and its
// centroid coordinates using the AD1C country file (cty.dat, courtesy of
// Jim Reisert AD1C, https://www.country-files.com). Positions are entity
// centroids - country-level accuracy by design.
package cty

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

//go:embed cty.dat
var ctyData string

type Entity struct {
	Name      string
	Latitude  float64
	Longitude float64
}

var (
	once     sync.Once
	prefixes map[string]Entity
	exact    map[string]Entity
	maxLen   int
)

func parseFloat(field string) float64 {
	var value float64
	var sign float64 = 1
	field = strings.TrimSpace(field)
	if strings.HasPrefix(field, "-") {
		sign = -1
		field = field[1:]
	}
	whole := true
	var fraction, divisor float64 = 0, 1
	for _, r := range field {
		switch {
		case r == '.':
			whole = false
		case r >= '0' && r <= '9':
			if whole {
				value = value*10 + float64(r-'0')
			} else {
				divisor *= 10
				fraction = fraction*10 + float64(r-'0')
			}
		}
	}
	return sign * (value + fraction/divisor)
}

func load() {
	prefixes = make(map[string]Entity, 4096)
	exact = make(map[string]Entity, 2048)
	scanner := bufio.NewScanner(strings.NewReader(ctyData))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var current Entity
	var haveEntity bool
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, " ") && strings.Contains(line, ":") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 {
				current = Entity{
					Name:     strings.TrimSpace(fields[0]),
					Latitude: parseFloat(fields[4]),
					// cty.dat longitudes are positive WEST.
					Longitude: -parseFloat(fields[5]),
				}
				haveEntity = true
			}
			continue
		}
		if !haveEntity {
			continue
		}
		for _, token := range strings.Split(strings.TrimSuffix(strings.TrimSpace(line), ";"), ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			// Strip zone/continent overrides like K5(4)[7] and time offsets.
			for _, cut := range []string{"(", "[", "<", "{", "~"} {
				if index := strings.Index(token, cut); index >= 0 {
					token = token[:index]
				}
			}
			if token == "" {
				continue
			}
			if strings.HasPrefix(token, "=") {
				exact[strings.ToUpper(token[1:])] = current
				continue
			}
			token = strings.ToUpper(token)
			prefixes[token] = current
			if len(token) > maxLen {
				maxLen = len(token)
			}
		}
	}
}

// Lookup resolves a callsign to its DXCC entity by longest prefix match
// (exact overrides win). Portable suffixes like /P are ignored; a leading
// portable prefix (EA8/HB9ABC) resolves to the operating entity.
func Lookup(callsign string) (Entity, bool) {
	once.Do(load)
	call := strings.ToUpper(strings.TrimSpace(callsign))
	if call == "" {
		return Entity{}, false
	}
	if entity, ok := exact[call]; ok {
		return entity, true
	}
	// EA8/HB9ABC style: the shorter leading part is the operating prefix.
	if index := strings.Index(call, "/"); index > 0 {
		head, tail := call[:index], call[index+1:]
		if len(head) <= 4 && len(head) < len(tail) && len(head) > 0 {
			call = head
		} else {
			call = head
		}
		if entity, ok := exact[call]; ok {
			return entity, true
		}
	}
	limit := len(call)
	if limit > maxLen {
		limit = maxLen
	}
	for length := limit; length > 0; length-- {
		if entity, ok := prefixes[call[:length]]; ok {
			return entity, true
		}
	}
	return Entity{}, false
}
