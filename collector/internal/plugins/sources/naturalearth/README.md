# Natural Earth extracts

`geography.naturalearth` embeds a **110m** public-domain extract next to this
package (`regions.json`, `borders.json`, `places.json`, `rivers.json`). Repo-root
`data/` is gitignored runtime cache and cannot be `go:embed`'d.

| File | Source layer | Use |
| --- | --- | --- |
| `regions.json` | curated continent / region / ocean points | globe labels |
| `borders.json` | `ne_110m_admin_0_boundary_lines_land` | country lines (disputed / LoC / claim omitted) |
| `places.json` | 110m populated places + country label points + elevation points | cities, country names, peaks |
| `rivers.json` | `ne_110m_rivers_lake_centerlines` | sparse named centerlines |

## Recook / bump detail

Download Natural Earth GeoJSON (public domain) and run:

```text
go run ./cmd/neextract -in /path/to/geojson -out internal/plugins/sources/naturalearth
```

Expected filenames are listed on that command. To raise detail later, point
`-in` at the matching **50m** (or 10m) files and keep the same JSON schema;
raise `extractMaxVertices` in `extract.go` if the globe should keep more
vertices. Runtime stays the bundled files — there is no live fetch.
