#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoPointLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;

// One tracked marker: stable per entity, refreshed on every sighting and
// expired by age instead of the old wipe-the-whole-class-at-cap behavior.
struct FRenderedGeoPoint
{
    FString EntityKey;
    FVector Location = FVector::ZeroVector;
    double LastSeenSeconds = 0.0;
    bool bObservation = false;
};

UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoPointLayerActor final : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoPointLayerActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;
    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;
    virtual void Submit(const FGeoMessageEnvelope& Message) override;
    virtual void Reset() override;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisiblePoints = 25000;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerScale = 0.14;
    // A marker survives this long past its last sighting; one-shot
    // observations (lightning strikes) fade much sooner than entities.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerLifetimeSeconds = 300.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ObservationLifetimeSeconds = 30.0;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    void ApplyZoomFactor(UInstancedStaticMeshComponent* Instances) const;
    void RebuildInstances();
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> EntityInstances;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> ObservationInstances;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    // Markers keep a roughly constant screen size: world scale follows the
    // camera distance instead of ballooning when the operator zooms in.
    double CurrentZoomFactor = 1.0;
    // Stable per-entity markers with age-based expiry and coalesced rebuilds.
    TArray<FRenderedGeoPoint> ActivePoints;
    TMap<FString, int32> EntityToPoint;
    bool bNeedsRebuild = false;
    double LastExpiryCheck = 0.0;
};
