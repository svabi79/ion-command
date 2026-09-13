#include "GeoArcLayerActor.h"
#include "GeoAreaLayerActor.h"
#include "GeoMathLibrary.h"
#include "GeoPathLayerActor.h"
#include "GeoPointLayerActor.h"
#include "Misc/AutomationTest.h"
#include "UObject/Package.h"

#if WITH_DEV_AUTOMATION_TESTS

// Actor-level checks that the real Submit()/Tick() call sequence produces
// the render-population counts the refactor promises, on top of the
// isolated index-arithmetic proof in GeoRenderSlotMathTests.cpp and the
// real-component proof in GeoRenderSlotComponentTests.cpp.
//
// These construct the actors with NewObject() directly (no SpawnActor, no
// UWorld, no BeginPlay()) rather than spinning up a full world context:
// every code path exercised here (Submit(), capacity trim, SetBandFocus,
// SetDomainVisible, and a directly-invoked Tick()) only touches
// constructor-initialized state and is written to tolerate GetWorld()
// returning null (the same guard the runtime already needs for an actor
// ticked before its first BeginPlay). This deliberately does NOT exercise
// time-based expiry or dead-reckoning coasting, both of which need a real
// advancing world clock - see the test-run report for what that leaves
// unverified.

namespace
{
    FGeoMessageEnvelope MakeArcMessage(int32 Index)
    {
        FGeoMessageEnvelope Message;
        Message.SchemaVersion = 1;
        Message.MessageId = FString::Printf(TEXT("arc-%d"), Index);
        Message.MessageType = EGeoMessageType::Relationship;
        Message.SemanticType = TEXT("test.arc");
        Message.Geometry.Type = EGeoGeometryType::GreatCircle;
        FGeoPosition From;
        From.Latitude = FMath::Fmod(Index * 3.7, 80.0) - 40.0;
        From.Longitude = FMath::Fmod(Index * 5.3, 340.0) - 170.0;
        FGeoPosition To;
        To.Latitude = FMath::Fmod(Index * 2.1 + 10.0, 80.0) - 40.0;
        To.Longitude = FMath::Fmod(Index * 7.9 + 20.0, 340.0) - 170.0;
        Message.Geometry.Positions.Add(From);
        Message.Geometry.Positions.Add(To);
        return Message;
    }

    FGeoMessageEnvelope MakePointMessage(int32 Index, const FString& Domain)
    {
        FGeoMessageEnvelope Message;
        Message.SchemaVersion = 1;
        Message.MessageId = FString::Printf(TEXT("point-msg-%d"), Index);
        Message.EntityId = FString::Printf(TEXT("entity-%d"), Index);
        Message.MessageType = EGeoMessageType::Entity;
        Message.Domain = Domain;
        Message.SemanticType = TEXT("test.point");
        Message.Geometry.Type = EGeoGeometryType::Point;
        FGeoPosition Position;
        Position.Latitude = FMath::Fmod(Index * 3.7, 80.0) - 40.0;
        Position.Longitude = FMath::Fmod(Index * 5.3, 340.0) - 170.0;
        Message.Geometry.Positions.Add(Position);
        return Message;
    }

