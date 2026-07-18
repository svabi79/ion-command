#pragma once

#include "CoreMinimal.h"
#include "Engine/DataAsset.h"
#include "HamBandVisualConfig.generated.h"

USTRUCT(BlueprintType)
struct IONCOMMANDHAMRADIO_API FHamBandVisualDefinition
{
    GENERATED_BODY()

    UPROPERTY(EditAnywhere, BlueprintReadOnly) FString BandId;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) int32 PaletteIndex = 10;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) FLinearColor Color = FLinearColor::White;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) double MinFrequencyHz = 0;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) double MaxFrequencyHz = 0;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) double LineThickness = 1.0;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) double ParticleSpeed = 1.0;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) double MaximumLifetimeSeconds = 900.0;
    UPROPERTY(EditAnywhere, BlueprintReadOnly) bool bDefaultVisible = true;
};

UCLASS(BlueprintType)
class IONCOMMANDHAMRADIO_API UHamBandVisualConfig final : public UDataAsset
{
    GENERATED_BODY()

public:
    UHamBandVisualConfig();
    int32 ResolvePaletteIndex(const FString& BandId) const;

    UPROPERTY(EditAnywhere, BlueprintReadOnly, Category="ION COMMAND|Ham Radio")
    TArray<FHamBandVisualDefinition> Bands;
};

