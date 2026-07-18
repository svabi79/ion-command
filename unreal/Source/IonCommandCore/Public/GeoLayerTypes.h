#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"
#include "GeoLayerTypes.generated.h"

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoLayerManifest
{
    GENERATED_BODY()

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString LayerId;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString DisplayName;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString Domain;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") TArray<FString> AcceptedSemanticTypes;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") TArray<EGeoGeometryType> GeometryTypes;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bDefaultVisibility = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") double DefaultTimeWindowSeconds = 900.0;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsSelection = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsReplay = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsAggregation = false;
};

class IONCOMMANDCORE_API IGeoRenderAdapter
{
public:
    virtual ~IGeoRenderAdapter() = default;
    virtual bool Supports(const FGeoMessageEnvelope& Message) const = 0;
    virtual void Submit(const FGeoMessageEnvelope& Message) = 0;
    virtual void Reset() = 0;
};

