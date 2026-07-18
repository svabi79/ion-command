# Canonical data contract

The normative schema is `schemas/canonical-envelope.schema.json`. Transport is
newline-free JSON per WebSocket message for bootstrap; JSONL uses one identical
object per line. A later MessagePack or Protobuf transport must preserve these
semantics.

## Envelope invariants

- `schemaVersion` is required and currently `1`.
- `messageId` is stable and unique within a source instance.
- `messageType` describes lifecycle/shape semantics; `semanticType` describes
  domain meaning.
- `source.pluginId` and `source.instanceId` are required.
- all timestamps are UTC; observed, received, valid-from, processing, and
  optional valid-until are distinct.
- WGS84 coordinates use GeoJSON order: longitude, latitude, optional altitude.
- frequency is Hertz; domain units must be explicit.
- absent data is omitted or `null`; it is never invented.
- consumers ignore unknown properties and retain unknown semantic types.

## Implemented geometry payloads

Point:

```json
{"type":"Point","coordinates":[8.3,47.2,500],"crs":"EPSG:4326"}
```

GreatCircle:

```json
{"type":"GreatCircle","coordinates":[[8.3,47.2],[-73.9,41.0]],"crs":"EPSG:4326"}
```

## Radio mapping

A spot produces endpoint entities the first time an endpoint identity is seen,
then a `relationship` with semantic type `radio.reception`. Its properties hold
frequency, band, mode, SNR, and source identifiers. `representation` is
`Observed Link`. The visual arc is presentation, not a measured flight path.

## Compatibility

The Go validator accepts unknown geometry names so they can be recorded and
forwarded. The bootstrap Unreal parser records the envelope but only renders
implemented geometry. Breaking semantic changes require a new schema version;
new properties and semantic types do not.

