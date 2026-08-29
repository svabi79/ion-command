#include "GeoTileMosaic.h"

#include "Engine/Texture2D.h"
#include "HttpModule.h"
#include "IImageWrapper.h"
#include "IImageWrapperModule.h"
#include "Interfaces/IHttpRequest.h"
#include "Interfaces/IHttpResponse.h"
#include "Misc/FileHelper.h"
#include "Misc/Paths.h"
#include "Modules/ModuleManager.h"

void UGeoTileMosaic::Configure(const FString& InBaseUrl, const TArray<FGeoTileLayer>& InLayers)
{
    BaseUrl = InBaseUrl;
    Layers = InLayers;
    // Shallow layers composite first so the sharp, sparse ones paint over
    // them, whatever order the caller listed them in.
    Layers.Sort([](const FGeoTileLayer& A, const FGeoTileLayer& B) { return A.MaxLevel < B.MaxLevel; });
}

int32 UGeoTileMosaic::ChooseLevel(double SpanDegrees, int32 ScreenHeightPixels, int32 MaxLevel) const
{
    if (SpanDegrees <= 0.0 || ScreenHeightPixels <= 0)
    {
        return 0;
    }
    // Stop at the first level whose pixels are finer than the screen can
    // resolve: deeper costs bandwidth and shows nothing more.
    const double DegreesPerScreenPixel = SpanDegrees / static_cast<double>(ScreenHeightPixels);
    for (int32 Candidate = 0; Candidate <= MaxLevel; ++Candidate)
    {
        if (LevelSpanDegrees(Candidate) / static_cast<double>(TilePixels) <= DegreesPerScreenPixel)
        {
            return Candidate;
        }
    }
    return MaxLevel;
}

bool UGeoTileMosaic::Update(double CenterLatitude, double CenterLongitude, double SpanDegrees, int32 ScreenHeightPixels)
{
    if (Layers.Num() == 0 || BaseUrl.IsEmpty())
    {
        return false;
    }
    const int32 DeepestAvailable = Layers.Last().MaxLevel;
    const int32 Level = ChooseLevel(SpanDegrees, ScreenHeightPixels, DeepestAvailable);
    const double Span = LevelSpanDegrees(Level);

    const int32 CenterCol = FMath::FloorToInt32((CenterLongitude + 180.0) / Span);
    const int32 CenterRow = FMath::FloorToInt32((90.0 - CenterLatitude) / Span);
    const int32 MatrixHeight = FMath::CeilToInt32(180.0 / Span);
    const int32 ColMin = CenterCol - TilesPerSide / 2;
    // Clamp north-south: the pyramid has no rows past the poles, and a
    // wrapped row would quietly show the wrong hemisphere.
    const int32 RowMin = FMath::Clamp(CenterRow - TilesPerSide / 2, 0, FMath::Max(0, MatrixHeight - TilesPerSide));

    if (Level == CurrentLevel && ColMin == CurrentColMin && RowMin == CurrentRowMin)
    {
        return false;
    }
    BeginRegion(Level, ColMin, RowMin);
    return true;
}

