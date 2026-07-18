#include "HamBandVisualConfig.h"

namespace
{
FHamBandVisualDefinition MakeBand(const TCHAR* Id, int32 PaletteIndex, const FLinearColor& Color, double MinMHz, double MaxMHz)
{
    FHamBandVisualDefinition Result;
    Result.BandId = Id; Result.PaletteIndex = PaletteIndex; Result.Color = Color;
    Result.MinFrequencyHz = MinMHz * 1000000.0; Result.MaxFrequencyHz = MaxMHz * 1000000.0;
    return Result;
}
}

UHamBandVisualConfig::UHamBandVisualConfig()
{
    Bands = {
        MakeBand(TEXT("160m"), 0, FLinearColor(0.55f, 0.08f, 1.0f), 1.8, 2.0),
        MakeBand(TEXT("80m"), 1, FLinearColor(0.15f, 0.16f, 1.0f), 3.5, 4.0),
        MakeBand(TEXT("60m"), 2, FLinearColor(0.0f, 0.9f, 1.0f), 5.0, 5.5),
        MakeBand(TEXT("40m"), 3, FLinearColor(0.05f, 1.0f, 0.36f), 7.0, 7.3),
        MakeBand(TEXT("30m"), 4, FLinearColor(0.55f, 1.0f, 0.05f), 10.1, 10.15),
        MakeBand(TEXT("20m"), 5, FLinearColor(0.0f, 0.42f, 1.0f), 14.0, 14.35),
        MakeBand(TEXT("17m"), 6, FLinearColor(1.0f, 0.9f, 0.05f), 18.068, 18.168),
        MakeBand(TEXT("15m"), 7, FLinearColor(1.0f, 0.42f, 0.02f), 21.0, 21.45),
        MakeBand(TEXT("12m"), 7, FLinearColor(1.0f, 0.42f, 0.02f), 24.89, 24.99),
        MakeBand(TEXT("10m"), 8, FLinearColor(1.0f, 0.05f, 0.02f), 28.0, 29.7),
        MakeBand(TEXT("6m"), 9, FLinearColor(1.0f, 0.02f, 0.6f), 50.0, 54.0)
    };
}

int32 UHamBandVisualConfig::ResolvePaletteIndex(const FString& BandId) const
{
    for (const FHamBandVisualDefinition& Band : Bands) if (Band.BandId == BandId) return Band.PaletteIndex;
    return 10;
}
