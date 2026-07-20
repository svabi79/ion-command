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
    // Position the render instance actually shows. Movement must be measured
    // against THIS, not Location: Location follows every sighting, so a
    // per-poll delta below the rebuild tolerance would otherwise creep along
    // with the bookkeeping and the instance would never move (aircraft froze
    // for up to an hour while satellites, whose per-update delta exceeds the
    // old threshold, kept moving).
    FVector RenderedLocation = FVector::ZeroVector;
    double LastSeenSeconds = 0.0;
    // Render-clock deadline derived from the envelope's validUntil; zero
    // falls back to the class lifetime defaults.
    double ExpireAtSeconds = 0.0;
    // Per-message size multiplier (visual.markerScale property).
    float Scale = 1.0f;
    bool bObservation = false;
    // Pictogram atlas tile (visual.icon property resolved through the icon
    // registry) and its tint, both baked into per-instance custom data.
    float IconIndex = 0.0f;
    FLinearColor Color = FLinearColor(0.0f, 0.82f, 1.0f);
    // Domain drives the overlay menu's per-category visibility; the display
    // strings feed the hover tooltip.
    FString Domain;
    FString Title;
    FString Primary;
    FString Secondary;
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

    // Overlay-menu support: domains present in the active window and their
    // per-category visibility (hidden domains skip rendering, data stays).
    void GetPresentDomains(TArray<FString>& OutDomains) const;
    bool IsDomainVisible(const FString& Domain) const { return !HiddenDomains.Contains(Domain); }
    void SetDomainVisible(const FString& Domain, bool bVisible);

    // Hover pick: nearest visible marker to the ray within MaxDistance world
    // units, or nullptr. CPU scan, call throttled.
    const FRenderedGeoPoint* FindNearestToRay(const FVector& RayOrigin, const FVector& RayDirection, double MaxDistance) const;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisiblePoints = 25000;
    // Pictogram quads read slightly smaller than the old solid spheres at
    // equal scale, hence the larger default.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerScale = 0.2;
    // A marker survives this long past its last sighting; one-shot
    // observations (lightning strikes) fade much sooner than entities.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerLifetimeSeconds = 300.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ObservationLifetimeSeconds = 30.0;
    // Moving markers trigger a coalesced rebuild once their rendered position
    // lags by this many world units (0.25 = ~1.6 km ground distance), at most
    // every MovementRebuildSeconds.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MovementTolerance = 0.25;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MovementRebuildSeconds = 2.0;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    void RebuildInstances();
    // One instance's worth of custom data for the pictogram material:
    // icon index, RGB tint, world origin (billboard pivot).
    static void AppendCustomData(TArray<float>& Out, const FRenderedGeoPoint& Point);
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    // Single camera-facing quad pool; entity/observation split lives in the
    // bookkeeping (lifetimes), not in separate components anymore.
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> MarkerInstances;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    // Markers keep a roughly constant screen size: world scale follows the
    // camera distance instead of ballooning when the operator zooms in.
    double CurrentZoomFactor = 1.0;
    // Stable per-entity markers with age-based expiry and coalesced rebuilds.
    TArray<FRenderedGeoPoint> ActivePoints;
    TMap<FString, int32> EntityToPoint;
    TSet<FString> HiddenDomains;
    bool bNeedsRebuild = false;
    // Movement-only dirtiness coalesces into a slower rebuild cadence so a
    // few hundred aircraft updating every poll do not force a full rebuild
    // four times a second.
    bool bMovementDirty = false;
    double LastMovementRebuild = 0.0;
    double LastExpiryCheck = 0.0;
};
