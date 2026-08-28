#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "GeoWatchSubsystem.generated.h"

// One saved watch: a search query the operator asked to be told about.
// Persisted the same way the settings panel persists callsign/locator - a
// plain string in Game.ini - so a watch survives restart.
USTRUCT(BlueprintType)
struct IONCOMMANDDATA_API FGeoWatchEntry
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FString Query;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FDateTime CreatedUtc;
};

// One recorded hit: what matched, which watch, and when it was observed.
USTRUCT(BlueprintType)
struct IONCOMMANDDATA_API FGeoAlertEntry
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FString WatchQuery;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FString DisplayLabel;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FString Domain;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FDateTime ObservedUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    bool bSeen = false;

    // Selection payload, so choosing an alert can reuse the exact same
    // FOCUS path a search result uses.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Watch")
    FGeoMessageEnvelope Envelope;
};

// Watchlist and alerts, built on UGeoSearchSubsystem's matching (Part A):
// the operator saves a search query as a watch; every newly accepted
// canonical message is tested against every saved watch using the identical
// stable-id / display.* matching UGeoSearchSubsystem::Search() uses, so
// "can I find it" and "will it alert me" never disagree.
//
// Alerts only fire for live traffic. docs/USER_VALUE_ROADMAP.md Priority 2
// requires indexing/expiry to stay timeline-aware, and the motivating use
// case for alerts (own-station activity while the operator is looking away
// from the globe) is inherently a live-monitoring feature: replaying history
// must not be misreported as new hits arriving right now.
UCLASS()
class IONCOMMANDDATA_API UGeoWatchSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

    // Returns false (no-op) for an empty query, a duplicate, or once the
    // bounded watch list is full.
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Watch")
    bool AddWatch(const FString& Query);

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Watch")
    void RemoveWatch(const FString& Query);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Watch")
    const TArray<FGeoWatchEntry>& GetWatches() const { return Watches; }

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Watch")
    const TArray<FGeoAlertEntry>& GetAlerts() const { return Alerts; }

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Watch")
    int32 GetUnseenCount() const { return UnseenCount; }

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Watch")
    void MarkAllSeen();

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Watch")
    void MarkAlertSeen(int32 AlertIndex);

    // Pure core, deliberately independent of any live GameInstance: the
    // caller supplies whether the timeline is currently live so this method
    // is directly unit-testable without standing up a real timeline
    // subsystem. The delegate-bound handler below is the only caller that
    // reaches into the engine to answer that question for real traffic.
    void IngestMessage(const FGeoMessageEnvelope& Message, bool bTimelineIsLive);
    void Reset();

    void LoadPersistedWatches();
    void SavePersistedWatches() const;

    static constexpr int32 MaxAlerts = 200;
    static constexpr int32 MaxWatches = 100;

private:
    void HandleMessageAccepted(const FGeoMessageEnvelope& Message);
    void HandleDataReset();

    TArray<FGeoWatchEntry> Watches;
    // Newest-first, bounded ring.
    TArray<FGeoAlertEntry> Alerts;
    int32 UnseenCount = 0;

    FDelegateHandle MessageAcceptedHandle;
    FDelegateHandle DataResetHandle;
};
