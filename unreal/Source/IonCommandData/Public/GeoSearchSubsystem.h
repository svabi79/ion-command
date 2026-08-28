#pragma once

#include "CoreMinimal.h"
#include "Containers/Ticker.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoSearchTypes.h"
#include "GeoSearchSubsystem.generated.h"

// Bounded, timeline-aware search index over accepted canonical messages.
//
// Fed incrementally from UGeoDataSubsystem's acceptance/reset events (never
// from renderer-private arrays: a renderer only knows what it currently
// draws, which is exactly the unreliable source docs/USER_VALUE_ROADMAP.md
// warns against for Priority 2). Entities and observations that share a
// stable EntityId are grouped into one current-state document; relationships
// and one-shot observations (no EntityId) each stay their own document.
//
// Matching only ever looks at opaque stable identifiers (EntityId,
// FromEntityId, ToEntityId, TargetId) and generic "display."-prefixed
// property values. It never reads a domain-specific key name, so this class
// carries no ham-radio/aviation/etc. vocabulary and needs none to work
// against any current or future domain.
UCLASS()
class IONCOMMANDDATA_API UGeoSearchSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

    // Bounded, ranked, deterministic. Empty query returns no results (the
    // overlay shows a hint instead of the whole index).
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Search")
    TArray<FGeoSearchResult> Search(const FString& Query, int32 MaxResults = 20) const;

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Search")
    int32 GetDocumentCount() const { return Documents.Num(); }

    // Direct lookup by stable key, e.g. to resolve a point-marker's entity id
    // (from a screen ray pick) back to a full canonical envelope without the
    // caller needing to know how the index is stored. Not UFUNCTION: raw
    // struct pointers are not Blueprint-representable, and every caller today
    // is C++.
    const FGeoSearchResult* FindByKey(const FString& Key) const { return Documents.Find(Key); }

    // Pure indexing/matching core, deliberately independent of any live
    // GameInstance so it is directly unit-testable. The delegate-bound
    // handler below is the only caller that reaches into the engine.
    void IngestMessage(const FGeoMessageEnvelope& Message);
    void Reset();
    void PurgeExpired(const FDateTime& TimelineUtc);

    // Shared matching primitive: also used by UGeoWatchSubsystem so a saved
    // watch matches exactly what search would have found for the same text -
    // Part B builds on Part A's index instead of re-implementing it.
    // Rank is lower for a better match (0 = exact stable-id match); callers
    // that only need a yes/no answer can use Matches() instead.
    static bool MatchRank(const FGeoMessageEnvelope& Envelope, const FString& UpperQuery, int32& OutRank);
    static bool Matches(const FGeoMessageEnvelope& Envelope, const FString& UpperQuery);

    // Test-only: raises the document cap so automation tests can measure
    // Search() at a specific scale (e.g. the 50,000-document benchmark in
    // docs/USER_VALUE_ROADMAP.md) without waiting on GConfig/Initialize().
    void SetMaxDocumentsForTesting(int32 Value) { MaxDocuments = Value; }

private:
    void HandleMessageAccepted(const FGeoMessageEnvelope& Message);
    bool Tick(float DeltaSeconds);
    void EvictOldest();
    static void PopulateResult(FGeoSearchResult& Result, const FGeoMessageEnvelope& Message, const FString& Key);

    TMap<FString, FGeoSearchResult> Documents;
    // First-seen insertion order, for FIFO eviction under the document cap -
    // the same bounded-growth strategy UGeoDataSubsystem uses for its active
    // message window. Not reordered on update, so a long-lived, frequently
    // refreshed entity is not penalised for being "old".
    TArray<FString> DocumentOrder;

    int32 MaxDocuments = 20000;
    // Fallback age limit for documents without a validUntil (matches
    // UGeoDataSubsystem's own ActiveWindowSeconds key/default so a document
    // without an explicit expiry ages out of search on the same schedule it
    // would age out of the active message window).
    double ActiveWindowSeconds = 900.0;
    double LastPurgeSeconds = -1000.0;

    FTSTicker::FDelegateHandle TickHandle;
    FDelegateHandle MessageAcceptedHandle;
    FDelegateHandle DataResetHandle;
};