    FGeoMessageEnvelope MakeAreaMessage(int32 Index)
    {
        FGeoMessageEnvelope Message;
        Message.SchemaVersion = 1;
        Message.MessageId = FString::Printf(TEXT("area-%d"), Index);
        Message.EntityId = FString::Printf(TEXT("area-entity-%d"), Index);
        Message.MessageType = EGeoMessageType::Area;
        Message.SemanticType = TEXT("test.area");
        Message.Geometry.Type = EGeoGeometryType::Polygon;
        const double Lon = FMath::Fmod(Index * 5.3, 340.0) - 170.0;
        const double Lat = FMath::Fmod(Index * 3.7, 80.0) - 40.0;
        const double CornerLon[] = {Lon, Lon + 2.0, Lon + 2.0, Lon, Lon};
        const double CornerLat[] = {Lat, Lat, Lat + 2.0, Lat + 2.0, Lat};
        for (int32 Corner = 0; Corner < 5; ++Corner)
        {
            FGeoPosition Position;
            Position.Longitude = CornerLon[Corner];
            Position.Latitude = CornerLat[Corner];
            Message.Geometry.Positions.Add(Position);
        }
        Message.Geometry.RingLengths.Add(5);
        return Message;
    }
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoArcLayerCapacityTrimIsIncrementalTest, "IONCOMMAND.Visualization.ArcLayer.CapacityTrimIsIncremental", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoArcLayerCapacityTrimIsIncrementalTest::RunTest(const FString& Parameters)
{
    AGeoArcLayerActor* Actor = NewObject<AGeoArcLayerActor>(GetTransientPackage());
    Actor->MaxVisibleArcs = 50;

    const int32 SubmitCount = Actor->MaxVisibleArcs * 3;
    for (int32 Index = 0; Index < SubmitCount; ++Index)
    {
        Actor->Submit(MakeArcMessage(Index));
    }

    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestTrue(TEXT("tracked count stays at or under the capacity bound"), Stats.TrackedItems <= Actor->MaxVisibleArcs);
    TestTrue(TEXT("capacity eviction actually fired for this overload"), Stats.CapacityEvictions > 0);
    TestEqual(TEXT("no full rebuild occurred - only Submit()/trim, no filter change"), Stats.FullRebuilds, (int64)0);
    TestEqual(TEXT("rendered instance count matches tracked arcs * SegmentsPerArc"), Stats.RenderedInstances, Stats.TrackedItems * Actor->SegmentsPerArc);
    TestEqual(TEXT("every submit produced exactly one incremental insert"), Stats.IncrementalInserts, (int64)SubmitCount);
    TestEqual(TEXT("removals are all accounted for as capacity evictions (no expiry ran)"), Stats.IncrementalRemovals, Stats.CapacityEvictions);

    TArray<FGeoPaletteBreakdownEntry> Breakdown;
    Actor->GetPaletteBreakdown(Breakdown);
    int32 BreakdownSum = 0;
    for (const FGeoPaletteBreakdownEntry& Entry : Breakdown) BreakdownSum += Entry.Count;
    TestEqual(TEXT("palette breakdown sums to tracked arc count"), BreakdownSum, Stats.TrackedItems);

    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoArcLayerBandFocusIsVisibilityOnlyTest, "IONCOMMAND.Visualization.ArcLayer.BandFocusDoesNotChurnInstances", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoArcLayerBandFocusIsVisibilityOnlyTest::RunTest(const FString& Parameters)
{
    AGeoArcLayerActor* Actor = NewObject<AGeoArcLayerActor>(GetTransientPackage());
    for (int32 Index = 0; Index < 40; ++Index) Actor->Submit(MakeArcMessage(Index));

    const FGeoRenderLayerStatistics Before = Actor->GetRenderStatistics();
    Actor->SetBandFocus(0);
    Actor->SetBandFocus(INDEX_NONE);
    const FGeoRenderLayerStatistics After = Actor->GetRenderStatistics();

    TestEqual(TEXT("band focus never inserts"), After.IncrementalInserts, Before.IncrementalInserts);
    TestEqual(TEXT("band focus never removes"), After.IncrementalRemovals, Before.IncrementalRemovals);
    TestEqual(TEXT("band focus never triggers a rebuild"), After.FullRebuilds, Before.FullRebuilds);
    TestEqual(TEXT("instance count unchanged by pure visibility toggling"), After.RenderedInstances, Before.RenderedInstances);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPointLayerCapacityTrimIsIncrementalTest, "IONCOMMAND.Visualization.PointLayer.CapacityTrimIsIncremental", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPointLayerCapacityTrimIsIncrementalTest::RunTest(const FString& Parameters)
{
    AGeoPointLayerActor* Actor = NewObject<AGeoPointLayerActor>(GetTransientPackage());
    Actor->MaxVisiblePoints = 50;

    const int32 SubmitCount = Actor->MaxVisiblePoints * 3;
    for (int32 Index = 0; Index < SubmitCount; ++Index)
    {
        Actor->Submit(MakePointMessage(Index, TEXT("test")));
    }

    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestTrue(TEXT("tracked count stays at or under the capacity bound"), Stats.TrackedItems <= Actor->MaxVisiblePoints);
    TestTrue(TEXT("capacity eviction actually fired for this overload"), Stats.CapacityEvictions > 0);
    TestEqual(TEXT("points never perform a full rebuild, even under capacity pressure"), Stats.FullRebuilds, (int64)0);
    TestEqual(TEXT("one render instance per tracked point (no filters active)"), Stats.RenderedInstances, Stats.TrackedItems);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPointLayerStableEntityDedupTest, "IONCOMMAND.Visualization.PointLayer.RepeatedEntitySightingDoesNotDuplicate", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPointLayerStableEntityDedupTest::RunTest(const FString& Parameters)
{
    AGeoPointLayerActor* Actor = NewObject<AGeoPointLayerActor>(GetTransientPackage());
    for (int32 Sighting = 0; Sighting < 10; ++Sighting)
    {
        FGeoMessageEnvelope Message = MakePointMessage(0, TEXT("test")); // same EntityId every time
        Message.MessageId = FString::Printf(TEXT("sighting-%d"), Sighting);
        Actor->Submit(Message);
    }
    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestEqual(TEXT("repeated sightings of one entity track as a single point"), Stats.TrackedItems, 1);
    TestEqual(TEXT("repeated sightings of one entity render as a single instance"), Stats.RenderedInstances, 1);
    TestEqual(TEXT("only the first sighting was a new insert"), Stats.IncrementalInserts, (int64)1);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPointLayerDomainToggleIsIncrementalTest, "IONCOMMAND.Visualization.PointLayer.DomainToggleIsIncremental", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPointLayerDomainToggleIsIncrementalTest::RunTest(const FString& Parameters)
{
    AGeoPointLayerActor* Actor = NewObject<AGeoPointLayerActor>(GetTransientPackage());
    constexpr int32 AlphaCount = 30;
    constexpr int32 BetaCount = 20;
    for (int32 Index = 0; Index < AlphaCount; ++Index) Actor->Submit(MakePointMessage(Index, TEXT("alpha")));
    for (int32 Index = 0; Index < BetaCount; ++Index) Actor->Submit(MakePointMessage(1000 + Index, TEXT("beta")));

    const FGeoRenderLayerStatistics Initial = Actor->GetRenderStatistics();
    TestEqual(TEXT("both domains start fully rendered"), Initial.RenderedInstances, AlphaCount + BetaCount);
    TestEqual(TEXT("both domains start fully tracked"), Initial.TrackedItems, AlphaCount + BetaCount);

    // Hiding "alpha" only sets a pending flag; the actual instance removal
    // happens on the next Tick(), called directly here since this
    // world-less actor is never ticked by an engine scheduler.
    Actor->SetDomainVisible(TEXT("alpha"), false);
    Actor->Tick(0.0f);

    const FGeoRenderLayerStatistics AfterHide = Actor->GetRenderStatistics();
    TestEqual(TEXT("hiding alpha removes exactly its instances"), AfterHide.RenderedInstances, BetaCount);
    TestEqual(TEXT("hiding alpha does not drop the tracked entities"), AfterHide.TrackedItems, AlphaCount + BetaCount);
    TestEqual(TEXT("hiding a domain is never a full rebuild"), AfterHide.FullRebuilds, (int64)0);
    TestTrue(TEXT("hiding alpha performed incremental removals"), AfterHide.IncrementalRemovals >= AlphaCount);

    Actor->SetDomainVisible(TEXT("alpha"), true);
    Actor->Tick(0.0f);

    const FGeoRenderLayerStatistics AfterShow = Actor->GetRenderStatistics();
    TestEqual(TEXT("re-showing alpha restores every instance"), AfterShow.RenderedInstances, AlphaCount + BetaCount);
    TestEqual(TEXT("re-showing a domain is never a full rebuild"), AfterShow.FullRebuilds, (int64)0);
    TestEqual(TEXT("tracked count is unaffected throughout"), AfterShow.TrackedItems, AlphaCount + BetaCount);

    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoAreaLayerEntityReplaceIsUpdateTest, "IONCOMMAND.Visualization.AreaLayer.RepeatedEntityReplaces", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoAreaLayerEntityReplaceIsUpdateTest::RunTest(const FString& Parameters)
{
    AGeoAreaLayerActor* Actor = NewObject<AGeoAreaLayerActor>(GetTransientPackage());
    for (int32 Sighting = 0; Sighting < 6; ++Sighting)
    {
        FGeoMessageEnvelope Message = MakeAreaMessage(0);
        Message.MessageId = FString::Printf(TEXT("area-sighting-%d"), Sighting);
        Actor->Submit(Message);
    }
    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestEqual(TEXT("one entity keeps a single area"), Stats.TrackedItems, 1);
    TestEqual(TEXT("first submit is the only insert"), Stats.IncrementalInserts, (int64)1);
    TestEqual(TEXT("later submits are in-place updates"), Stats.IncrementalUpdates, (int64)5);
    TestEqual(TEXT("entity replace is not a full rebuild"), Stats.FullRebuilds, (int64)0);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPathLayerEntityReplaceIsUpdateTest, "IONCOMMAND.Visualization.PathLayer.RepeatedEntityReplaces", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPathLayerEntityReplaceIsUpdateTest::RunTest(const FString& Parameters)
{
    AGeoPathLayerActor* Actor = NewObject<AGeoPathLayerActor>(GetTransientPackage());
    for (int32 Sighting = 0; Sighting < 6; ++Sighting)
    {
        FGeoMessageEnvelope Message;
        Message.MessageId = FString::Printf(TEXT("path-sighting-%d"), Sighting);
        Message.EntityId = TEXT("path-entity-0");
        Message.MessageType = EGeoMessageType::Relationship;
        Message.SemanticType = TEXT("test.path");
        Message.Geometry.Type = EGeoGeometryType::LineString;
        FGeoPosition From; From.Longitude = -5.0; From.Latitude = 36.0;
        FGeoPosition Mid; Mid.Longitude = 0.0; Mid.Latitude = 42.0;
        FGeoPosition To; To.Longitude = 5.0; To.Latitude = 37.0;
        Message.Geometry.Positions.Add(From);
        Message.Geometry.Positions.Add(Mid);
        Message.Geometry.Positions.Add(To);
        Actor->Submit(Message);
    }
    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestEqual(TEXT("one entity keeps a single path"), Stats.TrackedItems, 1);
    TestEqual(TEXT("first submit is the only insert"), Stats.IncrementalInserts, (int64)1);
    TestEqual(TEXT("later submits are in-place updates"), Stats.IncrementalUpdates, (int64)5);
    TestEqual(TEXT("entity replace is not a full rebuild"), Stats.FullRebuilds, (int64)0);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPathLayerCapacityTrimIsIncrementalTest, "IONCOMMAND.Visualization.PathLayer.CapacityTrimIsIncremental", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPathLayerCapacityTrimIsIncrementalTest::RunTest(const FString& Parameters)
{
    AGeoPathLayerActor* Actor = NewObject<AGeoPathLayerActor>(GetTransientPackage());
    Actor->MaxVisiblePaths = 8;
    const int32 SubmitCount = Actor->MaxVisiblePaths * 3;
    for (int32 Index = 0; Index < SubmitCount; ++Index)
    {
        FGeoMessageEnvelope Message;
        Message.MessageId = FString::Printf(TEXT("path-%d"), Index);
        Message.EntityId = FString::Printf(TEXT("path-entity-%d"), Index);
        Message.MessageType = EGeoMessageType::Relationship;
        Message.SemanticType = TEXT("test.path");
        Message.Geometry.Type = EGeoGeometryType::LineString;
        FGeoPosition From; From.Longitude = -5.0 + Index; From.Latitude = 36.0;
        FGeoPosition To; To.Longitude = 5.0 + Index; To.Latitude = 37.0;
        Message.Geometry.Positions.Add(From);
        Message.Geometry.Positions.Add(To);
        Actor->Submit(Message);
    }
    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestTrue(TEXT("tracked count stays at or under the capacity bound"), Stats.TrackedItems <= Actor->MaxVisiblePaths);
    TestTrue(TEXT("capacity eviction fired"), Stats.CapacityEvictions > 0);
    TestEqual(TEXT("no full rebuild under capacity pressure"), Stats.FullRebuilds, (int64)0);
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPathLayerRayPickHitsSubmittedRouteTest, "IONCOMMAND.Visualization.PathLayer.RayPickHitsSubmittedRoute", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPathLayerRayPickHitsSubmittedRouteTest::RunTest(const FString& Parameters)
{
    AGeoPathLayerActor* Actor = NewObject<AGeoPathLayerActor>(GetTransientPackage());
    FGeoMessageEnvelope Message;
    Message.MessageId = TEXT("cable-demo");
    Message.EntityId = TEXT("geography:cable:demo");
    Message.Domain = TEXT("geography");
    Message.MessageType = EGeoMessageType::Relationship;
    Message.SemanticType = TEXT("geography.cable");
    Message.Geometry.Type = EGeoGeometryType::LineString;
    FGeoPosition From; From.Longitude = -5.0; From.Latitude = 36.0;
    FGeoPosition To; To.Longitude = 5.0; To.Latitude = 37.0;
    Message.Geometry.Positions.Add(From);
    Message.Geometry.Positions.Add(To);
    Message.Properties.Add(TEXT("display.title"), TEXT("Demo Cable"));
    Message.Properties.Add(TEXT("display.primary"), TEXT("in service · regional  //  900 km"));
    Message.Properties.Add(TEXT("display.secondary"), TEXT("Cadiz  //  Algiers"));
    Actor->Submit(Message);

    const FGeoPosition Mid = UGeoMathLibrary::GreatCircleInterpolation(From, To, 0.5);
    const FVector Target = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Mid.Latitude, Mid.Longitude) * (Actor->GlobeRadius + 4.0);
    const FVector Origin = Target * 3.0;
    FGeoMessageEnvelope Hit;
    const bool bFound = Actor->FindClosestMessageToRay(Origin, (Target - Origin).GetSafeNormal(), 10000.0, 32.0, Hit);
    TestTrue(TEXT("ray aimed at the route hits it"), bFound);
    TestEqual(TEXT("pick returns the submitted cable"), Hit.MessageId, FString(TEXT("cable-demo")));
    TestEqual(TEXT("tooltip title is the cable name"), Hit.Properties.FindRef(TEXT("display.title")), FString(TEXT("Demo Cable")));
    TestEqual(TEXT("tooltip primary keeps class and km"), Hit.Properties.FindRef(TEXT("display.primary")), FString(TEXT("in service · regional  //  900 km")));
    TestEqual(TEXT("tooltip secondary keeps landing names"), Hit.Properties.FindRef(TEXT("display.secondary")), FString(TEXT("Cadiz  //  Algiers")));

    const FVector Away = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(-40.0, 80.0) * (Actor->GlobeRadius + 4.0);
    const FVector AwayOrigin = Away * 3.0;
    FGeoMessageEnvelope Miss;
    TestFalse(TEXT("ray aimed at empty ocean misses"), Actor->FindClosestMessageToRay(AwayOrigin, (Away - AwayOrigin).GetSafeNormal(), 10000.0, 32.0, Miss));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoPathLayerRoleFiltersCartographyTest, "IONCOMMAND.Visualization.PathLayer.RoleFiltersCartography", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoPathLayerRoleFiltersCartographyTest::RunTest(const FString& Parameters)
{
    auto MakeLine = [](const FString& SemanticType)
    {
        FGeoMessageEnvelope Message;
        Message.MessageId = SemanticType;
        Message.EntityId = SemanticType;
        Message.SemanticType = SemanticType;
        Message.Geometry.Type = EGeoGeometryType::LineString;
        FGeoPosition From; From.Longitude = -5.0; From.Latitude = 36.0;
        FGeoPosition To; To.Longitude = 5.0; To.Latitude = 37.0;
        Message.Geometry.Positions.Add(From);
        Message.Geometry.Positions.Add(To);
        return Message;
    };
    AGeoPathLayerActor* Cables = NewObject<AGeoPathLayerActor>(GetTransientPackage());
    AGeoPathLayerActor* Borders = NewObject<AGeoPathLayerActor>(GetTransientPackage());
    Borders->Role = EGeoPathLayerRole::Cartography;
    TestTrue(TEXT("cable role accepts a cable"), Cables->Supports(MakeLine(TEXT("geography.cable"))));
    TestFalse(TEXT("cable role rejects a border"), Cables->Supports(MakeLine(TEXT("geography.border"))));
    TestTrue(TEXT("cartography role accepts a border"), Borders->Supports(MakeLine(TEXT("geography.border"))));
    TestTrue(TEXT("cartography role accepts a river"), Borders->Supports(MakeLine(TEXT("geography.river"))));
    TestFalse(TEXT("cartography role rejects a cable"), Borders->Supports(MakeLine(TEXT("geography.cable"))));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoAreaLayerCapacityTrimIsIncrementalTest, "IONCOMMAND.Visualization.AreaLayer.CapacityTrimIsIncremental", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoAreaLayerCapacityTrimIsIncrementalTest::RunTest(const FString& Parameters)
{
    AGeoAreaLayerActor* Actor = NewObject<AGeoAreaLayerActor>(GetTransientPackage());
    Actor->MaxVisibleAreas = 8;
    const int32 SubmitCount = Actor->MaxVisibleAreas * 3;
    for (int32 Index = 0; Index < SubmitCount; ++Index)
    {
        Actor->Submit(MakeAreaMessage(Index));
    }
    const FGeoRenderLayerStatistics Stats = Actor->GetRenderStatistics();
    TestTrue(TEXT("tracked count stays at or under the capacity bound"), Stats.TrackedItems <= Actor->MaxVisibleAreas);
    TestTrue(TEXT("capacity eviction fired"), Stats.CapacityEvictions > 0);
    TestEqual(TEXT("no full rebuild under capacity pressure"), Stats.FullRebuilds, (int64)0);
    return true;
}

#endif
