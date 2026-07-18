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

