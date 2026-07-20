#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "IonGlobeActor.generated.h"

class UDirectionalLightComponent;
class UMaterialInstanceDynamic;
class UPointLightComponent;
class USceneComponent;
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
};
