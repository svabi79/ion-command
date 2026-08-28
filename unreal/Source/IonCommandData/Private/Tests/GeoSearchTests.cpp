#include "GeoSearchSubsystem.h"
#include "Engine/GameInstance.h"
#include "HAL/PlatformTime.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

namespace
{
FGeoMessageEnvelope MakeEnvelope(const FString& MessageId, const FString& EntityId, const FString& Domain, const FString& SemanticType, const FString& Title, const FString& Primary, const FDateTime& ObservedUtc)
{
    FGeoMessageEnvelope Envelope;
    Envelope.SchemaVersion = 1;
    Envelope.MessageId = MessageId;
    Envelope.MessageType = EGeoMessageType::Observation;
    Envelope.Domain = Domain;
    Envelope.SemanticType = SemanticType;
    Envelope.EntityId = EntityId;
    Envelope.Time.ObservedUtc = ObservedUtc;
    Envelope.Geometry.Type = EGeoGeometryType::Point;
    Envelope.Geometry.Positions.Add(FGeoPosition{8.0, 47.0, 0.0});
    if (!Title.IsEmpty()) Envelope.Properties.Add(TEXT("display.title"), Title);
    if (!Primary.IsEmpty()) Envelope.Properties.Add(TEXT("display.primary"), Primary);
    return Envelope;
}
}

// Indexing/lookup: a single accepted message is findable by its display
// label and by its stable entity id, both case-insensitively, and by direct
// key lookup. Proves search reads only generic identifiers/display.*
// metadata - nothing here depends on any domain-specific field name.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchIndexLookupTest, "IONCOMMAND.Data.Search.IndexAndLookup", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchIndexLookupTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    const FDateTime Now(2026, 8, 28, 12, 0, 0);
    Search->IngestMessage(MakeEnvelope(TEXT("msg-1"), TEXT("aviation:aircraft:a1b2c3"), TEXT("aviation"), TEXT("aviation.aircraft"), TEXT("DLH441"), TEXT("FL350"), Now));

    TestEqual(TEXT("one document indexed"), Search->GetDocumentCount(), 1);

    TArray<FGeoSearchResult> ByLabel = Search->Search(TEXT("dlh441"));
    TestEqual(TEXT("label search finds one result"), ByLabel.Num(), 1);
    if (ByLabel.Num() == 1)
    {
        TestEqual(TEXT("label search key"), ByLabel[0].Key, FString(TEXT("aviation:aircraft:a1b2c3")));
    }

    TArray<FGeoSearchResult> ById = Search->Search(TEXT("A1B2C3"));
    TestEqual(TEXT("entity id substring search finds one result"), ById.Num(), 1);

    const FGeoSearchResult* Direct = Search->FindByKey(TEXT("aviation:aircraft:a1b2c3"));
    TestNotNull(TEXT("direct key lookup succeeds"), Direct);

    TestEqual(TEXT("unrelated query finds nothing"), Search->Search(TEXT("ZZZ999")).Num(), 0);
    TestEqual(TEXT("empty query returns nothing rather than the whole index"), Search->Search(TEXT("")).Num(), 0);
    return true;
}

// Entity grouping: repeated observations of the same stable entity id
// collapse into one document carrying the latest state and a running
// observation count, while messages without an entity id (relationships,
// one-shot observations) always stay separately discoverable.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchEntityGroupingTest, "IONCOMMAND.Data.Search.EntityGrouping", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchEntityGroupingTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    Search->IngestMessage(MakeEnvelope(TEXT("fix-1"), TEXT("aviation:aircraft:dead01"), TEXT("aviation"), TEXT("aviation.aircraft"), TEXT("SWR100"), TEXT("FL340"), T0));
    Search->IngestMessage(MakeEnvelope(TEXT("fix-2"), TEXT("aviation:aircraft:dead01"), TEXT("aviation"), TEXT("aviation.aircraft"), TEXT("SWR100"), TEXT("FL341"), T0 + FTimespan::FromSeconds(10)));
    Search->IngestMessage(MakeEnvelope(TEXT("fix-3"), TEXT("aviation:aircraft:dead01"), TEXT("aviation"), TEXT("aviation.aircraft"), TEXT("SWR100"), TEXT("FL342"), T0 + FTimespan::FromSeconds(20)));

    TestEqual(TEXT("repeated observations of one entity collapse into one document"), Search->GetDocumentCount(), 1);
    const FGeoSearchResult* Result = Search->FindByKey(TEXT("aviation:aircraft:dead01"));
    if (TestNotNull(TEXT("grouped document exists"), Result))
    {
        TestEqual(TEXT("observation count reflects all three fixes"), Result->ObservationCount, 3);
        TestTrue(TEXT("marked as a grouped entity"), Result->bIsGrouped);
        TestEqual(TEXT("subtitle reflects the latest fix, not the first"), Result->DisplaySubtitle, FString(TEXT("FL342")));
    }

    FGeoMessageEnvelope OneShotA = MakeEnvelope(TEXT("strike-1"), FString(), TEXT("weather"), TEXT("weather.lightning"), TEXT("Strike"), FString(), T0);
    FGeoMessageEnvelope OneShotB = MakeEnvelope(TEXT("strike-2"), FString(), TEXT("weather"), TEXT("weather.lightning"), TEXT("Strike"), FString(), T0 + FTimespan::FromSeconds(1));
    Search->IngestMessage(OneShotA);
    Search->IngestMessage(OneShotB);
    TestEqual(TEXT("messages without an entity id stay separately discoverable"), Search->GetDocumentCount(), 3);
    return true;
}

