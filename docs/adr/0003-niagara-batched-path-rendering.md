# ADR 0003: Batched path rendering

Status: accepted; bootstrap adapter uses HISM pending Niagara asset generation.

An event never owns an Actor, spline, widget, Niagara component, or dynamic
material. Layers submit compact render items to shared render adapters. First
Light uses shared hierarchical instanced meshes so it can be built from source;
the production implementation will use Niagara Data Channels or a custom GPU
buffer behind the same interface. Only a selected item may receive a rich Actor.

