#include "GeoTileMosaic.h"

#include "Engine/Texture2D.h"
#include "HttpModule.h"
#include "IImageWrapper.h"
#include "IImageWrapperModule.h"
#include "Interfaces/IHttpRequest.h"
#include "Interfaces/IHttpResponse.h"
#include "HAL/FileManager.h"
#include "Misc/FileHelper.h"
#include "Misc/Paths.h"
#include "Modules/ModuleManager.h"

namespace
{
    // Web mercator is undefined at the poles and every implementation clips
    // at the latitude that makes the world square.
    constexpr double MercatorLatitudeLimit = 85.05112877980659;

    // Terrarium packs metres as (R*256 + G + B/256) - 32768. Raw is BGRA.
    double DecodeTerrariumMetres(const TArray64<uint8>& Raw, int32 Width, int32 Height, int32 X, int32 Y)
    {
        const int32 ClampedX = FMath::Clamp(X, 0, Width - 1);
        const int32 ClampedY = FMath::Clamp(Y, 0, Height - 1);
        const int64 Index = (static_cast<int64>(ClampedY) * Width + ClampedX) * 4;
        return (Raw[Index + 2] * 256.0 + Raw[Index + 1] + Raw[Index + 0] / 256.0) - 32768.0;
    }
}

void UGeoTileMosaic::Configure(const TArray<FGeoTileLayer>& InLayers)
{
    Layers = InLayers;
    // Shallow layers composite first so sharp, sparse ones paint over them,
    // whatever order the caller listed them in.
    Layers.Sort([](const FGeoTileLayer& A, const FGeoTileLayer& B) { return A.MaxLevel < B.MaxLevel; });
    bElevation = Layers.ContainsByPredicate(
        [](const FGeoTileLayer& L) { return L.Encoding == EGeoTileEncoding::TerrariumElevation; });
}

double UGeoTileMosaic::TileColumnForLongitude(const FGeoTileLayer& Layer, int32 Level, double Longitude)
{
    if (Layer.Layout == EGeoTileLayout::WebMercator)
    {
        return (Longitude + 180.0) / 360.0 * static_cast<double>(1 << Level);
    }
    return (Longitude + 180.0) / GeographicTileSpan(Level);
}

double UGeoTileMosaic::TileRowForLatitude(const FGeoTileLayer& Layer, int32 Level, double Latitude)
{
    if (Layer.Layout == EGeoTileLayout::WebMercator)
    {
        const double Clamped = FMath::Clamp(Latitude, -MercatorLatitudeLimit, MercatorLatitudeLimit);
        const double Radians = FMath::DegreesToRadians(Clamped);
        const double Projected = FMath::Loge(FMath::Tan(Radians) + 1.0 / FMath::Cos(Radians));
        return (1.0 - Projected / PI) / 2.0 * static_cast<double>(1 << Level);
    }
    return (90.0 - Latitude) / GeographicTileSpan(Level);
}

double UGeoTileMosaic::LatitudeForTileRow(const FGeoTileLayer& Layer, int32 Level, double Row)
{
    if (Layer.Layout == EGeoTileLayout::WebMercator)
    {
        const double Projected = PI * (1.0 - 2.0 * Row / static_cast<double>(1 << Level));
        return FMath::RadiansToDegrees(FMath::Atan(FMath::Sinh(Projected)));
    }
    return 90.0 - Row * GeographicTileSpan(Level);
}

double UGeoTileMosaic::GetMetresPerPixel() const
{
    if (WindowSpanDegrees <= 0.0)
    {
        return 0.0;
    }
    // 111319.49 m per degree of latitude, which is what the window's rows
    // are spaced in regardless of the source layout.
    return WindowSpanDegrees * 111319.49 / static_cast<double>(TextureHeight);
}

double UGeoTileMosaic::LayerDegreesPerPixel(const FGeoTileLayer& Layer, int32 Level)
{
    const double Pixels = static_cast<double>(FMath::Max(1, Layer.NativeTilePixels));
    if (Layer.Layout == EGeoTileLayout::WebMercator)
    {
        return 360.0 / static_cast<double>(1 << Level) / Pixels;
    }
    return GeographicTileSpan(Level) / Pixels;
}

int32 UGeoTileMosaic::LayerLevelForResolution(const FGeoTileLayer& Layer, double TargetDegreesPerPixel)
{
    for (int32 Candidate = 0; Candidate <= Layer.MaxLevel; ++Candidate)
    {
        if (LayerDegreesPerPixel(Layer, Candidate) <= TargetDegreesPerPixel)
        {
            return Candidate;
        }
    }
    return Layer.MaxLevel;
}

