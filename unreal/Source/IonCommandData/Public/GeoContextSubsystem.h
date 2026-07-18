#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "GeoContextSubsystem.generated.h"

UCLASS()
class IONCOMMANDDATA_API UGeoContextSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Context")
    TArray<FGeoMessageEnvelope> QueryNearby(const FGeoPosition& Center, double RadiusKm, const FString& SemanticTypePrefix) const;

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Context")
    TArray<FGeoMessageEnvelope> QueryTimeRange(FDateTime FromUtc, FDateTime ToUtc, const FString& SemanticTypePrefix) const;

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Context")
    TArray<FGeoMessageEnvelope> QueryEntityRelationships(const FString& EntityId) const;
};

