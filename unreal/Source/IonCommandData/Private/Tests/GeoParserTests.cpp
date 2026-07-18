#include "GeoEnvelopeJsonParser.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonEnvelopeParserTest, "IONCOMMAND.Data.CanonicalEnvelope", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonEnvelopeParserTest::RunTest(const FString& Parameters)
{
    const FString Json = TEXT(R"({"schemaVersion":1,"messageId":"one","messageType":"relationship","domain":"hamradio","semanticType":"radio.reception","source":{"pluginId":"mock.radio","instanceId":"test"},"time":{"observedUtc":"2026-07-18T18:00:00Z","receivedUtc":"2026-07-18T18:00:01Z","validFromUtc":"2026-07-18T18:00:00Z","processingUtc":"2026-07-18T18:00:01Z"},"geometry":{"type":"GreatCircle","crs":"EPSG:4326","coordinates":[[8.0,47.0],[-74.0,41.0]]},"properties":{"band":"20m","snrDb":-11}})");
    FGeoMessageEnvelope Envelope;
    FString Error;
    TestTrue(TEXT("valid envelope parses"), FGeoEnvelopeJsonParser::Parse(Json, Envelope, Error));
    TestEqual(TEXT("geometry"), Envelope.Geometry.Type, EGeoGeometryType::GreatCircle);
    TestEqual(TEXT("positions"), Envelope.Geometry.Positions.Num(), 2);
    TestEqual(TEXT("band"), Envelope.Properties.FindRef(TEXT("band")), FString(TEXT("20m")));
    return true;
}

#endif

