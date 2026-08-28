# NASA source texture attribution

All files here are produced by `tools/fetch-earth-textures.py` (download
plus a lossless re-save; native resolution is kept, no downscaling). They
are imported as Streaming Virtual Textures, which tile arbitrary source
dimensions themselves, so there is no power-of-two requirement.

`bluemarble-21600.png` is NASA's Blue Marble Next Generation with topography
and bathymetry, July 2004, at NASA's largest single-file resolution:
21600x10800 (roughly 2 km/pixel at the equator). Credit: NASA Earth
Observatory; Blue Marble Next Generation data courtesy of Reto Stockli
(NASA/GSFC).

Source: https://eoimages.gsfc.nasa.gov/images/imagerecords/73000/73751/world.topo.bathy.200407.3x21600x10800.jpg

A true ~500 m/pixel version of this same July 2004 composite exists, as an
eight-tile 21600x21600-per-tile mosaic (86400x43200 assembled). This script
does not fetch it -- see the docstring in `tools/fetch-earth-textures.py`
for why (it needs the full ~11 GB canvas resident in memory to assemble,
which is unsafe on a shared build machine) and how a maintainer with more
memory to spare could extend the script to use it.

`earthatnight-13500.png` is NASA's Black Marble 2016 night lights mosaic,
at its native 3 km grid, 13500x6750 -- the highest-resolution single-file
release of this composite. Credit: NASA Earth Observatory / NOAA NCEI;
imagery by Joshua Stevens using Suomi NPP VIIRS data from Miguel Roman
(NASA/GSFC).

Source: https://eoimages.gsfc.nasa.gov/images/imagerecords/144000/144898/BlackMarble_2016_3km.jpg

`clouds-2048.png` is NASA's Blue Marble cloud climatology (record 57747),
at its native and only published resolution, 2048x1024. Credit: NASA Earth
Observatory. This is the static, packaged fallback; at runtime
`AIonGlobeActor` replaces it with the current EUMETSAT world IR composite
(see `docs/DATA-SOURCES.md`).

Source: https://eoimages.gsfc.nasa.gov/images/imagerecords/57000/57747/cloud_combined_2048.jpg

`starmap_2020_4k.exr` is NASA/GSFC Scientific Visualization Studio's *Deep
Star Maps 2020* (Gaia DR2: ESA/Gaia/DPAC; also Hipparcos-2, Tycho-2, UCAC3).
Public domain.

Source: https://svs.gsfc.nasa.gov/vis/a000000/a004800/a004851/starmap_2020_4k.exr