int32 UGeoTileMosaic::ChooseLevel(double SpanDegrees, double SpanLongitudeDegrees, int32 ScreenHeightPixels) const
{
    if (SpanDegrees <= 0.0 || ScreenHeightPixels <= 0)
    {
        return 0;
    }
    // Never resolve finer than the best layer can actually supply, or the
    // window would fetch coarse tiles and magnify them for nothing.
    double FinestAvailable = TNumericLimits<double>::Max();
    for (const FGeoTileLayer& Layer : Layers)
    {
        FinestAvailable = FMath::Min(FinestAvailable, LayerDegreesPerPixel(Layer, Layer.MaxLevel));
    }
    // Stop at the first level whose pixels are finer than the screen can
    // resolve: deeper costs bandwidth and shows nothing more.
    const double DegreesPerScreenPixel = SpanDegrees / static_cast<double>(ScreenHeightPixels);
    const double Target = FMath::Max(DegreesPerScreenPixel, FinestAvailable);
    int32 Level = 0;
    for (int32 Candidate = 0; Candidate <= 24; ++Candidate)
    {
        Level = Candidate;
        if (GeographicTileSpan(Candidate) / static_cast<double>(TilePixels) <= Target)
        {
            break;
        }
    }
    // Coverage beats sharpness: a window that does not reach the edge of the
    // frame leaves a visibly blurred band there, which is worse than the
    // whole frame being one level coarser. Back off until it fits both axes.
    while (Level > 0 &&
           (GeographicTileSpan(Level) * TilesY < SpanDegrees ||
            GeographicTileSpan(Level) * TilesX < SpanLongitudeDegrees))
    {
        --Level;
    }
    return Level;
}

