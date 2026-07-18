# ADR 0002: Canonical geospatial event contract

Status: accepted.

All domains map into a versioned envelope containing message kind, hierarchical
semantic type, source identity, explicit time semantics, WGS84 geometry,
properties, quality, and relationships. The core does not enumerate every
domain concept. Unknown types remain forwardable. JSON is used first; transport
encoding may change without changing semantics.

