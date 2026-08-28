#include "GeoWatchSubsystem.h"
#include "Engine/GameInstance.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

namespace
{
// Shape mirrors a real radio.reception relationship (see
// collector/internal/plugins/domains/hamradio/domain.go): two opaque entity
// ids plus generic display.from/display.to metadata. Using a realistic
// fixture - rather than an abstract placeholder - is what proves the
// motivating end-to-end case actually works; the watch subsystem itself
// never reads a domain-specific field name, only entity ids and generic
// "display."-prefixed properties.
FGeoMessageEnvelope MakeRelationship(const FString& MessageId, const FString& From, const FString& To, const FDateTime& ObservedUtc)
{
    FGeoMessageEnvelope Envelope;
    Envelope.SchemaVersion = 1;
    Envelope.MessageId = MessageId;
    Envelope.MessageType = EGeoMessageType::Relationship;
    Envelope.Domain = TEXT("hamradio");
    Envelope.SemanticType = TEXT("radio.reception");
    Envelope.FromEntityId = TEXT("hamradio:station:") + From;
    Envelope.ToEntityId = TEXT("hamradio:receiver:") + To;
    Envelope.Time.ObservedUtc = ObservedUtc;
    Envelope.Geometry.Type = EGeoGeometryType::GreatCircle;
    Envelope.Geometry.Positions.Add(FGeoPosition{8.0, 47.0, 0.0});
    Envelope.Geometry.Positions.Add(FGeoPosition{-74.0, 41.0, 0.0});
    Envelope.Properties.Add(TEXT("display.title"), TEXT("Observed Link"));
    Envelope.Properties.Add(TEXT("display.from"), From);
    Envelope.Properties.Add(TEXT("display.to"), To);
    return Envelope;
}
}

// Watch list bookkeeping: add, case-insensitive dedupe, blank rejection,
// remove.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonWatchListManagementTest, "IONCOMMAND.Data.Watch.ListManagement", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonWatchListManagementTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoWatchSubsystem* Watch = NewObject<UGeoWatchSubsystem>(GameInstance);
    TestTrue(TEXT("first add succeeds"), Watch->AddWatch(TEXT("N0CALL")));
    TestFalse(TEXT("case-insensitive duplicate is rejected"), Watch->AddWatch(TEXT("n0call")));
    TestFalse(TEXT("blank query is rejected"), Watch->AddWatch(TEXT("   ")));
    TestEqual(TEXT("exactly one watch stored"), Watch->GetWatches().Num(), 1);
    TestEqual(TEXT("query is normalised to upper case"), Watch->GetWatches()[0].Query, FString(TEXT("N0CALL")));

    Watch->RemoveWatch(TEXT("n0call"));
    TestEqual(TEXT("watch removed"), Watch->GetWatches().Num(), 0);
    return true;
}

// The motivating end-to-end case: the operator's own station identifier is
// on the watchlist, and a remote station reporting hearing it produces an
// alert recording what matched, which watch, and the observation time.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonWatchMatchingTest, "IONCOMMAND.Data.Watch.OwnStationAlert", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonWatchMatchingTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoWatchSubsystem* Watch = NewObject<UGeoWatchSubsystem>(GameInstance);
    Watch->AddWatch(TEXT("N0CALL"));
    const FDateTime T0(2026, 8, 28, 12, 0, 0);

    const FGeoMessageEnvelope Heard = MakeRelationship(TEXT("spot-1"), TEXT("N0CALL"), TEXT("REMOTE1"), T0);
    Watch->IngestMessage(Heard, /*bTimelineIsLive=*/true);

    TestEqual(TEXT("one alert recorded"), Watch->GetAlerts().Num(), 1);
    TestEqual(TEXT("unseen count reflects the new alert"), Watch->GetUnseenCount(), 1);
    if (Watch->GetAlerts().Num() == 1)
    {
        TestEqual(TEXT("alert records which watch matched"), Watch->GetAlerts()[0].WatchQuery, FString(TEXT("N0CALL")));
        TestEqual(TEXT("alert keeps the observation time"), Watch->GetAlerts()[0].ObservedUtc, T0);
        TestFalse(TEXT("new alert starts unseen"), Watch->GetAlerts()[0].bSeen);
    }

    const FGeoMessageEnvelope Unrelated = MakeRelationship(TEXT("spot-2"), TEXT("OTHER1"), TEXT("OTHER2"), T0);
    Watch->IngestMessage(Unrelated, /*bTimelineIsLive=*/true);
    TestEqual(TEXT("unrelated traffic does not alert"), Watch->GetAlerts().Num(), 1);

    Watch->MarkAllSeen();
    TestEqual(TEXT("marking all seen clears the unseen count"), Watch->GetUnseenCount(), 0);
    TestTrue(TEXT("alert is now marked seen"), Watch->GetAlerts()[0].bSeen);
    return true;
}

