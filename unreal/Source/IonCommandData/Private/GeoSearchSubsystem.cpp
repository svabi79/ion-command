#include "GeoSearchSubsystem.h"

#include "GeoDataSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "HAL/PlatformTime.h"
#include "Misc/ConfigCacheIni.h"

void UGeoSearchSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    GConfig->GetInt(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("MaxSearchDocuments"), MaxDocuments, GGameIni);
    MaxDocuments = FMath::Max(MaxDocuments, 500);
    // Same key UGeoDataSubsystem reads: a document without its own validUntil
    // ages out of search on the same schedule it ages out of the active
    // message window, so the two never visibly disagree.
    GConfig->GetDouble(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("ActiveWindowSeconds"), ActiveWindowSeconds, GGameIni);

    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>())
        {
            MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &UGeoSearchSubsystem::HandleMessageAccepted);
            DataResetHandle = Data->OnDataReset().AddUObject(this, &UGeoSearchSubsystem::Reset);
        }
    }
    TickHandle = FTSTicker::GetCoreTicker().AddTicker(FTickerDelegate::CreateUObject(this, &UGeoSearchSubsystem::Tick));
}

void UGeoSearchSubsystem::Deinitialize()
{
    FTSTicker::GetCoreTicker().RemoveTicker(TickHandle);
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

void UGeoSearchSubsystem::HandleMessageAccepted(const FGeoMessageEnvelope& Message)
{
    IngestMessage(Message);
}

bool UGeoSearchSubsystem::Tick(float DeltaSeconds)
{
    // Mirrors UGeoStreamSubsystem's own throttle: the ticker itself runs
    // every frame, but the actual sweep is gated to roughly once a second.
    const double NowSeconds = FPlatformTime::Seconds();
    if (NowSeconds - LastPurgeSeconds < 1.0)
    {
        return true;
    }
    LastPurgeSeconds = NowSeconds;
    if (const UGameInstance* GameInstance = GetGameInstance())
    {
        if (const UGeoTimelineSubsystem* Timeline = GameInstance->GetSubsystem<UGeoTimelineSubsystem>())
        {
            PurgeExpired(Timeline->GetTimelineUtc());
        }
    }
    return true;
}

void UGeoSearchSubsystem::IngestMessage(const FGeoMessageEnvelope& Message)
{
    const FString Key = !Message.EntityId.IsEmpty() ? Message.EntityId : Message.MessageId;
    if (Key.IsEmpty())
    {
        return;
    }
    FGeoSearchResult* Existing = Documents.Find(Key);
    if (!Existing)
    {
        if (Documents.Num() >= MaxDocuments)
        {
            EvictOldest();
        }
        DocumentOrder.Add(Key);
        Existing = &Documents.Add(Key);
        Existing->bIsGrouped = !Message.EntityId.IsEmpty();
        Existing->ObservationCount = 0;
    }
    PopulateResult(*Existing, Message, Key);
    ++Existing->ObservationCount;
}

void UGeoSearchSubsystem::PopulateResult(FGeoSearchResult& Result, const FGeoMessageEnvelope& Message, const FString& Key)
{
    Result.Key = Key;
    Result.DisplayLabel = Message.Properties.FindRef(TEXT("display.title"));
    if (Result.DisplayLabel.IsEmpty())
    {
        // Generic fallback so every result has some label even when a domain
        // has not published a display.title for this semantic type - never a
        // domain-specific guess, just the identifiers the envelope already
        // carries.
        Result.DisplayLabel = !Message.EntityId.IsEmpty() ? Message.EntityId : Message.SemanticType;
    }
    Result.DisplaySubtitle = Message.Properties.FindRef(TEXT("display.primary"));
    Result.Domain = Message.Domain;
    Result.SemanticType = Message.SemanticType;
    Result.GeometryType = Message.Geometry.Type;
    Result.SourcePluginId = Message.Source.PluginId;
    Result.ObservedUtc = Message.Time.ObservedUtc;
    Result.ValidUntilUtc = Message.Time.ValidUntilUtc;
    Result.bHasValidUntil = Message.Time.bHasValidUntil;
    Result.Envelope = Message;
}

void UGeoSearchSubsystem::EvictOldest()
{
    // Same policy as UGeoDataSubsystem::AcceptMessage: trim a slice from the
    // front instead of refusing new keys, so a firehose domain cannot starve
    // every other domain's search coverage by filling the cap once.
    const int32 RemoveCount = FMath::Min(MaxDocuments / 20 + 1, DocumentOrder.Num());
    for (int32 Index = 0; Index < RemoveCount; ++Index)
    {
        Documents.Remove(DocumentOrder[Index]);
    }
    DocumentOrder.RemoveAt(0, RemoveCount, EAllowShrinking::No);
}

void UGeoSearchSubsystem::Reset()
{
    Documents.Reset();
    DocumentOrder.Reset();
}

void UGeoSearchSubsystem::PurgeExpired(const FDateTime& TimelineUtc)
{
    if (DocumentOrder.IsEmpty())
    {
        return;
    }
    const FDateTime Cutoff = TimelineUtc - FTimespan::FromSeconds(ActiveWindowSeconds);
    TArray<FString> Survivors;
    Survivors.Reserve(DocumentOrder.Num());
    for (const FString& Key : DocumentOrder)
    {
        const FGeoSearchResult* Doc = Documents.Find(Key);
        if (!Doc)
        {
            continue; // already gone (evicted); drop the stale order entry too
        }
        const bool bExpired = Doc->bHasValidUntil ? (Doc->ValidUntilUtc < TimelineUtc) : (Doc->ObservedUtc < Cutoff);
        if (bExpired)
        {
            Documents.Remove(Key);
        }
        else
        {
            Survivors.Add(Key);
        }
    }
    DocumentOrder = MoveTemp(Survivors);
}

namespace
{
// Case-insensitive id test against one opaque stable identifier. Rank 0 is
// an exact match, 1 a prefix match, 2 any substring match; INDEX_NONE means
// no match at all.
int32 RankIdMatch(const FString& Id, const FString& UpperQuery)
{
    if (Id.IsEmpty())
    {
        return INDEX_NONE;
    }
    const FString UpperId = Id.ToUpper();
    if (UpperId == UpperQuery) return 0;
    if (UpperId.StartsWith(UpperQuery)) return 1;
    if (UpperId.Contains(UpperQuery)) return 2;
    return INDEX_NONE;
}
}

bool UGeoSearchSubsystem::MatchRank(const FGeoMessageEnvelope& Envelope, const FString& UpperQuery, int32& OutRank)
{
    if (UpperQuery.IsEmpty())
    {
        return false;
    }
    int32 BestRank = TNumericLimits<int32>::Max();
    bool bMatched = false;
    auto Consider = [&BestRank, &bMatched](int32 Rank)
    {
        if (Rank == INDEX_NONE) return;
        bMatched = true;
        BestRank = FMath::Min(BestRank, Rank);
    };

    Consider(RankIdMatch(Envelope.EntityId, UpperQuery));
    Consider(RankIdMatch(Envelope.FromEntityId, UpperQuery));
    Consider(RankIdMatch(Envelope.ToEntityId, UpperQuery));
    Consider(RankIdMatch(Envelope.TargetId, UpperQuery));

    // Generic display metadata only - never a domain-specific property key.
    // Ranks 3-5 mirror the id tiers above but sort behind them, so an exact
    // id match always beats a substring hit inside a display string.
    for (const TPair<FString, FString>& Prop : Envelope.Properties)
    {
        if (!Prop.Key.StartsWith(TEXT("display."))) continue;
        const FString UpperValue = Prop.Value.ToUpper();
        if (UpperValue.IsEmpty()) continue;
        if (UpperValue == UpperQuery) Consider(3);
        else if (UpperValue.StartsWith(UpperQuery)) Consider(4);
        else if (UpperValue.Contains(UpperQuery)) Consider(5);
    }

    if (bMatched)
    {
        OutRank = BestRank;
    }
    return bMatched;
}

bool UGeoSearchSubsystem::Matches(const FGeoMessageEnvelope& Envelope, const FString& UpperQuery)
{
    int32 Rank = 0;
    return MatchRank(Envelope, UpperQuery, Rank);
}

TArray<FGeoSearchResult> UGeoSearchSubsystem::Search(const FString& Query, int32 MaxResults) const
{
    TArray<FGeoSearchResult> Results;
    const FString UpperQuery = Query.TrimStartAndEnd().ToUpper();
    if (UpperQuery.IsEmpty() || MaxResults <= 0)
    {
        return Results;
    }

    struct FScoredResult
    {
        const FGeoSearchResult* Result;
        int32 Rank;
    };
    TArray<FScoredResult> Scored;
    Scored.Reserve(Documents.Num());
    for (const TPair<FString, FGeoSearchResult>& Pair : Documents)
    {
        int32 Rank = 0;
        if (MatchRank(Pair.Value.Envelope, UpperQuery, Rank))
        {
            Scored.Add({&Pair.Value, Rank});
        }
    }
    // Deterministic order: best rank first, then most recently observed,
    // then key as a final tiebreaker so equal candidates never reorder
    // between two searches for the same query.
    Scored.Sort([](const FScoredResult& A, const FScoredResult& B)
    {
        if (A.Rank != B.Rank) return A.Rank < B.Rank;
        if (A.Result->ObservedUtc != B.Result->ObservedUtc) return A.Result->ObservedUtc > B.Result->ObservedUtc;
        return A.Result->Key < B.Result->Key;
    });

    const int32 Count = FMath::Min(MaxResults, Scored.Num());
    Results.Reserve(Count);
    for (int32 Index = 0; Index < Count; ++Index)
    {
        Results.Add(*Scored[Index].Result);
    }
    return Results;
}
