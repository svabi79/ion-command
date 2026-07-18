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

