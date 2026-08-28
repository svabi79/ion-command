#include "GeoTrackTracker.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

namespace
{
FGeoPosition MakePosition(double Longitude, double Latitude, double AltitudeMeters = 0.0)
{
    FGeoPosition Position;
    Position.Longitude = Longitude;
    Position.Latitude = Latitude;
    Position.AltitudeMeters = AltitudeMeters;
    return Position;
}
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackMovingEntityTest, "IONCOMMAND.Visualization.Track.MovingEntityAccumulatesPoints", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackMovingEntityTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;

    // First sighting only ever seeds a single point: nothing is renderable
    // from one sample alone.
    const EGeoTrackUpdateResult First = Tracker.AddSighting(TEXT("A1"), MakePosition(8.0, 47.0), 1.0, 0.0);
    TestEqual(TEXT("first sighting is reported as FirstSighting"), static_cast<int32>(First), static_cast<int32>(EGeoTrackUpdateResult::FirstSighting));
    const FGeoTrackedTrail* SeededTrail = Tracker.FindTrail(TEXT("A1"));
    TestNotNull(TEXT("entity is tracked after the first sighting"), SeededTrail);
    if (!SeededTrail)
    {
        return false;
    }
    TestEqual(TEXT("exactly one seed point recorded"), SeededTrail->Points.Num(), 1);

    // A real move (fractions of a degree away is tens of kilometers, far past
    // the default movement threshold) must append a second point.
    const EGeoTrackUpdateResult Second = Tracker.AddSighting(TEXT("A1"), MakePosition(8.2, 47.1), 1.0, 5.0);
    TestEqual(TEXT("moved sighting is reported as Moved"), static_cast<int32>(Second), static_cast<int32>(EGeoTrackUpdateResult::Moved));
    TestEqual(TEXT("two points retained after the move"), Tracker.FindTrail(TEXT("A1"))->Points.Num(), 2);

    // A third move keeps extending the same trail.
    Tracker.AddSighting(TEXT("A1"), MakePosition(8.4, 47.2), 1.0, 10.0);
    TestEqual(TEXT("three points retained after a second move"), Tracker.FindTrail(TEXT("A1"))->Points.Num(), 3);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackStationaryEntityTest, "IONCOMMAND.Visualization.Track.StationaryEntityNeverAccumulates", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackStationaryEntityTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    Tracker.AddSighting(TEXT("GROUND1"), MakePosition(8.5417, 47.3769), 1.0, 0.0);

    // A fixed ground station or ionosonde reports the exact same position on
    // every observation; none of these may ever add a second point.
    for (int32 Index = 1; Index <= 20; ++Index)
    {
        const EGeoTrackUpdateResult Result = Tracker.AddSighting(TEXT("GROUND1"), MakePosition(8.5417, 47.3769), 1.0, static_cast<double>(Index));
        TestEqual(TEXT("repeated identical position is reported as Stationary"), static_cast<int32>(Result), static_cast<int32>(EGeoTrackUpdateResult::Stationary));
    }
    const FGeoTrackedTrail* Trail = Tracker.FindTrail(TEXT("GROUND1"));
    TestNotNull(TEXT("entity is still tracked"), Trail);
    if (!Trail)
    {
        return false;
    }
    TestEqual(TEXT("trail never grew past the single seed point"), Trail->Points.Num(), 1);