// Alerts must respect the timeline: replayed traffic (bTimelineIsLive=false)
// must never fire a "new hit right now" alert, even though it matches
// exactly the same watch the live case does.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonWatchReplayGateTest, "IONCOMMAND.Data.Watch.NoAlertsDuringReplay", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonWatchReplayGateTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoWatchSubsystem* Watch = NewObject<UGeoWatchSubsystem>(GameInstance);
    Watch->AddWatch(TEXT("N0CALL"));
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    const FGeoMessageEnvelope Heard = MakeRelationship(TEXT("spot-1"), TEXT("N0CALL"), TEXT("REMOTE1"), T0);

    Watch->IngestMessage(Heard, /*bTimelineIsLive=*/false);
    TestEqual(TEXT("no alert while not live (paused or replay)"), Watch->GetAlerts().Num(), 0);
    TestEqual(TEXT("unseen count stays zero while not live"), Watch->GetUnseenCount(), 0);

    // Same message, now live: proves the gate above - not the match itself -
    // is what suppressed the alert.
    Watch->IngestMessage(Heard, /*bTimelineIsLive=*/true);
    TestEqual(TEXT("alert fires once live"), Watch->GetAlerts().Num(), 1);
    return true;
}

// Alerts are bounded: a sustained flood of matches never grows the alert
// list past its cap, and the newest hit always stays at the front.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonWatchBoundedAlertsTest, "IONCOMMAND.Data.Watch.BoundedAlerts", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonWatchBoundedAlertsTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoWatchSubsystem* Watch = NewObject<UGeoWatchSubsystem>(GameInstance);
    Watch->AddWatch(TEXT("N0CALL"));
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    const int32 OverCap = UGeoWatchSubsystem::MaxAlerts + 50;
    for (int32 Index = 0; Index < OverCap; ++Index)
    {
        const FGeoMessageEnvelope Heard = MakeRelationship(FString::Printf(TEXT("spot-%04d"), Index), TEXT("N0CALL"), TEXT("REMOTE1"), T0 + FTimespan::FromSeconds(Index));
        Watch->IngestMessage(Heard, /*bTimelineIsLive=*/true);
    }
    TestEqual(TEXT("alert list is bounded at MaxAlerts"), Watch->GetAlerts().Num(), static_cast<int32>(UGeoWatchSubsystem::MaxAlerts));
    TestEqual(TEXT("newest alert stays at the front"), Watch->GetAlerts()[0].WatchQuery, FString(TEXT("N0CALL")));
    return true;
}

// Reset (replay start / return-to-live) clears alerts, which are derived
// from live traffic, but keeps watches, which are persisted operator
// configuration exactly like the callsign/locator they live alongside.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FIonWatchResetTest, "IONCOMMAND.Data.Watch.ResetKeepsWatchesClearsAlerts", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FIonWatchResetTest::RunTest(const FString& Parameters)
{
    // UGameInstanceSubsystem is declared UCLASS(Within = GameInstance): it
    // must be constructed with a real UGameInstance as its Outer, or the
    // engine's outer-validity check fails (silently past the first hit,
    // since ensure() only reports once per callsite - masking the same
    // issue in every later test unless every call site is correct).
    UGameInstance* GameInstance = NewObject<UGameInstance>();
    UGeoWatchSubsystem* Watch = NewObject<UGeoWatchSubsystem>(GameInstance);
    Watch->AddWatch(TEXT("N0CALL"));
    const FDateTime T0(2026, 8, 28, 12, 0, 0);
    Watch->IngestMessage(MakeRelationship(TEXT("spot-1"), TEXT("N0CALL"), TEXT("REMOTE1"), T0), /*bTimelineIsLive=*/true);
    TestEqual(TEXT("alert present before reset"), Watch->GetAlerts().Num(), 1);

    Watch->Reset();
    TestEqual(TEXT("alerts cleared by reset"), Watch->GetAlerts().Num(), 0);
    TestEqual(TEXT("unseen count cleared by reset"), Watch->GetUnseenCount(), 0);
    TestEqual(TEXT("watches survive reset"), Watch->GetWatches().Num(), 1);
    return true;
}

#endif
