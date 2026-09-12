#include "GeoTileMosaic.h"
#include "Misc/AutomationTest.h"
#include "UObject/Package.h"

#if WITH_DEV_AUTOMATION_TESTS

// The packaged client GPU-crashed on D3D12's Copy Engine after
// "detail imagery: level 5 ... at 47.416, 8.473" at 5120x1440. The hang
// was not VRAM: it was UpdateResource() tearing down the mosaic texture
// on every tile while a previous copy was still reading it. These tests
// lock the two decisions that prevent that class of fault: the wall-
// resolution window still chooses a real close-orbit level (we do not
// "fix" the crash by staying coarse), and a GPU copy is issued only when
// the destination resource exists, the CPU buffer matches it, and no
// previous copy is still in flight.

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoTileMosaicWallResolutionChoosesLevel5Test,
    "IONCOMMAND.Visualization.TileMosaic.WallResolutionChoosesLevel5",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoTileMosaicWallResolutionChoosesLevel5Test::RunTest(const FString& Parameters)
{
    UGeoTileMosaic* Mosaic = NewObject<UGeoTileMosaic>(GetTransientPackage());
    FGeoTileLayer Base;
    Base.MaxLevel = 7;
    Base.NativeTilePixels = 512;
    Base.RootSpanDegrees = 288.0;
    FGeoTileLayer Detail;
    Detail.MaxLevel = 13;
    Detail.NativeTilePixels = 256;
    Detail.RootSpanDegrees = 180.0;
    Mosaic->Configure({Base, Detail});

    // Same numbers as the crash log: mosaic just turned on (12 deg span
    // threshold), Zurich, 5120x1440. Cos(47.416°) ≈ 0.677.
    constexpr double SpanDegrees = 12.0;
    constexpr double Latitude = 47.416;
    constexpr int32 ScreenWidth = 5120;
    constexpr int32 ScreenHeight = 1440;
    const double AspectRatio = static_cast<double>(ScreenWidth) / static_cast<double>(ScreenHeight);
    const double CosLatitude = FMath::Max(0.05, FMath::Cos(FMath::DegreesToRadians(Latitude)));
    const double SpanLongitudeDegrees = FMath::Min(360.0, SpanDegrees * AspectRatio / CosLatitude);

    const int32 Level = Mosaic->PreviewLevel(SpanDegrees, SpanLongitudeDegrees, ScreenHeight);
    TestEqual(TEXT("5120x1440 at the mosaic threshold over Zurich is window level 5"), Level, 5);
    TestTrue(TEXT("level is a real close-orbit window, not a zoom cap"), Level >= 5);

    // Closer orbit must still go deeper. The crash fix is the upload path,
    // not a ceiling on ChooseLevel.
    const int32 CloseLevel = Mosaic->PreviewLevel(0.57, 3.0, ScreenHeight);
    TestTrue(TEXT("closest approach still picks a finer window than level 5"), CloseLevel > 5);
    TestTrue(TEXT("closest approach stays inside the 0..24 search"), CloseLevel <= 24);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoTileMosaicUploadDecisionIsSafeTest,
    "IONCOMMAND.Visualization.TileMosaic.UploadDecisionIsSafe",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoTileMosaicUploadDecisionIsSafeTest::RunTest(const FString& Parameters)
{
    const int64 Expected = static_cast<int64>(UGeoTileMosaic::TextureWidth) * UGeoTileMosaic::TextureHeight;

    TestEqual(TEXT("clean frame does not touch the GPU"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(false, true, false, Expected, Expected)),
        static_cast<int32>(EGeoMosaicUploadDecision::NoWork));

    TestEqual(TEXT("in-flight copy is retried, not overlapped"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(true, true, true, Expected, Expected)),
        static_cast<int32>(EGeoMosaicUploadDecision::RetryLater));

    TestEqual(TEXT("missing RHI resource is retried, not recreated from here"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(true, false, false, Expected, Expected)),
        static_cast<int32>(EGeoMosaicUploadDecision::RetryLater));

    TestEqual(TEXT("zero-size destination is skipped, not copied"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(true, true, false, Expected, 0)),
        static_cast<int32>(EGeoMosaicUploadDecision::SkipInvalid));

    TestEqual(TEXT("cpu buffer that does not match the texture is skipped"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(true, true, false, 0, Expected)),
        static_cast<int32>(EGeoMosaicUploadDecision::SkipInvalid));

    TestEqual(TEXT("ready resource, matching buffer, no in-flight copy: upload"),
        static_cast<int32>(UGeoTileMosaic::DecideUpload(true, true, false, Expected, Expected)),
        static_cast<int32>(EGeoMosaicUploadDecision::Upload));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoTileMosaicPumpBudgetTest,
    "IONCOMMAND.Visualization.TileMosaic.PumpBudget",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoTileMosaicPumpBudgetTest::RunTest(const FString& Parameters)
{
    int32 CacheReads = 0;
    int32 DownloadStarts = 0;

    UGeoTileMosaic::DecidePumpBudget(400, 200, 0, CacheReads, DownloadStarts);
    TestEqual(TEXT("warm cache is drained a few tiles per frame, not all at once"),
        CacheReads, UGeoTileMosaic::MaxCacheLoadsPerPump);
    TestEqual(TEXT("cold cache starts a bounded parallel batch"),
        DownloadStarts, UGeoTileMosaic::MaxInFlightDownloads);

    UGeoTileMosaic::DecidePumpBudget(3, 50, 7, CacheReads, DownloadStarts);
    TestEqual(TEXT("leftover warm tiles still paint this frame"), CacheReads, 3);
    TestEqual(TEXT("in-flight slots are not oversubscribed"), DownloadStarts, 1);

    UGeoTileMosaic::DecidePumpBudget(0, 10, UGeoTileMosaic::MaxInFlightDownloads, CacheReads, DownloadStarts);
    TestEqual(TEXT("full HTTP pool starts nothing more"), DownloadStarts, 0);
    TestEqual(TEXT("no cache work when the queue is downloads-only"), CacheReads, 0);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoTileMosaicCacheKeyTest,
    "IONCOMMAND.Visualization.TileMosaic.CacheKey",
    EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoTileMosaicCacheKeyTest::RunTest(const FString& Parameters)
{
    const FString Path = UGeoTileMosaic::MakeCacheRelativePath(TEXT("s2cloudless"), 5, 20, 10, TEXT("jpeg"));
    const FString Expected = FString(TEXT("TileCache")) / TEXT("s2cloudless") / TEXT("5_10_20.jpeg");
    TestEqual(TEXT("cache key is layer/level_row_col.ext"), Path, Expected);
    TestTrue(TEXT("write and read use the same name"),
        Path == UGeoTileMosaic::MakeCacheRelativePath(TEXT("s2cloudless"), 5, 20, 10, TEXT("jpeg")));
    return true;
}

#endif
