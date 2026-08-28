#include "GeoTrackTracker.h"

#include "GeoMathLibrary.h"

EGeoTrackUpdateResult FGeoTrackTracker::AddSighting(const FString& EntityKey, const FGeoPosition& Position, double DeclaredAltitudeScale, double NowSeconds)
{
    if (EntityKey.IsEmpty())
    {
        return EGeoTrackUpdateResult::Ignored;
    }

    if (FGeoTrackedTrail* Existing = Trails.Find(EntityKey))
    {
        Existing->LastUpdateSeconds = NowSeconds;
        const FGeoTrailPoint& Last = Existing->Points.Last();
        // Horizontal distance only considers latitude/longitude (great-circle
        // ignores altitude); a purely vertical climb/descent must count as
        // movement too, so it is checked separately rather than folded into
        // one 3D distance.
        const double HorizontalMeters = UGeoMathLibrary::GreatCircleDistanceKm(Last.Position, Position) * 1000.0;
        const double VerticalMeters = FMath::Abs(Position.AltitudeMeters - Last.Position.AltitudeMeters);
        if (HorizontalMeters <= MovementThresholdMeters && VerticalMeters <= MovementThresholdMeters)
        {
            return EGeoTrackUpdateResult::Stationary;
        }
        FGeoTrailPoint& NewPoint = Existing->Points.AddDefaulted_GetRef();
        NewPoint.Position = Position;
        NewPoint.DeclaredAltitudeScale = DeclaredAltitudeScale;
        NewPoint.AddedSeconds = NowSeconds;
        const int32 Cap = FMath::Max(1, MaxPointsPerTrail);
        if (Existing->Points.Num() > Cap)
        {
            // Oldest point(s) evicted first: trails are chronological, so the
            // front of the array is always the earliest retained sample. The
            // count form (rather than assuming exactly one overflow) stays
            // correct even if MaxPointsPerTrail is lowered at runtime.
            Existing->Points.RemoveAt(0, Existing->Points.Num() - Cap, EAllowShrinking::No);
        }
        return EGeoTrackUpdateResult::Moved;
    }

    EvictLeastRecentlyUpdatedIfFull();
    FGeoTrackedTrail& NewTrail = Trails.Add(EntityKey);
    NewTrail.EntityKey = EntityKey;
    NewTrail.LastUpdateSeconds = NowSeconds;
    FGeoTrailPoint& SeedPoint = NewTrail.Points.AddDefaulted_GetRef();
    SeedPoint.Position = Position;
    SeedPoint.DeclaredAltitudeScale = DeclaredAltitudeScale;
    SeedPoint.AddedSeconds = NowSeconds;
    return EGeoTrackUpdateResult::FirstSighting;
}

bool FGeoTrackTracker::RemoveExpiredPoints(double NowSeconds, double MaxPointAgeSeconds)
{
    bool bChanged = false;
    for (auto It = Trails.CreateIterator(); It; ++It)
    {
        TArray<FGeoTrailPoint>& Points = It->Value.Points;
        const int32 Before = Points.Num();
        Points.RemoveAll([NowSeconds, MaxPointAgeSeconds](const FGeoTrailPoint& Point)
        {
            return NowSeconds - Point.AddedSeconds > MaxPointAgeSeconds;
        });
        if (Points.Num() == Before)
        {
            // Nothing aged out this sweep - in particular a brand-new
            // single-point seed is left completely alone here, so it always
            // survives long enough for a second sighting to compare against.
            continue;
        }
        bChanged = true;
        if (Points.Num() < 2)
        {
            It.RemoveCurrent();
        }
    }
    return bChanged;
}

void FGeoTrackTracker::Reset()
{
    Trails.Reset();
}

void FGeoTrackTracker::EvictLeastRecentlyUpdatedIfFull()
{
    const int32 Cap = FMath::Max(1, MaxTrackedEntities);
    if (Trails.Num() < Cap)
    {
        return;
    }
    FString OldestKey;
    double OldestSeconds = TNumericLimits<double>::Max();
    for (const TPair<FString, FGeoTrackedTrail>& Pair : Trails)
    {
        if (Pair.Value.LastUpdateSeconds < OldestSeconds)
        {
            OldestSeconds = Pair.Value.LastUpdateSeconds;
            OldestKey = Pair.Key;
        }
    }
    if (!OldestKey.IsEmpty())
    {
        Trails.Remove(OldestKey);
    }
}