void UGeoTileMosaic::BeginRegion(int32 Level, int32 ColMin, int32 RowMin)
{
    CurrentLevel = Level;
    CurrentColMin = ColMin;
    CurrentRowMin = RowMin;
    ++RegionSerial;

    const double Span = LevelSpanDegrees(Level);
    // Bounds in the globe's UV space. u = (lon+180)/360 and v = (90-lat)/180
    // are exactly the pyramid's own axes, so the window is an axis-aligned
    // rectangle and no tile is ever resampled to fit it.
    const double LonMin = ColMin * Span - 180.0;
    const double LatMax = 90.0 - RowMin * Span;
    const double WindowSpan = Span * TilesPerSide;
    BoundsUV = FLinearColor(
        static_cast<float>((LonMin + 180.0) / 360.0),
        static_cast<float>((90.0 - LatMax) / 180.0),
        static_cast<float>(WindowSpan / 360.0),
        static_cast<float>(WindowSpan / 180.0));

    EnsureTexture();
    // Keep the previous region's pixels until new ones land: clearing here
    // would flash a black window across the globe on every level change.
    Coverage = 0.0f;
    PendingTiles = 0;
    ExpectedTiles = 0;

    // Collect the work first, then issue it, so ExpectedTiles is final
    // before any response can come back and divide by it.
    struct FTileFetch
    {
        FGeoTileLayer Layer;
        int32 Level;
        int32 Col;
        int32 Row;
        FIntRect Dest;
        FIntRect Source;
    };
    TArray<FTileFetch> Fetches;

    for (const FGeoTileLayer& Layer : Layers)
    {
        // A layer shallower than the requested level still contributes: its
        // coarsest covering tiles fill the window, and a sparse sharper
        // layer paints over wherever it actually observed something.
        const int32 LayerLevel = FMath::Min(Level, Layer.MaxLevel);
        const int32 Shift = Level - LayerLevel;
        const int32 Ratio = 1 << Shift;
        const int32 LayerMatrixWidth = FMath::CeilToInt32(360.0 / LevelSpanDegrees(LayerLevel));
        const int32 LayerMatrixHeight = FMath::CeilToInt32(180.0 / LevelSpanDegrees(LayerLevel));

        // Iterate the distinct source tiles, not the mosaic slots: one
        // coarse tile can cover the whole window, and fetching it once per
        // slot would be sixteen identical requests.
        const int32 SrcColMin = FMath::FloorToInt32(static_cast<float>(ColMin) / Ratio);
        const int32 SrcColMax = FMath::FloorToInt32(static_cast<float>(ColMin + TilesPerSide - 1) / Ratio);
        const int32 SrcRowMin = FMath::FloorToInt32(static_cast<float>(RowMin) / Ratio);
        const int32 SrcRowMax = FMath::FloorToInt32(static_cast<float>(RowMin + TilesPerSide - 1) / Ratio);

        for (int32 SrcRow = SrcRowMin; SrcRow <= SrcRowMax; ++SrcRow)
        {
            for (int32 SrcCol = SrcColMin; SrcCol <= SrcColMax; ++SrcCol)
            {
                if (SrcCol < 0 || SrcRow < 0 || SrcCol >= LayerMatrixWidth || SrcRow >= LayerMatrixHeight)
                {
                    continue;
                }
                // Which target tiles does this source tile cover, and which
                // of those fall inside the window?
                const int32 FirstCol = FMath::Max(SrcCol * Ratio, ColMin);
                const int32 LastCol = FMath::Min((SrcCol + 1) * Ratio, ColMin + TilesPerSide);
                const int32 FirstRow = FMath::Max(SrcRow * Ratio, RowMin);
                const int32 LastRow = FMath::Min((SrcRow + 1) * Ratio, RowMin + TilesPerSide);
                if (FirstCol >= LastCol || FirstRow >= LastRow)
                {
                    continue;
                }
                FTileFetch Fetch;
                Fetch.Layer = Layer;
                Fetch.Level = LayerLevel;
                Fetch.Col = SrcCol;
                Fetch.Row = SrcRow;
                Fetch.Dest = FIntRect(
                    (FirstCol - ColMin) * TilePixels, (FirstRow - RowMin) * TilePixels,
                    (LastCol - ColMin) * TilePixels, (LastRow - RowMin) * TilePixels);
                // The matching sub-rectangle of the source tile. At Ratio 1
                // this is the whole tile; deeper down it is the fraction the
                // window actually sits on, which is what makes a coarse
                // underlay line up with the sharp layer above it instead of
                // being stretched across the whole window.
                const int32 SourcePixelsPerTargetTile = FMath::Max(1, TilePixels / Ratio);
                Fetch.Source = FIntRect(
                    (FirstCol - SrcCol * Ratio) * SourcePixelsPerTargetTile,
                    (FirstRow - SrcRow * Ratio) * SourcePixelsPerTargetTile,
                    (LastCol - SrcCol * Ratio) * SourcePixelsPerTargetTile,
                    (LastRow - SrcRow * Ratio) * SourcePixelsPerTargetTile);
                Fetches.Add(MoveTemp(Fetch));
            }
        }
    }

    ExpectedTiles = Fetches.Num();
    PendingTiles = ExpectedTiles;
    if (ExpectedTiles == 0)
    {
        Coverage = 1.0f;
        return;
    }
    for (const FTileFetch& Fetch : Fetches)
    {
        RequestTile(Fetch.Layer, Fetch.Level, Fetch.Col, Fetch.Row, Fetch.Dest, Fetch.Source);
    }
}

