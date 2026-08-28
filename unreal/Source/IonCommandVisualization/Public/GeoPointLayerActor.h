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
    // Kinematics from visual.headingDeg / visual.speedMps: world-space unit
    // heading orients the glyph (material) and, with the speed, dead-reckons
    // the marker between sightings. Zero heading = static marker.
    FVector HeadingWorld = FVector::ZeroVector;
    double SpeedUnitsPerSecond = 0.0;
    // Altitude is kept re-derivable so the exaggeration toggle can recompute
    // every marker: unit radial at the surface point, true altitude, and the
    // domain-declared visual exaggeration factor.
    FVector RadialDirection = FVector::ZeroVector;
    double AltitudeMeters = 0.0;
    double DeclaredAltitudeScale = 1.0;
    bool bOnGround = false;
    // Domain drives the overlay menu's per-category visibility; the display
    // strings feed the hover tooltip.
    FString Domain;
    FString Title;
    FString Primary;
    FString Secondary;
    FString Tertiary;
    // Sticky emergency state: once an aircraft squawks 7500/7600/7700 the red
    // tint, enlarged scale, and alarm title survive later non-emergency
    // sightings from another source until the marker expires.
    bool bEmergency = false;
    // Stable render slot: the index into MarkerInstances currently showing
    // this marker, or INDEX_NONE while it is tracked but not rendered (e.g.
    // hidden by a domain/altitude/ground filter or expired but not yet
    // swept). See AGeoPointLayerActor::AddRenderInstance/RemoveRenderInstance.
    int32 RenderSlot = INDEX_NONE;
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

    // Overlay-menu toggle: apply/ignore the domain-declared altitude
    // exaggeration (visual.altitudeScale). Off renders true-scale altitude.
    bool IsAltitudeExaggerationEnabled() const { return bAltitudeExaggeration; }
    void SetAltitudeExaggerationEnabled(bool bEnabled);

    // Aviation declutter (settings panel): hide aircraft below a minimum true
    // altitude, and/or hide on-ground aircraft. Other domains are unaffected.
    double GetMinAircraftAltitudeMeters() const { return MinAircraftAltitudeMeters; }
    void SetMinAircraftAltitudeMeters(double Meters);
    bool GetShowGroundAircraft() const { return bShowGroundAircraft; }
    void SetShowGroundAircraft(bool bShow);
    // Client marker lifetime (settings panel).
    void SetMarkerLifetimeSeconds(double Seconds);
    double GetMarkerLifetimeSeconds() const { return MarkerLifetimeSeconds; }

    // Render-population instrumentation: counts of incremental slot
    // operations (this layer no longer performs a full clear-and-readd
    // rebuild anywhere - see the class-level comment in the .cpp), plus live
    // tracked/rendered counts. Exposed the same way
    // UGeoDataSubsystem::GetStatistics() exposes its counters.
    FGeoRenderLayerStatistics GetRenderStatistics() const;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    // Sized for the global aviation snapshot (~8k airframes) on top of the
    // ham station, lightning, and satellite populations.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisiblePoints = 60000;
    // Pictogram quads read slightly smaller than the old solid spheres at
    // equal scale, hence the larger default.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerScale = 0.2;
    // A marker survives this long past its last sighting; one-shot
    // observations (lightning strikes) fade much sooner than entities.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MarkerLifetimeSeconds = 300.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ObservationLifetimeSeconds = 30.0;
    // Moving markers trigger a coalesced dead-reckoning pass once their
    // rendered position lags by this many world units (0.25 = ~1.6 km ground
    // distance), at most every MovementRebuildSeconds.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MovementTolerance = 0.25;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MovementRebuildSeconds = 2.0;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    // One instance's worth of custom data for the pictogram material:
    // icon index, RGB tint, world origin (billboard pivot).
    static void AppendCustomData(TArray<float>& Out, const FRenderedGeoPoint& Point);
    // Single source of truth for marker expiry, shared by the render path,
    // the batched cleanup sweep, and the hover pick so they never disagree.
    bool IsExpired(const FRenderedGeoPoint& Point, double NowSeconds) const;
    bool IsAircraftFiltered(const FRenderedGeoPoint& Point) const;
    // Single source of truth for "where is this point right now": resets to
    // the authoritative Location, then re-applies the dead-reckoning coast
    // delta for a kinematic point. Used by both the movement cadence and
    // the style/filter reconcile pass so a kinematic marker's position
    // never depends on WHICH trigger last touched it - without this shared
    // computation, a reconcile pass triggered by an unrelated point's style
    // change would snap a moving marker back to its last sighting instead
    // of its current coasted position.
    void RefreshRenderedLocation(FRenderedGeoPoint& Point, double NowSeconds) const;

    // --- Stable render slots ---------------------------------------------
    // ActivePoints tracks every live entity regardless of visibility;
    // SlotToPointIndex[RenderSlot] gives the ActivePoints index currently
    // occupying a given MarkerInstances slot, and MarkerInstances holds a
    // dense (hole-free) run - i.e. SlotToPointIndex.Num() ==
    // MarkerInstances->GetInstanceCount() always. A tracked point that is
    // not currently rendered (hidden by a filter, or awaiting the expiry
    // sweep) has RenderSlot == INDEX_NONE and no entry in either array. See
    // GeoRenderSlotMath.h for the swap-and-pop removal strategy shared with
    // the arc layer, and GeoRenderSlotMathTests.cpp for the consistency
    // proof.

    // Gives ActivePoints[Index] a render instance; it must not already have
    // one. Appends to the tail of MarkerInstances.
    void AddRenderInstance(int32 Index, double NowSeconds);
    // Removes ActivePoints[Index]'s render instance via swap-and-pop against
    // MarkerInstances' current last instance; it must already have one.
    void RemoveRenderInstance(int32 Index);
    // Removes ActivePoints[Index] entirely (releasing its render instance
    // first if it has one) via swap-and-pop against ActivePoints' current
    // last tracked entry, fixing up EntityToPoint and SlotToPointIndex for
    // whichever entry moved.
    void RemoveTrackedPoint(int32 Index);
    // Removes up to MaxRemovals expired points, oldest-eligible first,
    // bounding the work any single call can do. Returns the number removed.
    int32 EvictExpiredPoints(double NowSeconds, int32 MaxRemovals);
    // Evicts the least-recently-seen points, bounded by MaxRemovals, to
    // bring ActivePoints back under MaxVisiblePoints. Returns the number
    // removed.
    int32 TrimToCapacity(int32 MaxRemovals);
    // Coalesced dead-reckoning pass (see MovementRebuildSeconds): re-coasts
    // every kinematic point's RenderedLocation and flushes any point whose
    // true Location drifted past MovementTolerance since the last pass,
    // touching the instance buffer only for points that are both rendered
    // and actually moved.
    void ApplyMovementUpdates(double NowSeconds);
    // Reconciles the rendered subset of ActivePoints against current
    // visibility (domain/altitude/ground filters, expiry) and per-point
    // style (icon/tint/emergency/scale) and global presentation state
    // (zoom, altitude exaggeration). Adds/removes only the points whose
    // should-render state changed; every already-and-still-rendered point
    // gets its transform/custom data refreshed in place on its existing
    // slot, so no other point's slot is ever disturbed by this pass.
    void ReconcileRenderedSet();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    // Single camera-facing quad pool; entity/observation split lives in the
    // bookkeeping (lifetimes), not in separate components anymore.
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> MarkerInstances;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    // Markers keep a roughly constant screen size: world scale follows the
    // camera distance instead of ballooning when the operator zooms in.
    double CurrentZoomFactor = 1.0;
    // Stable per-entity markers with age-based expiry and coalesced
    // dead-reckoning updates. Not a UPROPERTY: FRenderedGeoPoint is a plain
    // struct (no UObject references for the GC to trace), matching
    // EntityToPoint/HiddenDomains below.
    TArray<FRenderedGeoPoint> ActivePoints;
    TMap<FString, int32> EntityToPoint;
    // Render-slot reverse map; see the stable-render-slots comment above.
    TArray<int32> SlotToPointIndex;
    TSet<FString> HiddenDomains;
    bool bAltitudeExaggeration = true;
    // Aviation declutter filter (settings-panel controlled).
    double MinAircraftAltitudeMeters = 0.0;
    bool bShowGroundAircraft = true;
    bool bNeedsRebuild = false;
    // Entities whose true Location drifted past MovementTolerance since the
    // last dead-reckoning pass and are awaiting the coalesced flush (see
    // ApplyMovementUpdates). Keyed by entity id rather than ActivePoints
    // index so an index invalidated by an unrelated swap-and-pop removal
    // between submission and flush is never misread as a different point:
    // a stale key simply misses the EntityToPoint lookup at flush time.
    TSet<FString> DirtyEntityKeys;
    // True while any visible marker dead-reckons; keeps the movement
    // cadence running so gliding markers actually glide.
    bool bHasKinematicPoints = false;
    double LastMovementRebuild = 0.0;
    double LastExpiryCheck = 0.0;
    FGeoRenderLayerStatistics RuntimeStats;
};
