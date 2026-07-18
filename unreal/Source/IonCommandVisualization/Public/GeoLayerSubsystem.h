#pragma once

#include "CoreMinimal.h"
#include "GeoLayerTypes.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoLayerSubsystem.generated.h"

DECLARE_MULTICAST_DELEGATE_TwoParams(FOnGeoLayerVisibilityChanged, const FString&, bool);

UCLASS()
class IONCOMMANDVISUALIZATION_API UGeoLayerSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Layer") bool RegisterLayer(const FGeoLayerManifest& Manifest);
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Layer") bool SetLayerVisible(const FString& LayerId, bool bVisible);
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Layer") bool IsLayerVisible(const FString& LayerId) const;
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Layer") TArray<FGeoLayerManifest> GetLayers() const;
    FOnGeoLayerVisibilityChanged& OnLayerVisibilityChanged() { return LayerVisibilityChanged; }

private:
    UPROPERTY(Transient) TMap<FString, FGeoLayerManifest> Manifests;
    UPROPERTY(Transient) TMap<FString, bool> Visibility;
    FOnGeoLayerVisibilityChanged LayerVisibilityChanged;
};
