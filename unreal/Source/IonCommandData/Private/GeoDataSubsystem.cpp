#include "GeoDataSubsystem.h"

#include "IonCommandData.h"
#include "Misc/ConfigCacheIni.h"

void UGeoDataSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    GConfig->GetInt(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("MaxActiveMessages"), MaxActiveMessages, GGameIni);
    GConfig->GetDouble(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("ActiveWindowSeconds"), ActiveWindowSeconds, GGameIni);
    MaxActiveMessages = FMath::Max(MaxActiveMessages, 1000);
    ActiveMessages.Reserve(MaxActiveMessages);
}

bool UGeoDataSubsystem::AcceptMessage(FGeoMessageEnvelope&& Message)
{
    if (bPaused) return false;
    if (Message.SchemaVersion != 1 || Message.MessageId.IsEmpty() || Message.SemanticType.IsEmpty())
    {
        NotifyInvalidMessage();
        return false;
    }
    if (ActiveMessages.Num() >= MaxActiveMessages)
    {
        // Window-cap eviction is the bounded history working as designed at
        // firehose rates - tracked separately from real backpressure drops.
        const int32 RemoveCount = FMath::Min(MaxActiveMessages / 20 + 1, ActiveMessages.Num());
        ActiveMessages.RemoveAt(0, RemoveCount, EAllowShrinking::No);
        EvictedMessageCount += RemoveCount;
    }
    const int32 Index = ActiveMessages.Add(MoveTemp(Message));
    ++AcceptedMessageCount;
    MessageAccepted.Broadcast(ActiveMessages[Index]);
    return true;
}

void UGeoDataSubsystem::NotifyInvalidMessage() { ++InvalidMessageCount; }
void UGeoDataSubsystem::NotifyDroppedMessage() { ++DroppedMessageCount; }
void UGeoDataSubsystem::SetPaused(bool bInPaused) { bPaused = bInPaused; }
void UGeoDataSubsystem::Reset() { ActiveMessages.Reset(); DataReset.Broadcast(); }

void UGeoDataSubsystem::ClearExpired(const FDateTime& TimelineUtc)
{
    if (bPaused || ActiveMessages.IsEmpty()) return;
    const FDateTime Cutoff = TimelineUtc - FTimespan::FromSeconds(ActiveWindowSeconds);
    ActiveMessages.RemoveAll([&Cutoff](const FGeoMessageEnvelope& Message) { return Message.Time.ObservedUtc < Cutoff; });
}

FGeoRuntimeStatistics UGeoDataSubsystem::GetStatistics() const
{
    FGeoRuntimeStatistics Result;
    Result.AcceptedMessages = AcceptedMessageCount;
    Result.InvalidMessages = InvalidMessageCount;
    Result.DroppedMessages = DroppedMessageCount;
    Result.EvictedMessages = EvictedMessageCount;
    Result.ActiveMessages = ActiveMessages.Num();
    return Result;
}