// Bounded growth: the index never grows past its configured cap, evicting
// the oldest-inserted documents first (matching UGeoDataSubsystem's own
// window-cap eviction policy for the active message array).
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchBoundedGrowthTest, "IONCOMMAND.Data.Search.BoundedGrowth", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchBoundedGrowthTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    const int32 OverCap = 20200; // past the 20,000 production default
    for (int32 Index = 0; Index < OverCap; ++Index)
    {
        Search->IngestMessage(MakeEnvelope(FString::Printf(TEXT("evt-%06d"), Index), FString(), TEXT("weather"), TEXT("weather.lightning"), TEXT("Strike"), FString(), T0 + FTimespan::FromMilliseconds(Index)));
    }
    TestTrue(TEXT("document count never exceeds the bounded cap"), Search->GetDocumentCount() <= 20000);
    TestNull(TEXT("earliest document was evicted"), Search->FindByKey(TEXT("evt-000000")));
    TestNotNull(TEXT("most recently inserted document survives"), Search->FindByKey(FString::Printf(TEXT("evt-%06d"), OverCap - 1)));
    return true;
}

// Timeline-aware expiry: a document with an explicit validUntil expires the
// moment the timeline passes it; a document without one falls back to the
// same active-window cutoff UGeoDataSubsystem uses, measured from its own
// observed time - and nothing is purged before its time.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchExpiryTest, "IONCOMMAND.Data.Search.TimelineExpiry", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchExpiryTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    const FDateTime T0(2026, 8, 28, 12, 0, 0);

    FGeoMessageEnvelope WithValidity = MakeEnvelope(TEXT("q-1"), TEXT("geophysics:quake:one"), TEXT("geophysics"), TEXT("geophysics.earthquake"), TEXT("M 4.7"), FString(), T0);
    WithValidity.Time.bHasValidUntil = true;
    WithValidity.Time.ValidUntilUtc = T0 + FTimespan::FromMinutes(10);
    Search->IngestMessage(WithValidity);
    Search->IngestMessage(MakeEnvelope(TEXT("s-1"), FString(), TEXT("weather"), TEXT("weather.lightning"), TEXT("Strike"), FString(), T0));

    TestEqual(TEXT("both documents indexed"), Search->GetDocumentCount(), 2);

    Search->PurgeExpired(T0 + FTimespan::FromMinutes(5));
    TestEqual(TEXT("nothing expired yet"), Search->GetDocumentCount(), 2);

    Search->PurgeExpired(T0 + FTimespan::FromMinutes(11));
    TestEqual(TEXT("validUntil document expired, fallback-window document has not"), Search->GetDocumentCount(), 1);
    TestNull(TEXT("expired document is gone"), Search->FindByKey(TEXT("geophysics:quake:one")));

    Search->PurgeExpired(T0 + FTimespan::FromMinutes(16)); // past the 900s default active window
    TestEqual(TEXT("fallback-window document expires once past the active window"), Search->GetDocumentCount(), 0);
    return true;
}

// Reset (replay start / return-to-live) clears the index so live state can
// never leak into a paused or replayed view.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchResetTest, "IONCOMMAND.Data.Search.Reset", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchResetTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    Search->IngestMessage(MakeEnvelope(TEXT("m-1"), TEXT("aviation:aircraft:aaaaaa"), TEXT("aviation"), TEXT("aviation.aircraft"), TEXT("AAA1"), FString(), FDateTime(2026, 8, 28, 12, 0, 0)));
    TestEqual(TEXT("indexed before reset"), Search->GetDocumentCount(), 1);
    Search->Reset();
    TestEqual(TEXT("empty after reset"), Search->GetDocumentCount(), 0);
    TestEqual(TEXT("stale query returns nothing after reset"), Search->Search(TEXT("AAA1")).Num(), 0);
    return true;
}

// Query-latency proof at the roadmap's 50,000-message target window. Logs
// the measured latency so it shows up in the automation run's output.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonSearchQueryLatencyTest, "IONCOMMAND.Data.Search.QueryLatencyAt50000", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonSearchQueryLatencyTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoSearchSubsystem* Search = NewObject<UGeoSearchSubsystem>(GameInstance);
    Search->SetMaxDocumentsForTesting(60000); // headroom above the 50,000 target window
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    constexpr int32 TargetCount = 50000;
    for (int32 Index = 0; Index < TargetCount; ++Index)
    {
        const FString EntityId = FString::Printf(TEXT("aviation:aircraft:%06x"), Index);
        const FString Title = FString::Printf(TEXT("FLT%04d"), Index % 9000);
        Search->IngestMessage(MakeEnvelope(FString::Printf(TEXT("evt-%06d"), Index), EntityId, TEXT("aviation"), TEXT("aviation.aircraft"), Title, TEXT("FL350"), T0 + FTimespan::FromMilliseconds(Index)));
    }
    TestEqual(TEXT("index reaches the 50,000-document target window"), Search->GetDocumentCount(), TargetCount);

    const double StartSeconds = FPlatformTime::Seconds();
    const TArray<FGeoSearchResult> Results = Search->Search(TEXT("FLT1234"));
    const double ElapsedMs = (FPlatformTime::Seconds() - StartSeconds) * 1000.0;

    TestTrue(TEXT("query matches at least one indexed flight"), Results.Num() > 0);
    UE_LOG(LogTemp, Display, TEXT("IONCOMMAND search query-latency at %d documents: %.3f ms"), TargetCount, ElapsedMs);
    TestTrue(FString::Printf(TEXT("query latency at 50,000 documents stays under 100 ms (measured %.3f ms)"), ElapsedMs), ElapsedMs < 100.0);
    return true;
}

#endif
