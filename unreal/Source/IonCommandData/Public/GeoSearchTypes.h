#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"
#include "GeoSearchTypes.generated.h"

// Domain-neutral search result: one discoverable object built only from
// stable canonical identifiers and generic display.* metadata (see
// docs/USER_VALUE_ROADMAP.md, "Priority 2: search and focus"). Nothing here
// is specific to any one domain plugin.
USTRUCT(BlueprintType)
struct IONCOMMANDDATA_API FGeoSearchResult
{
    GENERATED_BODY()

    // Stable grouping key: the canonical EntityId when the message carries
    // one, otherwise the message's own MessageId. Relationships and one-shot
    // observations (no EntityId) are therefore never grouped - each stays
    // its own discoverable result, exactly like the roadmap requires.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString Key;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString DisplayLabel;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString DisplaySubtitle;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString Domain;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString SemanticType;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    EGeoGeometryType GeometryType = EGeoGeometryType::None;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FString SourcePluginId;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FDateTime ObservedUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FDateTime ValidUntilUtc;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    bool bHasValidUntil = false;

    // True for an entity/observation group keyed by a stable EntityId; false
    // for a one-shot observation or relationship keyed by its own MessageId.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    bool bIsGrouped = false;

    // How many accepted messages have updated this key. A frequently updated
    // aircraft therefore stays one result with a growing count rather than
    // hundreds of position-fix duplicates.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    int32 ObservationCount = 1;

    // Canonical selection payload: the latest snapshot for this key, passed
    // straight to UGeoSelectionSubsystem::SelectMessage() on FOCUS. Carries
    // the geometry (Point or GreatCircle) the camera frames.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Search")
    FGeoMessageEnvelope Envelope;
};
