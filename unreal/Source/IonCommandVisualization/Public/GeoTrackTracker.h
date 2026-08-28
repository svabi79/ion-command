#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"

// One retained sample along a moving entity's trail: the canonical position,
// the domain-declared altitude-exaggeration factor active for THIS sample
// (visual.altitudeScale, read the same way GeoPointLayerActor reads it, so a
// trail lifts off the surface by the same amount as the marker it follows),
// and the render-clock time (GetWorld()->GetTimeSeconds() at the caller) it
// was captured. AddedSeconds drives both the GPU age fade of the segment
// leading into this point and this point's own CPU-side expiry.
struct FGeoTrailPoint
{
    FGeoPosition Position;
    double DeclaredAltitudeScale = 1.0;
    double AddedSeconds = 0.0;
};

// One tracked entity's bounded, chronological trail. Points[0] is the oldest
// retained sample and is the first one evicted once the per-trail cap is
// exceeded; Points.Last() is the most recent sighting.
struct FGeoTrackedTrail
{
    FString EntityKey;
    TArray<FGeoTrailPoint> Points;
    double LastUpdateSeconds = 0.0;
};

// Outcome of feeding one canonical Point sighting into the tracker.
enum class EGeoTrackUpdateResult : uint8
{
    // No stable entity id: a one-shot observation can never form a trail, so
    // it is never even considered.
    Ignored,
    // Known entity, but the new position is within tolerance of the last
    // recorded point: no point was added. This is what keeps a fixed ground
    // station or ionosonde from ever accumulating a trail, without the
    // tracker knowing what a "ground station" is.
    Stationary,
    // First sighting of this entity: a single seed point is recorded so the
    // NEXT sighting can be compared against it, but nothing is renderable yet
    // (a trail needs two points to draw one segment).
    FirstSighting,
    // The position moved past the tolerance: a new point was appended and at
    // least one segment is now renderable for this entity.
    Moved,
};

// Domain-neutral movement detection and bounded trail bookkeeping. Deliberately
// free of any Unreal Actor/rendering dependency so it is covered directly by
// automation tests without spinning up a game world (the same shape as
// FGeoEnvelopeJsonParser in IonCommandData). AGeoTrackLayerActor owns one
// instance and turns its trails into instanced-mesh geometry every rebuild.
class IONCOMMANDVISUALIZATION_API FGeoTrackTracker
{
public:
    // Feeds one canonical Point sighting for a stable entity. EntityKey must
    // already be the caller's resolved stable id; an empty key is reported as
    // Ignored rather than tracked under a throwaway key, since a one-shot
    // message id can never repeat and would only waste a tracked-entity slot.
    EGeoTrackUpdateResult AddSighting(const FString& EntityKey, const FGeoPosition& Position, double DeclaredAltitudeScale, double NowSeconds);

    // Drops points older than MaxPointAgeSeconds from every trail, mirroring
    // the GPU fade window so fully-faded geometry is not retained forever.
    // An entity that LOSES points to this sweep and is left with fewer than
    // two (nothing left to render) is dropped entirely: if it starts moving
    // again later, it begins a fresh trail rather than reconnecting to a
    // stale point across an arbitrarily long gap. A brand-new entity that
    // simply has not had its second sighting yet is untouched by this rule -
    // only entities that actually lost points to the age filter are eligible
    // for removal. Returns true if any point or entity changed.
    bool RemoveExpiredPoints(double NowSeconds, double MaxPointAgeSeconds);

    void Reset();

    int32 NumTrackedEntities() const { return Trails.Num(); }
    const TMap<FString, FGeoTrackedTrail>& GetTrails() const { return Trails; }
    const FGeoTrackedTrail* FindTrail(const FString& EntityKey) const { return Trails.Find(EntityKey); }

    // Bounded population: once this many entities are tracked, the least
    // recently updated one is evicted to make room for a new one.
    int32 MaxTrackedEntities = 300;
    // Bounded per-entity retention: the oldest point is dropped once a new
    // one would exceed this cap.
    int32 MaxPointsPerTrail = 12;
    // A new sighting counts as "moved" only past this horizontal (great-
    // circle) OR vertical distance from the last recorded point, in meters.
    // Large enough to absorb coordinate round-trip/GPS jitter on a genuinely
    // fixed position, tiny next to one polling interval of real aircraft,
    // satellite, or vessel movement.
    double MovementThresholdMeters = 25.0;

private:
    void EvictLeastRecentlyUpdatedIfFull();
    TMap<FString, FGeoTrackedTrail> Trails;
};