FString UGeoTileMosaic::CacheFilePath(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row) const
{
    return FPaths::ProjectSavedDir() / TEXT("TileCache") / Layer.Identifier / Layer.Time /
        FString::Printf(TEXT("%d_%d_%d.%s"), Level, Row, Col, *Layer.Extension);
}

void UGeoTileMosaic::TileResolved()
{
    PendingTiles = FMath::Max(0, PendingTiles - 1);
    Coverage = ExpectedTiles > 0
        ? 1.0f - static_cast<float>(PendingTiles) / static_cast<float>(ExpectedTiles)
        : 1.0f;
}

void UGeoTileMosaic::RequestTile(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row,
                                 const FIntRect& Dest, const FIntRect& Source)
{
    const uint32 Serial = RegionSerial;

    // Disk cache first: the operator returns to the same places, and a
    // restart should not re-fetch what is already here.
    const FString CachePath = CacheFilePath(Layer, Level, Col, Row);
    TArray<uint8> Cached;
    if (FFileHelper::LoadFileToArray(Cached, *CachePath, FILEREAD_Silent) && Cached.Num() > 0)
    {
        CompositeTile(Cached, Layer, Dest, Source);
        TileResolved();
        return;
    }

    const FString Url = FString::Printf(TEXT("%s/%s/default/%s/%s/%d/%d/%d.%s"),
        *BaseUrl, *Layer.Identifier, *Layer.Time, *Layer.MatrixSet, Level, Row, Col, *Layer.Extension);

    const TSharedRef<IHttpRequest, ESPMode::ThreadSafe> Request = FHttpModule::Get().CreateRequest();
    Request->SetURL(Url);
    Request->SetVerb(TEXT("GET"));
    Request->SetTimeout(45.0f);
    TWeakObjectPtr<UGeoTileMosaic> WeakThis(this);
    const FGeoTileLayer LayerCopy = Layer;
    Request->OnProcessRequestComplete().BindLambda(
        [WeakThis, LayerCopy, Dest, Source, Serial, CachePath](FHttpRequestPtr, FHttpResponsePtr Response, bool bConnected)
        {
            if (!WeakThis.IsValid())
            {
                return;
            }
            UGeoTileMosaic* Self = WeakThis.Get();
            if (bConnected && Response.IsValid() && Response->GetResponseCode() == 200)
            {
                const TArray<uint8>& Bytes = Response->GetContent();
                FFileHelper::SaveArrayToFile(Bytes, *CachePath);
                // Drop tiles from a region the camera has already left:
                // compositing them would smear the old place over the new.
                if (Serial == Self->RegionSerial)
                {
                    Self->CompositeTile(Bytes, LayerCopy, Dest, Source);
                }
            }
            // A missing tile is ordinary, not a failure: sparse layers only
            // cover what was observed, and the layer beneath stays visible.
            if (Serial == Self->RegionSerial)
            {
                Self->TileResolved();
            }
        });
    Request->ProcessRequest();
}

void UGeoTileMosaic::EnsureTexture()
{
    if (Pixels.Num() != TextureSize * TextureSize)
    {
        Pixels.SetNumZeroed(TextureSize * TextureSize);
    }
    if (Texture)
    {
        return;
    }
    Texture = UTexture2D::CreateTransient(TextureSize, TextureSize, PF_B8G8R8A8);
    if (!Texture)
    {
        return;
    }
    // Colour imagery, blended against the sRGB global textures.
    Texture->SRGB = true;
    Texture->AddressX = TA_Clamp;
    Texture->AddressY = TA_Clamp;
    Texture->Filter = TF_Trilinear;
    Texture->UpdateResource();
}

