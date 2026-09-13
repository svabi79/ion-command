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

IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonEnvelopeParserPolygonTest, "IONCOMMAND.Data.CanonicalEnvelope.Polygon", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonEnvelopeParserPolygonTest::RunTest(const FString& Parameters)
{
    const FString Json = TEXT(R"({"schemaVersion":1,"messageId":"cone","messageType":"area","domain":"weather","semanticType":"weather.storm.cone","entityId":"weather:storm:ep142026","source":{"pluginId":"nhc","instanceId":"test"},"time":{"observedUtc":"2026-09-13T03:00:00Z","receivedUtc":"2026-09-13T03:00:01Z","validFromUtc":"2026-09-13T03:00:00Z","processingUtc":"2026-09-13T03:00:01Z"},"geometry":{"type":"Polygon","crs":"EPSG:4326","coordinates":[[[-130.5,17.9],[-129.0,18.0],[-128.5,20.0],[-131.0,19.5],[-130.5,17.9]]]},"properties":{"display.title":"Tropical Storm Norbert"}})");
    FGeoMessageEnvelope Envelope;
    FString Error;
    TestTrue(TEXT("polygon envelope parses"), FGeoEnvelopeJsonParser::Parse(Json, Envelope, Error));
    TestEqual(TEXT("message type"), Envelope.MessageType, EGeoMessageType::Area);
    TestEqual(TEXT("geometry"), Envelope.Geometry.Type, EGeoGeometryType::Polygon);
    TestEqual(TEXT("ring count"), Envelope.Geometry.NumRings(), 1);
    TestEqual(TEXT("positions"), Envelope.Geometry.Positions.Num(), 5);
    TArray<FGeoPosition> Ring;
    TestTrue(TEXT("ring extract"), Envelope.Geometry.GetRing(0, Ring));
    TestEqual(TEXT("ring length"), Ring.Num(), 5);
    TestEqual(TEXT("first lon"), Ring[0].Longitude, -130.5);
    return true;
}

#endif