    // Sub-threshold coordinate/GPS jitter on an otherwise fixed position must
    // not be mistaken for movement either.
    const EGeoTrackUpdateResult JitterResult = Tracker.AddSighting(TEXT("GROUND1"), MakePosition(8.54171, 47.37691), 1.0, 21.0);
    TestEqual(TEXT("tiny jitter below the threshold is still Stationary"), static_cast<int32>(JitterResult), static_cast<int32>(EGeoTrackUpdateResult::Stationary));
    TestEqual(TEXT("still a single point after jitter"), Tracker.FindTrail(TEXT("GROUND1"))->Points.Num(), 1);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackEntityCapTest, "IONCOMMAND.Visualization.Track.EntityCapEvictsLeastRecentlyUpdated", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackEntityCapTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    Tracker.MaxTrackedEntities = 3;
    Tracker.AddSighting(TEXT("E1"), MakePosition(0.0, 0.0), 1.0, 0.0);
    Tracker.AddSighting(TEXT("E2"), MakePosition(10.0, 0.0), 1.0, 1.0);
    Tracker.AddSighting(TEXT("E3"), MakePosition(20.0, 0.0), 1.0, 2.0);
    TestEqual(TEXT("three entities tracked, exactly at the cap"), Tracker.NumTrackedEntities(), 3);

    // E1 is the least recently updated; a fourth new entity must evict
    // exactly it, not one of the more recently updated entities.
    Tracker.AddSighting(TEXT("E4"), MakePosition(30.0, 0.0), 1.0, 3.0);
    TestEqual(TEXT("population stays bounded at the cap"), Tracker.NumTrackedEntities(), 3);
    TestNull(TEXT("least recently updated entity was evicted"), Tracker.FindTrail(TEXT("E1")));
    TestNotNull(TEXT("more recently updated entity survives"), Tracker.FindTrail(TEXT("E2")));
    TestNotNull(TEXT("most recently updated entity survives"), Tracker.FindTrail(TEXT("E3")));
    TestNotNull(TEXT("newly added entity is tracked"), Tracker.FindTrail(TEXT("E4")));

    // Touching E2 (a Stationary update still refreshes LastUpdateSeconds)
    // makes E3 the new least-recently-updated entity.
    Tracker.AddSighting(TEXT("E2"), MakePosition(10.0, 0.0), 1.0, 4.0);
    Tracker.AddSighting(TEXT("E5"), MakePosition(40.0, 0.0), 1.0, 5.0);
    TestNull(TEXT("now-oldest entity is evicted in its place"), Tracker.FindTrail(TEXT("E3")));
    TestNotNull(TEXT("touched entity survived the later eviction"), Tracker.FindTrail(TEXT("E2")));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackPointCapTest, "IONCOMMAND.Visualization.Track.PointCapEvictsOldestPointFirst", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackPointCapTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    Tracker.MaxPointsPerTrail = 3;
    Tracker.AddSighting(TEXT("A1"), MakePosition(0.0, 0.0), 1.0, 0.0);
    Tracker.AddSighting(TEXT("A1"), MakePosition(1.0, 0.0), 1.0, 1.0);
    Tracker.AddSighting(TEXT("A1"), MakePosition(2.0, 0.0), 1.0, 2.0);
    TestEqual(TEXT("filled to the per-trail cap"), Tracker.FindTrail(TEXT("A1"))->Points.Num(), 3);

    Tracker.AddSighting(TEXT("A1"), MakePosition(3.0, 0.0), 1.0, 3.0);
    const FGeoTrackedTrail* Trail = Tracker.FindTrail(TEXT("A1"));
    TestNotNull(TEXT("entity still tracked"), Trail);
    if (!Trail)
    {
        return false;
    }
    TestEqual(TEXT("stays at the cap instead of growing unbounded"), Trail->Points.Num(), 3);
    TestTrue(TEXT("oldest point (longitude 0) was evicted"), FMath::IsNearlyEqual(Trail->Points[0].Position.Longitude, 1.0));
    TestTrue(TEXT("newest point (longitude 3) is retained"), FMath::IsNearlyEqual(Trail->Points.Last().Position.Longitude, 3.0));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackResetTest, "IONCOMMAND.Visualization.Track.ResetClearsAllState", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackResetTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    Tracker.AddSighting(TEXT("A1"), MakePosition(0.0, 0.0), 1.0, 0.0);
    Tracker.AddSighting(TEXT("A1"), MakePosition(1.0, 0.0), 1.0, 1.0);
    Tracker.AddSighting(TEXT("A2"), MakePosition(5.0, 5.0), 1.0, 1.0);
    TestEqual(TEXT("two entities tracked before reset"), Tracker.NumTrackedEntities(), 2);

