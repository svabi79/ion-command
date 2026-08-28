#include "GeoWatchSubsystem.h"

#include "GeoDataSubsystem.h"
#include "GeoSearchSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "Misc/ConfigCacheIni.h"

void UGeoWatchSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    LoadPersistedWatches();
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>())
        {
            MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &UGeoWatchSubsystem::HandleMessageAccepted);
            DataResetHandle = Data->OnDataReset().AddUObject(this, &UGeoWatchSubsystem::HandleDataReset);
        }
    }
}

void UGeoWatchSubsystem::Deinitialize()
{
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>())
        {
            Data->OnMessageAccepted().Remove(MessageAcceptedHandle);
            Data->OnDataReset().Remove(DataResetHandle);
        }
    }
    Super::Deinitialize();
}

void UGeoWatchSubsystem::HandleMessageAccepted(const FGeoMessageEnvelope& Message)
{
    bool bLive = true;
    if (const UGameInstance* GameInstance = GetGameInstance())
    {
        if (const UGeoTimelineSubsystem* Timeline = GameInstance->GetSubsystem<UGeoTimelineSubsystem>())
        {
            bLive = Timeline->IsLive();
        }
    }
    IngestMessage(Message, bLive);
}

void UGeoWatchSubsystem::HandleDataReset()
{
    Reset();
}

bool UGeoWatchSubsystem::AddWatch(const FString& Query)
{
    const FString Clean = Query.TrimStartAndEnd().ToUpper();
    if (Clean.IsEmpty())
    {
        return false;
    }
    for (const FGeoWatchEntry& Existing : Watches)
    {
        if (Existing.Query == Clean)
        {
            return false; // already watching this term
        }
    }
    if (Watches.Num() >= MaxWatches)
    {
        return false;
    }
    FGeoWatchEntry Entry;
    Entry.Query = Clean;
    Entry.CreatedUtc = FDateTime::UtcNow();
    Watches.Add(MoveTemp(Entry));
    SavePersistedWatches();
    return true;
}

void UGeoWatchSubsystem::RemoveWatch(const FString& Query)
{
    const FString Clean = Query.TrimStartAndEnd().ToUpper();
    const int32 RemovedCount = Watches.RemoveAll([&Clean](const FGeoWatchEntry& Entry) { return Entry.Query == Clean; });
    if (RemovedCount > 0)
    {
        SavePersistedWatches();
    }
}

void UGeoWatchSubsystem::LoadPersistedWatches()
{
    TArray<FString> Lines;
    GConfig->GetArray(TEXT("IonCommand.Watchlist"), TEXT("Query"), Lines, GGameIni);
    Watches.Reset();
    for (const FString& Line : Lines)
    {
        const FString Clean = Line.TrimStartAndEnd().ToUpper();
        if (Clean.IsEmpty())
        {
            continue;
        }
        if (Watches.ContainsByPredicate([&Clean](const FGeoWatchEntry& Entry) { return Entry.Query == Clean; }))
        {
            continue; // corrupt/duplicated ini line - skip rather than double-watch
        }
        if (Watches.Num() >= MaxWatches)
        {
            break;
        }
        FGeoWatchEntry Entry;
        Entry.Query = Clean;
        Entry.CreatedUtc = FDateTime::UtcNow();
        Watches.Add(MoveTemp(Entry));
    }
}

void UGeoWatchSubsystem::SavePersistedWatches() const
{
    TArray<FString> Lines;
    Lines.Reserve(Watches.Num());
    for (const FGeoWatchEntry& Entry : Watches)
    {
        Lines.Add(Entry.Query);
    }
    GConfig->SetArray(TEXT("IonCommand.Watchlist"), TEXT("Query"), Lines, GGameIni);
    GConfig->Flush(false, GGameIni);
}

void UGeoWatchSubsystem::IngestMessage(const FGeoMessageEnvelope& Message, bool bTimelineIsLive)
{
    // No live alerts while paused or in replay: a replayed message is old
    // news re-arriving, not something new happening right now, and firing an
    // alert for it would misrepresent history as a live event.
    if (!bTimelineIsLive || Watches.IsEmpty())
    {
        return;
    }
    bool bAnyMatch = false;
    for (const FGeoWatchEntry& Watch : Watches)
    {
        if (!UGeoSearchSubsystem::Matches(Message, Watch.Query))
        {
            continue;
        }
        bAnyMatch = true;
        FGeoAlertEntry Alert;
        Alert.WatchQuery = Watch.Query;
        Alert.DisplayLabel = Message.Properties.FindRef(TEXT("display.title"));
        if (Alert.DisplayLabel.IsEmpty())
        {
            Alert.DisplayLabel = !Message.EntityId.IsEmpty() ? Message.EntityId : Message.SemanticType;
        }
        Alert.Domain = Message.Domain;
        Alert.ObservedUtc = Message.Time.ObservedUtc;
        Alert.bSeen = false;
        Alert.Envelope = Message;
        Alerts.Insert(MoveTemp(Alert), 0);
        ++UnseenCount;
    }
    if (bAnyMatch && Alerts.Num() > MaxAlerts)
    {
        Alerts.SetNum(MaxAlerts, EAllowShrinking::No);
    }
}

void UGeoWatchSubsystem::MarkAllSeen()
{
    for (FGeoAlertEntry& Alert : Alerts)
    {
        Alert.bSeen = true;
    }
    UnseenCount = 0;
}

void UGeoWatchSubsystem::MarkAlertSeen(int32 AlertIndex)
{
    if (!Alerts.IsValidIndex(AlertIndex) || Alerts[AlertIndex].bSeen)
    {
        return;
    }
    Alerts[AlertIndex].bSeen = true;
    UnseenCount = FMath::Max(0, UnseenCount - 1);
}

void UGeoWatchSubsystem::Reset()
{
    // Watches are persisted operator configuration, exactly like the
    // callsign/locator settings they live alongside in Game.ini, and survive
    // a data reset (replay start / return-to-live). Alerts are derived from
    // live traffic and are cleared, matching every other live-derived cache
    // (selection, HUD aggregates, the active message window itself).
    Alerts.Reset();
    UnseenCount = 0;
}
