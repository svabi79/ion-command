#pragma once

#include "CoreMinimal.h"
#include "UObject/Object.h"
#include "GeoTileMosaic.generated.h"

class UTexture2D;

// One layer of a WMTS tile pyramid, as a source the mosaic can draw from.
// Levels share the pyramid geometry below; a layer only declares how deep it
// goes and how to address it.
USTRUCT()
struct FGeoTileLayer
{
    GENERATED_BODY()

    // Layer identifier as the service names it.
    UPROPERTY() FString Identifier;
    // Tile matrix set the layer is published in.
    UPROPERTY() FString MatrixSet;
    // File extension of the tile images ("jpeg", "png").
    UPROPERTY() FString Extension;
    // Deepest zoom level the layer offers. Requesting beyond it is an error
    // from the service, not a blurrier tile.
    UPROPERTY() int32 MaxLevel = 0;
    // Time dimension value, or "default" for layers without one.
    UPROPERTY() FString Time = TEXT("default");
    // Tiles that carry an alpha channel may be partly or wholly empty where
    // the source has no observation; those pixels must not overwrite the
    // coarser layer already in the mosaic.
    UPROPERTY() bool bMayBeSparse = false;
};

// A window of map imagery that follows the camera: the visible region is
// resolved to whole tiles of a WMTS pyramid, fetched, and assembled into one
// texture the globe material samples inside the region's bounds.
//
// This exists because no single global texture can serve a close orbit. At
// the closest approach the camera sees ~43 km across, which needs ~40 m per
// pixel; a global texture at that density would be a million pixels wide.
// A window only has to cover what is on screen, so the same texture budget
// buys whatever resolution the current altitude actually calls for.
//
// The pyramid is the OGC WMTS geographic (EPSG:4326) layout: level 0 is two
// 512-pixel tiles spanning 288 degrees each from the top-left corner
// (-180, 90), and every level halves that span. That is the same
// equirectangular convention the globe mesh's UVs use, so a tile is an
// axis-aligned rectangle in UV space and drops into the mosaic without
// resampling.
UCLASS()
class IONCOMMANDVISUALIZATION_API UGeoTileMosaic final : public UObject
{
    GENERATED_BODY()

public:
    // Tiles across and down the mosaic. Four covers the visible span with
    // room to pan before a refill is needed, at 16 requests per region.
    static constexpr int32 TilesPerSide = 4;
    static constexpr int32 TilePixels = 512;
    static constexpr int32 TextureSize = TilesPerSide * TilePixels;

    // Base of the WMTS REST endpoint, up to and including the projection and
    // the "best" (or equivalent) collection segment.
    void Configure(const FString& InBaseUrl, const TArray<FGeoTileLayer>& InLayers);

    // Re-aim the window. Center is degrees, SpanDegrees is the north-south
    // extent the camera can see. Returns true if this started a new region;
    // panning inside the current region is free.
    bool Update(double CenterLatitude, double CenterLongitude, double SpanDegrees, int32 ScreenHeightPixels);

    // Texture holding the assembled region, or null before the first tile
    // has landed.
    UTexture2D* GetTexture() const { return Texture; }

    // Region bounds as (uMin, vMin, uSpan, vSpan) in the globe's
    // equirectangular UV space, for the material to map a pixel into the
    // mosaic. Zero span means "no region", i.e. sample the global texture.
    FLinearColor GetBoundsUV() const { return BoundsUV; }

    // 0 while the region is still filling, 1 once every tile has resolved.
    // The material fades the window in on this so a half-filled mosaic never
    // shows as holes in the Earth.
    float GetCoverage() const { return Coverage; }

    // Deepest level currently requested, for diagnostics.
    int32 GetLevel() const { return CurrentLevel; }

private:
    // Pixels per tile edge in degrees at a given level.
    static double LevelSpanDegrees(int32 Level) { return 288.0 / static_cast<double>(1 << Level); }
    // Deepest level whose pixels are still coarser than the screen needs, so
    // the mosaic never fetches detail the display cannot show.
    int32 ChooseLevel(double SpanDegrees, int32 ScreenHeightPixels, int32 MaxLevel) const;
    void BeginRegion(int32 Level, int32 ColMin, int32 RowMin);
    // Dest is the rectangle of the mosaic this tile fills; Source is the
    // sub-rectangle of the tile that belongs there, which is the whole tile
    // except when a shallow layer is covering a deeper level.
    void RequestTile(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row,
                     const FIntRect& Dest, const FIntRect& Source);
    void CompositeTile(const TArray<uint8>& Bytes, const FGeoTileLayer& Layer,
                       const FIntRect& Dest, const FIntRect& Source);
    void TileResolved();
    void EnsureTexture();
    void PushToGpu();
    FString CacheFilePath(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row) const;

    FString BaseUrl;
    UPROPERTY() TArray<FGeoTileLayer> Layers;
    UPROPERTY(Transient) TObjectPtr<UTexture2D> Texture;

    // CPU-side mosaic; tiles composite here before one upload to the GPU.
    TArray<FColor> Pixels;
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