bool UGeoTileMosaic::Update(double CenterLatitude, double CenterLongitude, double SpanDegrees,
                            double SpanLongitudeDegrees, int32 ScreenHeightPixels)
{
    if (Layers.Num() == 0)
    {
        return false;
    }
    // The window is laid out on the geographic grid whatever the layers use,
    // so one level and one tile origin describe it for all of them.
    const int32 Level = ChooseLevel(SpanDegrees, SpanLongitudeDegrees, ScreenHeightPixels);
    const double Span = GeographicTileSpan(Level);

    const int32 CenterCol = FMath::FloorToInt32((CenterLongitude + 180.0) / Span);
    const int32 CenterRow = FMath::FloorToInt32((90.0 - CenterLatitude) / Span);
    const int32 MatrixHeight = FMath::CeilToInt32(180.0 / Span);
    const int32 ColMin = CenterCol - TilesX / 2;
    // Clamp north-south: there are no rows past the poles, and a wrapped row
    // would quietly show the wrong hemisphere.
    const int32 RowMin = FMath::Clamp(CenterRow - TilesY / 2, 0, FMath::Max(0, MatrixHeight - TilesY));

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

    const double Span = GeographicTileSpan(Level);
    WindowLonMin = ColMin * Span - 180.0;
    WindowLatMax = 90.0 - RowMin * Span;
    WindowSpanDegrees = Span * TilesY;
    WindowSpanLongitude = Span * TilesX;

    // u = (lon+180)/360 and v = (90-lat)/180 are the globe mesh's own axes.
    BoundsUV = FLinearColor(
        static_cast<float>((WindowLonMin + 180.0) / 360.0),
        static_cast<float>((90.0 - WindowLatMax) / 180.0),
        static_cast<float>(WindowSpanLongitude / 360.0),
        static_cast<float>(WindowSpanDegrees / 180.0));

    EnsureTexture();
    // Keep the previous region's pixels until new ones land: clearing here
    // would flash a hole across the globe on every level change.
    Coverage = 0.0f;

    const double WindowLonMax = WindowLonMin + WindowSpanLongitude;
    const double WindowLatMin = WindowLatMax - WindowSpanDegrees;

    // Collect the work first so ExpectedTiles is final before any response
    // can come back and divide by it.
    struct FTileFetch
    {
        FGeoTileLayer Layer;
        int32 Level;
        int32 Col;
        int32 Row;
    };
    TArray<FTileFetch> Fetches;

    for (const FGeoTileLayer& Layer : Layers)
    {
        // A layer that cannot reach the window's resolution still
        // contributes: its finest covering tiles fill the window, and a
        // sparse sharper layer paints over wherever it observed something.
        // Resolution, not level number, is what the layers have in common.
        const int32 LayerLevel = LayerLevelForResolution(Layer, WindowSpanDegrees / static_cast<double>(TextureHeight));
        const int32 TilesAcross = Layer.Layout == EGeoTileLayout::WebMercator
            ? (1 << LayerLevel)
            : FMath::CeilToInt32(360.0 / GeographicTileSpan(LayerLevel));
        const int32 TilesDown = Layer.Layout == EGeoTileLayout::WebMercator
            ? (1 << LayerLevel)
            : FMath::CeilToInt32(180.0 / GeographicTileSpan(LayerLevel));

        // Which of this layer's tiles overlap the window? Ask in the layer's
        // own numbering rather than assuming it matches the window's.
        const int32 FirstCol = FMath::FloorToInt32(TileColumnForLongitude(Layer, LayerLevel, WindowLonMin));
        const int32 LastCol = FMath::FloorToInt32(TileColumnForLongitude(Layer, LayerLevel, FMath::Min(WindowLonMax, 179.999999)));
        // Rows count southward, so the window's north edge is the low row.
        const int32 FirstRow = FMath::FloorToInt32(TileRowForLatitude(Layer, LayerLevel, WindowLatMax));
        const int32 LastRow = FMath::FloorToInt32(TileRowForLatitude(Layer, LayerLevel, WindowLatMin));

        for (int32 Row = FirstRow; Row <= LastRow; ++Row)
        {
            for (int32 Col = FirstCol; Col <= LastCol; ++Col)
            {
                if (Col < 0 || Row < 0 || Col >= TilesAcross || Row >= TilesDown)
                {
                    continue;
                }
                Fetches.Add({Layer, LayerLevel, Col, Row});
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
        RequestTile(Fetch.Layer, Fetch.Level, Fetch.Col, Fetch.Row);
    }
}

int64 UGeoTileMosaic::TrimCache(int64 BudgetBytes)
{
    const FString Root = FPaths::ConvertRelativePathToFull(FPaths::ProjectSavedDir() / TEXT("TileCache"));
    IFileManager& Files = IFileManager::Get();
    if (!Files.DirectoryExists(*Root))
    {
        return 0;
    }

    struct FCachedTile
    {
        FString Path;
        FDateTime Age;
        int64 Size = 0;
    };
    TArray<FCachedTile> Tiles;
    int64 Total = 0;

    // Only files this class writes are candidates. The cache lives under
    // Saved/, which holds plenty that is not ours, and a deletion sweep that
    // trusts a directory path alone is one typo away from removing the wrong
    // tree - so match the shape of the names as well.
    TArray<FString> Found;
    Files.FindFilesRecursive(Found, *Root, TEXT("*.*"), true, false);
    for (const FString& Path : Found)
    {
        const FString Name = FPaths::GetCleanFilename(Path);
        const FString Extension = FPaths::GetExtension(Name);
        if (Extension != TEXT("png") && Extension != TEXT("jpeg") && Extension != TEXT("jpg"))
        {
            continue;
        }
        // "<level>_<row>_<col>.<ext>", all digits.
        TArray<FString> Parts;
        FPaths::GetBaseFilename(Name).ParseIntoArray(Parts, TEXT("_"), false);
        if (Parts.Num() != 3 || !Parts.ContainsByPredicate([](const FString& P) { return !P.IsEmpty(); }))
        {
            continue;
        }
        bool bNumeric = true;
        for (const FString& Part : Parts)
        {
            bNumeric &= !Part.IsEmpty() && Part.IsNumeric();
        }
        if (!bNumeric)
        {
            continue;
        }
        FCachedTile Tile;
        Tile.Path = Path;
        Tile.Size = Files.FileSize(*Path);
        Tile.Age = Files.GetTimeStamp(*Path);
        if (Tile.Size <= 0)
        {
            continue;
        }
        Total += Tile.Size;
        Tiles.Add(MoveTemp(Tile));
    }

    if (Total <= BudgetBytes)
    {
        return 0;
    }
    // Oldest first. Timestamps are good enough here: a tile fetched long ago
    // and never revisited is exactly what should go.
    Tiles.Sort([](const FCachedTile& A, const FCachedTile& B) { return A.Age < B.Age; });
    int64 Reclaimed = 0;
    for (const FCachedTile& Tile : Tiles)
    {
        if (Total - Reclaimed <= BudgetBytes)
        {
            break;
        }
        if (Files.Delete(*Tile.Path, false, false, true))
        {
            Reclaimed += Tile.Size;
        }
    }
    UE_LOG(LogTemp, Display, TEXT("ION COMMAND tile cache: %.1f MB over budget, reclaimed %.1f MB from %d files"),
        (Total - BudgetBytes) / 1048576.0, Reclaimed / 1048576.0, Tiles.Num());
    return Reclaimed;
}

FString UGeoTileMosaic::CacheFilePath(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row) const
{
    return FPaths::ProjectSavedDir() / TEXT("TileCache") / Layer.CacheName /
        FString::Printf(TEXT("%d_%d_%d.%s"), Level, Row, Col, *Layer.Extension);
}

void UGeoTileMosaic::TileResolved()
{
    PendingTiles = FMath::Max(0, PendingTiles - 1);
    Coverage = ExpectedTiles > 0
        ? 1.0f - static_cast<float>(PendingTiles) / static_cast<float>(ExpectedTiles)
        : 1.0f;
}

void UGeoTileMosaic::RequestTile(const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row)
{
    const uint32 Serial = RegionSerial;

    // Disk cache first: the operator returns to the same places, and a
    // restart should not re-fetch what is already here.
    const FString CachePath = CacheFilePath(Layer, Level, Col, Row);
    TArray<uint8> Cached;
    if (FFileHelper::LoadFileToArray(Cached, *CachePath, FILEREAD_Silent) && Cached.Num() > 0)
    {
        CompositeTile(Cached, Layer, Level, Col, Row);
        TileResolved();
        return;
    }

    FString Url = Layer.UrlTemplate;
    Url.ReplaceInline(TEXT("{z}"), *FString::FromInt(Level));
    Url.ReplaceInline(TEXT("{x}"), *FString::FromInt(Col));
    Url.ReplaceInline(TEXT("{y}"), *FString::FromInt(Row));

    const TSharedRef<IHttpRequest, ESPMode::ThreadSafe> Request = FHttpModule::Get().CreateRequest();
    Request->SetURL(Url);
    Request->SetVerb(TEXT("GET"));
    Request->SetTimeout(45.0f);
    TWeakObjectPtr<UGeoTileMosaic> WeakThis(this);
    const FGeoTileLayer LayerCopy = Layer;
    Request->OnProcessRequestComplete().BindLambda(
        [WeakThis, LayerCopy, Level, Col, Row, Serial, CachePath](FHttpRequestPtr, FHttpResponsePtr Response, bool bConnected)
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
                    Self->CompositeTile(Bytes, LayerCopy, Level, Col, Row);
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
    if (bElevation)
    {
        if (Heights.Num() != TextureWidth * TextureHeight)
        {
            Heights.SetNumZeroed(TextureWidth * TextureHeight);
        }
    }
    else if (Pixels.Num() != TextureWidth * TextureHeight)
    {
        Pixels.SetNumZeroed(TextureWidth * TextureHeight);
    }
    if (Texture)
    {
        return;
    }
    // 16 bits for elevation: 8 would quantise the range to 41 m steps, which
    // reads as terraces once the material takes a slope from it.
    Texture = UTexture2D::CreateTransient(TextureWidth, TextureHeight, bElevation ? PF_G16 : PF_B8G8R8A8);
    if (!Texture)
    {
        return;
    }
    Texture->SRGB = !bElevation;
    Texture->AddressX = TA_Clamp;
    Texture->AddressY = TA_Clamp;
    Texture->Filter = TF_Bilinear;
    Texture->UpdateResource();
}

void UGeoTileMosaic::CompositeTile(const TArray<uint8>& Bytes, const FGeoTileLayer& Layer, int32 Level, int32 Col, int32 Row)
{
    if (Bytes.Num() == 0 || WindowSpanDegrees <= 0.0)
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

    // Work in geography rather than in pixel rectangles: that is what lets a
    // mercator tile land on the right rows of an equirectangular window,
    // and it collapses the "coarse layer covering a deeper level" case into
    // the same code path - such a tile simply covers more of the window.
    const double TileLonMin = -180.0 + (Layer.Layout == EGeoTileLayout::WebMercator
        ? static_cast<double>(Col) / static_cast<double>(1 << Level) * 360.0
        : Col * GeographicTileSpan(Level));
    const double TileLonSpan = Layer.Layout == EGeoTileLayout::WebMercator
        ? 360.0 / static_cast<double>(1 << Level)
        : GeographicTileSpan(Level);
    const double TileLatTop = LatitudeForTileRow(Layer, Level, Row);
    const double TileLatBottom = LatitudeForTileRow(Layer, Level, Row + 1);

    // One tile edge maps to the same number of degrees on both axes, so a
    // single pixel span describes the window even though it is not square.
    const double PixelSpan = WindowSpanDegrees / static_cast<double>(TextureHeight);

    // Only walk the window rows and columns this tile can reach.
    const int32 FirstY = FMath::Max(0, FMath::FloorToInt32((WindowLatMax - TileLatTop) / PixelSpan));
    const int32 LastY = FMath::Min(TextureHeight - 1, FMath::CeilToInt32((WindowLatMax - TileLatBottom) / PixelSpan));
    const int32 FirstX = FMath::Max(0, FMath::FloorToInt32((TileLonMin - WindowLonMin) / PixelSpan));
    const int32 LastX = FMath::Min(TextureWidth - 1, FMath::CeilToInt32((TileLonMin + TileLonSpan - WindowLonMin) / PixelSpan));
    if (FirstY > LastY || FirstX > LastX)
    {
        return;
    }

    for (int32 Y = FirstY; Y <= LastY; ++Y)
    {
        const double Latitude = WindowLatMax - (Y + 0.5) * PixelSpan;
        // Fractional row within this tile, which is where the projection
        // difference actually shows up.
        const double RowInTile = TileRowForLatitude(Layer, Level, Latitude) - Row;
        if (RowInTile < 0.0 || RowInTile >= 1.0)
        {
            continue;
        }
        const int32 SrcY = FMath::Clamp(static_cast<int32>(RowInTile * SrcHeight), 0, SrcHeight - 1);
        for (int32 X = FirstX; X <= LastX; ++X)
        {
            const double Longitude = WindowLonMin + (X + 0.5) * PixelSpan;
            const double ColumnInTile = (Longitude - TileLonMin) / TileLonSpan;
            if (ColumnInTile < 0.0 || ColumnInTile >= 1.0)
            {
                continue;
            }
            const int32 SrcX = FMath::Clamp(static_cast<int32>(ColumnInTile * SrcWidth), 0, SrcWidth - 1);
            const int64 SrcIndex = (static_cast<int64>(SrcY) * SrcWidth + SrcX) * 4;
            const int64 DestIndex = static_cast<int64>(Y) * TextureWidth + X;

            if (Layer.Encoding == EGeoTileEncoding::TerrariumElevation)
            {
                // Bilinear, unlike the colour path. Elevation tiles are
                // magnified into the window (256-pixel sources against
                // 512-pixel window tiles), and the material differentiates
                // whatever lands here: nearest-neighbour leaves stair steps
                // that the slope turns into hard ridges, which show up as
                // streaks across otherwise smooth ground.
                const double SampleX = ColumnInTile * SrcWidth - 0.5;
                const double SampleY = RowInTile * SrcHeight - 0.5;
                const int32 X0 = FMath::FloorToInt32(SampleX);
                const int32 Y0 = FMath::FloorToInt32(SampleY);
                const double FractionX = SampleX - X0;
                const double FractionY = SampleY - Y0;
                const double Top = FMath::Lerp(
                    DecodeTerrariumMetres(Raw, SrcWidth, SrcHeight, X0, Y0),
                    DecodeTerrariumMetres(Raw, SrcWidth, SrcHeight, X0 + 1, Y0), FractionX);
                const double Bottom = FMath::Lerp(
                    DecodeTerrariumMetres(Raw, SrcWidth, SrcHeight, X0, Y0 + 1),
                    DecodeTerrariumMetres(Raw, SrcWidth, SrcHeight, X0 + 1, Y0 + 1), FractionX);
                const double Metres = FMath::Lerp(Top, Bottom, FractionY);
                const double Normalised = (Metres - ElevationFloorMetres) / (ElevationCeilingMetres - ElevationFloorMetres);
                Heights[DestIndex] = static_cast<uint16>(FMath::Clamp(Normalised, 0.0, 1.0) * 65535.0);
                continue;
            }
            // Where a sparse layer observed nothing, leave whatever is
            // underneath standing rather than punching a hole in the globe.
            if (Layer.bMayBeSparse && Raw[SrcIndex + 3] < 8)
            {
                continue;
            }
            Pixels[DestIndex] = FColor(Raw[SrcIndex + 2], Raw[SrcIndex + 1], Raw[SrcIndex + 0], 255);
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
    if (bElevation)
    {
        FMemory::Memcpy(Data, Heights.GetData(), Heights.Num() * sizeof(uint16));
    }
    else
    {
        FMemory::Memcpy(Data, Pixels.GetData(), Pixels.Num() * sizeof(FColor));
    }
    Mip.BulkData.Unlock();
    Texture->UpdateResource();
    bDirty = false;
}
