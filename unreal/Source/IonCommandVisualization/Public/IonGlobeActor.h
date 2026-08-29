#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "IonGlobeActor.generated.h"

class UDirectionalLightComponent;
class UMaterialInstanceDynamic;
class UPointLightComponent;
class USceneComponent;
class UGeoTileMosaic;
class UStaticMeshComponent;
class UTexture2D;

UCLASS()
class IONCOMMANDVISUALIZATION_API AIonGlobeActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonGlobeActor();
    virtual void BeginPlay() override;
    virtual void Tick(float DeltaSeconds) override;

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Globe")
    double GlobeRadius = 1000.0;

private:
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Starfield;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Earth;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Atmosphere;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Clouds;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UDirectionalLightComponent> SunLight;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UDirectionalLightComponent> RimLight;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UPointLightComponent> CoreGlow;
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> EarthMID;
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> AtmosphereMID;
    // Live cloud overlay: the packaged static texture is replaced at runtime
    // by the current EUMETSAT world IR composite (hourly refresh).
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> CloudsMID;
    UPROPERTY(Transient) TObjectPtr<UTexture2D> LiveCloudTexture;
    FTimerHandle CloudRefreshTimer;
    void RequestLiveClouds();
    void ApplyLiveClouds(const TArray<uint8>& PngBytes);

    // Close-orbit imagery. The global textures top out around 1.9 km per
    // pixel, which is already coarse at 1000 km up and pure mush at the
    // closest approach; below that altitude a tile window follows the camera
    // and supplies whatever resolution the current altitude calls for.
    UPROPERTY(Transient) TObjectPtr<UGeoTileMosaic> DetailImagery;
    // Terrain relief for the same window. The globe mesh is far too coarse
    // to displace at this scale - one quad spans ~100 km, and the closest
    // orbit sees 43 km - so the height field drives a surface normal
    // instead. The silhouette stays smooth; everything the light touches
    // does not.
    UPROPERTY(Transient) TObjectPtr<UGeoTileMosaic> DetailElevation;
    void UpdateDetailImagery();
    // Seconds since the window was last re-aimed. The camera moves every
    // frame; the tile service does not need to hear about it that often.
    double DetailCheckTimer = 0.0;
    int32 LastDetailLevel = -1;
};
