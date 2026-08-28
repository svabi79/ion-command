#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoTrackTracker.h"
#include "GeoTrackLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;

// Motion trails: a short, fading comet tail behind any moving entity -
// aircraft, satellite, vessel, whatever the domain turns out to be. The layer
// has no idea which: it renders a trail for any stable entity id whose
// successive Point sightings actually move, and never for one that does not
// (see FGeoTrackTracker). Segments are instanced and GPU age-faded exactly
// like AGeoArcLayerActor's propagation arcs - study that class for the
// reference pattern this one reuses rather than inventing a new mechanism.
//
// Hidden by default is NOT chosen here: trails start visible (matching the
// always-on marker/arc layers) because the point of the feature is to be
// seen without operator setup. Tastefulness instead comes from the small
// default caps below (see their comments) plus the GPU fade, not from a
// closed-by-default toggle. The operator can still hide the whole layer with
// the T key or the overlay menu's TRAILS row, same as the H/I toggles for
// the heatmap and ionosphere shells.
UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoTrackLayerActor final : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoTrackLayerActor();
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;
    virtual void Submit(const FGeoMessageEnvelope& Message) override;
    virtual void Reset() override;

    // Read-only access for instrumentation and tests.
    const FGeoTrackTracker& GetTracker() const { return Tracker; }

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    // Bounded population: 300 keeps the layer to the currently-busiest
    // movers even under the global aviation snapshot (thousands of
    // airframes). Least-recently-updated entities are evicted first, so the
    // tracked set self-selects toward whoever is currently reporting fresh
    // positions rather than whoever happened to be seen first.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer", meta=(ClampMin="1")) int32 MaxTrackedEntities = 300;
    // Short comet tail by design: enough samples to read a heading and a
    // recent turn, not a full flight history. Kept small so the default-on
    // layer cannot bury the globe in traffic (see the class comment).
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer", meta=(ClampMin="2")) int32 MaxPointsPerTrail = 12;
    // Horizontal/vertical movement gate in meters; see
    // FGeoTrackTracker::MovementThresholdMeters for the rationale.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double MovementThresholdMeters = 25.0;
    // Great-circle subdivisions per retained point-to-point leg, so a trail
    // hugs the globe surface instead of a straight chord that can cut
    // through the planet when two consecutive samples are far apart.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer", meta=(ClampMin="1")) int32 SegmentsPerLeg = 4;
    // GPU age-fade lifetime for one trail segment (reuses the arc layer's
    // spawn-time/inverse-lifetime custom-data mechanism - no CPU fade).
    // Chosen so a fully populated 12-point trail at a typical multi-second
    // update cadence fades out at roughly the same render-clock age its
    // oldest point would be evicted by count, instead of the count cap
    // truncating a still-bright tail or the fade leaving stale invisible
    // geometry well past its point being evicted.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double TrailPointLifetimeSeconds = 45.0;
    // Cylinder scale factor, thinner than propagation arcs (0.02 default) so
    // a trail reads as a subtle wake instead of competing with markers and
    // arcs for attention.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double TrailThickness = 0.012;
    // Geometry rebuilds coalesce to this cadence instead of firing per
    // accepted message, matching GeoPointLayerActor's movement-rebuild
    // coalescing so a burst of position updates costs one rebuild, not one
    // per message.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double RebuildIntervalSeconds = 1.0;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    bool ResolveAltitudeExaggerationEnabled() const;
    FVector ResolveRenderLocation(const FGeoPosition& Position, double DeclaredAltitudeScale, bool bAltitudeExaggeration) const;
    void RebuildInstances();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> TrailInstances;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;

    FGeoTrackTracker Tracker;
    bool bNeedsRebuild = false;
    double LastRebuildSeconds = 0.0;
    double LastExpirySweepSeconds = 0.0;
};