void UGeoTileMosaic::CompositeTile(const TArray<uint8>& Bytes, const FGeoTileLayer& Layer,
                                   const FIntRect& Dest, const FIntRect& Source)
{
    if (Bytes.Num() == 0)
    {
        return;
    }
    IImageWrapperModule& Wrappers = FModuleManager::LoadModuleChecked<IImageWrapperModule>(FName("ImageWrapper"));
    const EImageFormat Format = Layer.Extension == TEXT("png") ? EImageFormat::PNG : EImageFormat::JPEG;
    const TSharedPtr<IImageWrapper> Wrapper = Wrappers.CreateImageWrapper(Format);
    if (!Wrapper.IsValid() || !Wrapper->SetCompressed(Bytes.GetData(), Bytes.Num()))
    {
        return;
    }
    TArray64<uint8> Raw;
    if (!Wrapper->GetRaw(ERGBFormat::BGRA, 8, Raw))
    {
        return;
    }
    const int32 SrcWidth = Wrapper->GetWidth();
    const int32 SrcHeight = Wrapper->GetHeight();
    if (SrcWidth <= 0 || SrcHeight <= 0)
    {
        return;
    }
    EnsureTexture();

    const int32 DestWidth = Dest.Width();
    const int32 DestHeight = Dest.Height();
    const int32 SourceWidth = FMath::Max(1, Source.Width());
    const int32 SourceHeight = FMath::Max(1, Source.Height());
    // The decoded tile may not be TilePixels across if the service ever
    // serves a different size, so map through the tile's own dimensions.
    const double SourceScaleX = static_cast<double>(SrcWidth) / static_cast<double>(TilePixels);
    const double SourceScaleY = static_cast<double>(SrcHeight) / static_cast<double>(TilePixels);

    for (int32 Y = 0; Y < DestHeight; ++Y)
    {
        const int32 DestY = Dest.Min.Y + Y;
        if (DestY < 0 || DestY >= TextureSize)
        {
            continue;
        }
        const double SourceY = (Source.Min.Y + (static_cast<double>(Y) * SourceHeight) / DestHeight) * SourceScaleY;
        const int32 SrcY = FMath::Clamp(static_cast<int32>(SourceY), 0, SrcHeight - 1);
        for (int32 X = 0; X < DestWidth; ++X)
        {
            const int32 DestX = Dest.Min.X + X;
            if (DestX < 0 || DestX >= TextureSize)
            {
                continue;
            }
            const double SourceX = (Source.Min.X + (static_cast<double>(X) * SourceWidth) / DestWidth) * SourceScaleX;
            const int32 SrcX = FMath::Clamp(static_cast<int32>(SourceX), 0, SrcWidth - 1);
            const int64 SrcIndex = (static_cast<int64>(SrcY) * SrcWidth + SrcX) * 4;
            // Where a sparse layer observed nothing, leave the coarser layer
            // underneath standing rather than punching a hole in the globe.
            if (Layer.bMayBeSparse && Raw[SrcIndex + 3] < 8)
            {
                continue;
            }
            Pixels[static_cast<int64>(DestY) * TextureSize + DestX] =
                FColor(Raw[SrcIndex + 2], Raw[SrcIndex + 1], Raw[SrcIndex + 0], 255);
        }
    }
    bDirty = true;
    PushToGpu();
}

void UGeoTileMosaic::PushToGpu()
{
    if (!bDirty || !Texture || !Texture->GetPlatformData() || Texture->GetPlatformData()->Mips.Num() == 0)
    {
        return;
    }
    FTexture2DMipMap& Mip = Texture->GetPlatformData()->Mips[0];
    void* Data = Mip.BulkData.Lock(LOCK_READ_WRITE);
    FMemory::Memcpy(Data, Pixels.GetData(), Pixels.Num() * sizeof(FColor));
    Mip.BulkData.Unlock();
    Texture->UpdateResource();
    bDirty = false;
}