    Tracker.Reset();
    TestEqual(TEXT("no entities remain after reset"), Tracker.NumTrackedEntities(), 0);
    TestNull(TEXT("previously tracked entity is gone"), Tracker.FindTrail(TEXT("A1")));

    // The tracker must be immediately usable again after a reset, exactly
    // like the data reset it mirrors (a fresh replay or a reconnect).
    const EGeoTrackUpdateResult Result = Tracker.AddSighting(TEXT("A3"), MakePosition(9.0, 9.0), 1.0, 2.0);
    TestEqual(TEXT("tracking resumes cleanly after reset"), static_cast<int32>(Result), static_cast<int32>(EGeoTrackUpdateResult::FirstSighting));
    TestEqual(TEXT("exactly one entity tracked again"), Tracker.NumTrackedEntities(), 1);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackIgnoresEntitylessTest, "IONCOMMAND.Visualization.Track.EntitylessSightingIsIgnored", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackIgnoresEntitylessTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    const EGeoTrackUpdateResult Result = Tracker.AddSighting(FString(), MakePosition(0.0, 0.0), 1.0, 0.0);
    TestEqual(TEXT("empty entity id is Ignored"), static_cast<int32>(Result), static_cast<int32>(EGeoTrackUpdateResult::Ignored));
    TestEqual(TEXT("nothing is tracked for a one-shot message"), Tracker.NumTrackedEntities(), 0);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonTrackExpiryTest, "IONCOMMAND.Visualization.Track.ExpiredPointsDropAndStaleEntitiesAreRemoved", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonTrackExpiryTest::RunTest(const FString& Parameters)
{
    FGeoTrackTracker Tracker;
    Tracker.AddSighting(TEXT("A1"), MakePosition(0.0, 0.0), 1.0, 0.0);
    Tracker.AddSighting(TEXT("A1"), MakePosition(1.0, 0.0), 1.0, 1.0);
    TestEqual(TEXT("two points before any expiry sweep"), Tracker.FindTrail(TEXT("A1"))->Points.Num(), 2);

    // Neither point is older than the window yet: nothing should change, and
    // a brand-new single-point entity must survive an early sweep untouched.
    Tracker.AddSighting(TEXT("A2"), MakePosition(50.0, 10.0), 1.0, 1.5);
    const bool bChangedEarly = Tracker.RemoveExpiredPoints(2.0, 45.0);
    TestFalse(TEXT("nothing changes well within the age window"), bChangedEarly);
    TestNotNull(TEXT("moving entity is still tracked"), Tracker.FindTrail(TEXT("A1")));
    TestNotNull(TEXT("freshly seeded single-point entity survives the sweep"), Tracker.FindTrail(TEXT("A2")));
    TestEqual(TEXT("seed point untouched"), Tracker.FindTrail(TEXT("A2"))->Points.Num(), 1);

    // Long past the age window: A1's points expire and, having nothing left
    // to render, the entity is dropped entirely rather than left as a dead
    // stub. A2's single seed point is equally old and is removed the same
    // way once it too has aged past the window.
    const bool bChangedLate = Tracker.RemoveExpiredPoints(1000.0, 45.0);
    TestTrue(TEXT("sweeping far in the future changes state"), bChangedLate);
    TestNull(TEXT("fully-faded moving entity is removed"), Tracker.FindTrail(TEXT("A1")));
    TestNull(TEXT("aged-out single-point entity is removed"), Tracker.FindTrail(TEXT("A2")));
    TestEqual(TEXT("no entities remain"), Tracker.NumTrackedEntities(), 0);
    return true;
}

#endif
