#pragma once

#include "CoreMinimal.h"
#include "UObject/Object.h"
#include "GeoTileMosaic.generated.h"

class UTexture2D;

// How a tile service numbers its tiles.
UENUM()
enum class EGeoTileLayout : uint8
{
    // OGC WMTS geographic (EPSG:4326), from the top-left corner (-180, 90),
    // every level halving the previous span. Latitude is linear within a
    // tile. The root span and tile size differ between services - GIBS uses
    // 288 degrees over 512 pixels, EOX 180 over 256 - so both are per-layer
    // rather than baked in here.
    Geographic,
    // XYZ web mercator (EPSG:3857), the {z}/{x}/{y} scheme. 2^z tiles each
    // way; latitude is non-linear within a tile and the poles beyond
    // +-85.05 degrees do not exist.
    WebMercator,
};

// What the tile's pixels mean.
UENUM()
enum class EGeoTileEncoding : uint8
{
    // Ordinary colour, composited as-is.
    Colour,
    // Terrarium elevation: metres = (R * 256 + G + B / 256) - 32768,
    // rescaled into the mosaic's single 16-bit channel.
    TerrariumElevation,
};

// One layer of a tile pyramid the mosaic can draw from.
USTRUCT()
struct FGeoTileLayer
{
    GENERATED_BODY()

    // Full URL template with {z}, {x}, {y} placeholders. Keeping the whole
    // template here rather than assembling it from parts is what lets one
    // mosaic serve both a WMTS REST endpoint and a plain XYZ bucket.
    UPROPERTY() FString UrlTemplate;
    // Short name for the on-disk cache directory.
    UPROPERTY() FString CacheName;
    // File extension of the tile images ("jpeg", "png").
    UPROPERTY() FString Extension;
    // Deepest zoom level the layer offers. Requesting beyond it is an error
    // from the service, not a blurrier tile.
    UPROPERTY() int32 MaxLevel = 0;
    UPROPERTY() EGeoTileLayout Layout = EGeoTileLayout::Geographic;
    UPROPERTY() EGeoTileEncoding Encoding = EGeoTileEncoding::Colour;
    // Edge length of the service's own tiles. WMTS commonly serves 512, the
    // XYZ convention 256, and the difference matters: the same level number
    // means different ground resolutions, so levels are never compared
    // across layers - only resolutions are.
    UPROPERTY() int32 NativeTilePixels = 512;
    // Degrees one tile spans at level 0, for Geographic layers. GIBS uses
    // 288 (two tiles covering 360 with overlap at the seam); the OGC WGS84
    // convention EOX follows uses 180 (exactly two tiles). Ignored for
    // WebMercator, which is always 360 over 2^z.
    UPROPERTY() double RootSpanDegrees = 288.0;
    // Tiles that may be partly or wholly empty where the source has no
    // observation; those pixels must not overwrite whatever is already in
    // the mosaic beneath them.
    UPROPERTY() bool bMayBeSparse = false;
};

// A window of map data that follows the camera: the visible region is
// resolved to whole tiles, fetched, and assembled into one texture the globe
// material samples inside the region's bounds.
//
// This exists because no single global texture can serve a close orbit. At
// the closest approach the camera sees ~43 km across, which needs ~40 m per
// pixel; a global texture at that density would be a million pixels wide.
// A window only has to cover what is on screen, so the same texture budget
// buys whatever resolution the current altitude actually calls for.
//
// The window itself is always equirectangular - the same convention the
// globe mesh's UVs use - so the material needs no projection maths. Tiles
// from a web mercator source are resampled into it on the way in, which is
// only a per-row remapping because longitude stays linear in both layouts.
UCLASS()
class IONCOMMANDVISUALIZATION_API UGeoTileMosaic final : public UObject
{
    GENERATED_BODY()

public:
    // Tiles across and down the window. Wider than tall because screens are:
    // a square window that just covers the visible height falls short of the
    // width on 16:9 and badly short on 21:9 or 32:9, and the shortfall shows
    // as a blurred band down the side of the frame where the window ends.
    static constexpr int32 TilesX = 8;
    static constexpr int32 TilesY = 4;
    static constexpr int32 TilePixels = 512;
    static constexpr int32 TextureWidth = TilesX * TilePixels;
    static constexpr int32 TextureHeight = TilesY * TilePixels;

    // How much disk the tile cache may occupy before the oldest tiles are
    // dropped. Roughly a thousand tiles at typical sizes, which covers a lot
    // of revisiting without growing without limit.
    static constexpr int64 DefaultCacheBudgetBytes = 512LL * 1024 * 1024;

