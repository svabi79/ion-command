#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoTypes.h"
#include "IonAuroraActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;
class UMaterialInstanceDynamic;

UCLASS()
class IONCOMMANDVISUALIZATION_API AIonAuroraActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonAuroraActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

private:
    void BuildOval(UInstancedStaticMeshComponent* Instances, bool bNorth);
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> NorthOval;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SouthOval;
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> NorthMaterial;
    UPROPERTY(Transient) TObjectPtr<UMaterialInstanceDynamic> SouthMaterial;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    // Live planetary Kp drives oval latitude and brightness; the mock or the
    // real SWPC feed both arrive as spaceweather.state observations.
    double CurrentKp = 2.0;
    double BuiltKp = -10.0;
};
