#include "GeoMathLibrary.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonMaidenheadTest, "IONCOMMAND.Core.Geo.Maidenhead", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonMaidenheadTest::RunTest(const FString& Parameters)
{
    FGeoPosition Position;
    TestTrue(TEXT("JN47AA parses"), UGeoMathLibrary::MaidenheadToLatLon(TEXT("JN47AA"), Position));
    TestTrue(TEXT("latitude in Switzerland"), Position.Latitude > 46.0 && Position.Latitude < 48.0);
    TestTrue(TEXT("longitude in Switzerland"), Position.Longitude > 6.0 && Position.Longitude < 10.0);
    TestFalse(TEXT("odd locator rejected"), UGeoMathLibrary::MaidenheadToLatLon(TEXT("JN4"), Position));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSphereFrameTest, "IONCOMMAND.Core.Geo.SphereFrame", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSphereFrameTest::RunTest(const FString& Parameters)
{
    // Pins the world frame to the engine sphere's texture mapping (measured
    // with Scripts/probe_sphere_uv.py): Greenwich renders at world +Y, 90E at
    // world +X. A regression here means every station drifts off its terrain.
    const FVector Greenwich = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(0.0, 0.0);
    TestTrue(TEXT("Greenwich sits on +Y"), Greenwich.Equals(FVector(0, 1, 0), 1e-6));
    const FVector East90 = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(0.0, 90.0);
    TestTrue(TEXT("90E sits on +X"), East90.Equals(FVector(1, 0, 0), 1e-6));
    const FVector NorthPole = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(90.0, 0.0);
    TestTrue(TEXT("north pole sits on +Z"), NorthPole.Equals(FVector(0, 0, 1), 1e-6));
    const FGeoPosition Roundtrip = UGeoMathLibrary::UnitSphereToLatitudeLongitude(UGeoMathLibrary::LatitudeLongitudeToUnitSphere(47.3, 8.5));
    TestTrue(TEXT("roundtrip latitude"), FMath::IsNearlyEqual(Roundtrip.Latitude, 47.3, 1e-4));
    TestTrue(TEXT("roundtrip longitude"), FMath::IsNearlyEqual(Roundtrip.Longitude, 8.5, 1e-4));
    return true;
}

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonGreatCircleTest, "IONCOMMAND.Core.Geo.GreatCircle", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonGreatCircleTest::RunTest(const FString& Parameters)
{
    FGeoPosition Zurich; Zurich.Latitude = 47.3769; Zurich.Longitude = 8.5417;
    FGeoPosition NewYork; NewYork.Latitude = 40.7128; NewYork.Longitude = -74.0060;
    TestTrue(TEXT("distance is plausible"), FMath::IsNearlyEqual(UGeoMathLibrary::GreatCircleDistanceKm(Zurich, NewYork), 6320.0, 50.0));
    const FGeoPosition Midpoint = UGeoMathLibrary::GreatCircleInterpolation(Zurich, NewYork, 0.5);
    TestTrue(TEXT("midpoint is northern Atlantic"), Midpoint.Latitude > 50.0 && Midpoint.Longitude < 0.0);
    return true;
}

#endif

