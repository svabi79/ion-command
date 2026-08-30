# Visual design

The product is a spatial instrument, not a dashboard. At ultrawide resolution,
the globe owns roughly seventy percent of attention; controls occupy a curved
physical console below it, while status and contextual panels recede into the
scene.

## Bootstrap composition

- central 2,000-unit globe with UTC-driven directional sunlight;
- atmosphere and four conceptual ionosphere shells generated as translucent,
  edge-lit materials;
- northern and southern instanced aurora ovals with slow counter-motion;
- hundreds to thousands of luminous great-circle links;
- small instanced entity and observation markers;
- a three-part physical operator console with world-space telemetry;
- orbital camera with right-mouse orbit and wheel zoom.

The scene uses black-blue space, cyan structure, green live state, amber
degradation, band-specific arc colors, and selective white highlights. Surfaces
should have depth, glow, and reflection. Rectangular screen-space cards are not
part of the visual language.

`Conceptual Layer Visualisation` must accompany ionosphere shells. The analysis
language distinguishes Observed Link, Visual Arc, and Modelled Path.

## Asset strategy

First Light uses engine primitives and generated materials. External paid assets
are forbidden. Editor scripts preserve existing assets and create missing
materials, data assets, and the level repeatably. Final Earth albedo, night
lights, clouds, star field, panel typography, and audio are a separate art pass.

## Arc response to zoom

Arcs are additive and there are thousands of them. From the default orbit
they overlap into the weave the globe is known for; at close range the same
weave saturates to white and hides the Earth entirely - with a full live
load, the closest approach used to render as a blank white frame.

Two things scale with camera altitude, both exactly 1.0 at the default orbit
so the far view is unchanged:

| | At default orbit | At closest approach | Floor |
| --- | --- | --- | --- |
| `ZoomThickness` | 1.0 | ~0.002 | 0.012 |
| `ZoomDim` | 1.0 | ~0.0 | 0.05 |

**Thickness** exists because `ArcThickness` is a world size - 0.02 on a
100-unit cube, about 12.7 km. That is a hairline from orbit and a third of
the screen at 43 km across, which is why arcs appeared to fatten as the
camera descended. The segment is squeezed perpendicular to its own axis in
the vertex shader rather than by rewriting instance transforms: there are
tens of thousands of them, and this costs nothing per frame.

**Brightness** falls faster than linearly (exponent 1.6) because additive
arcs stack. Halving the distance has to do more than halve the light, or the
overlap wins.

The floors keep paths visible rather than making them vanish; `V` still
hides them outright.