    // Elevation range the 16-bit channel is scaled across. Earth's land runs
    // from the Dead Sea shore (-430 m) to Everest (8849 m); the margins keep
    // bathymetry near the coast and any future outlier inside the range.
    static constexpr double ElevationFloorMetres = -1000.0;
    static constexpr double ElevationCeilingMetres = 9500.0;

    void Configure(const TArray<FGeoTileLayer>& InLayers);

    // Delete the oldest cached tiles until the cache is under the budget.
    // Call once at startup: the cache only grows while the client runs, so
    // trimming on the way in bounds it without touching the hot path.
    // Returns the number of bytes reclaimed.
    static int64 TrimCache(int64 BudgetBytes);

    // Re-aim the window. Center is degrees; SpanDegrees is the north-south
    // extent the camera can see and SpanLongitudeDegrees the east-west one,
    // which is the larger of the two on any normal display and is what
    // decides whether the window actually covers the frame. Returns true if
    // this started a new region; panning inside the current one is free.
    bool Update(double CenterLatitude, double CenterLongitude, double SpanDegrees,
                double SpanLongitudeDegrees, int32 ScreenHeightPixels);

    UTexture2D* GetTexture() const { return Texture; }

    // Region bounds as (uMin, vMin, uSpan, vSpan) in the globe's
    // equirectangular UV space. Zero span means "no region".
    FLinearColor GetBoundsUV() const { return BoundsUV; }

    // 0 while the region is still filling, 1 once every tile has resolved.
    float GetCoverage() const { return Coverage; }

    int32 GetLevel() const { return CurrentLevel; }

    // Ground sample distance of the window in metres, for a material that
    // needs to turn height differences into a slope. Rows and columns are
    // the same size on the ground because the window is square in degrees
    // per texel, not in count.
    double GetMetresPerPixel() const;

private:
    // Degrees spanned by one tile edge at a level, for the window's own
    // grid. The window is laid out on the GIBS geometry whatever the layers
    // use; layers are then resolved against it by ground resolution.
    static double GeographicTileSpan(int32 Level) { return 288.0 / static_cast<double>(1 << Level); }
    // The same, for a specific layer's grid.
    static double LayerTileSpan(const FGeoTileLayer& Layer, int32 Level)
    {
        return Layer.RootSpanDegrees / static_cast<double>(1 << Level);
    }
    // Fractional tile row for a latitude, in whichever layout. This is the
    // only place the two projections differ.
    static double TileRowForLatitude(const FGeoTileLayer& Layer, int32 Level, double Latitude);
    static double TileColumnForLongitude(const FGeoTileLayer& Layer, int32 Level, double Longitude);
    // Inverse of TileRowForLatitude, for working out which latitudes a tile
    // covers.
    static double LatitudeForTileRow(const FGeoTileLayer& Layer, int32 Level, double Row);

    // Degrees per pixel a layer delivers at one of its own levels, and the
    // shallowest level of that layer that is at least as fine as a target.
    static double LayerDegreesPerPixel(const FGeoTileLayer& Layer, int32 Level);
    static int32 LayerLevelForResolution(const FGeoTileLayer& Layer, double TargetDegreesPerPixel);
    int32 ChooseLevel(double SpanDegrees, double SpanLongitudeDegrees, int32 ScreenHeightPixels) const;
    void BeginRegion(int32 Level, int32 ColMin, int32 RowMin);
    void RequestTile(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row);
    // Paints the tile into whichever part of the window it covers, mapping
    // through latitude/longitude so a mercator tile lands in the right rows.
    void CompositeTile(const TArray<uint8>& Bytes, const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row);
    void TileResolved();
    void EnsureTexture();
    void PushToGpu();
    FString CacheFilePath(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row) const;

    UPROPERTY() TArray<FGeoTileLayer> Layers;
    UPROPERTY(Transient) TObjectPtr<UTexture2D> Texture;

    // CPU-side window; tiles composite here before one upload to the GPU.
    // Colour mosaics use Pixels, elevation mosaics use Heights.
    TArray<FColor> Pixels;
    TArray<uint16> Heights;
    bool bElevation = false;

    // Geographic bounds of the window, which is always equirectangular
    // whatever layout the tiles came in.
    double WindowLonMin = 0.0;
    double WindowLatMax = 0.0;
    double WindowSpanDegrees = 0.0;
    double WindowSpanLongitude = 0.0;

    FLinearColor BoundsUV = FLinearColor(0, 0, 0, 0);
    float Coverage = 0.0f;
    int32 CurrentLevel = -1;
    int32 CurrentColMin = -1;
    int32 CurrentRowMin = -1;
    int32 PendingTiles = 0;
    int32 ExpectedTiles = 0;
    // Rising counter so tiles from an abandoned region cannot composite into
    // the one that replaced it - the camera moves faster than HTTP.
    uint32 RegionSerial = 0;
    bool bDirty = false;
};
