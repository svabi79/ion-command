#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"
#include "GeoLayerTypes.generated.h"

USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoLayerManifest
{
    GENERATED_BODY()

    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString LayerId;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString DisplayName;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") FString Domain;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") TArray<FString> AcceptedSemanticTypes;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") TArray<EGeoGeometryType> GeometryTypes;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bDefaultVisibility = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") double DefaultTimeWindowSeconds = 900.0;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsSelection = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsReplay = true;
    UPROPERTY(EditAnywhere, BlueprintReadWrite, Category="ION COMMAND|Layer") bool bSupportsAggregation = false;
};

class IONCOMMANDCORE_API IGeoRenderAdapter
{
public:
    virtual ~IGeoRenderAdapter() = default;
    virtual bool Supports(const FGeoMessageEnvelope& Message) const = 0;
    virtual void Submit(const FGeoMessageEnvelope& Message) = 0;
    virtual void Reset() = 0;
};

// Shared instrumentation for renderers that keep stable per-item render
// slots and update them incrementally instead of rebuilding a whole
// instanced population. One struct serves every such renderer (arcs,
// points, and any future geometry) so the operator-facing diagnostics stay
// uniform; a renderer that has nothing to report for a field simply leaves
// it at zero.
USTRUCT(BlueprintType)
struct IONCOMMANDCORE_API FGeoRenderLayerStatistics
{
    GENERATED_BODY()

    // Full-population resubmits (clear and re-add every item). Expected to
    // stay at zero during steady-state traffic; only a deliberate,
    // infrequent operator action (e.g. changing a filter) should pay this.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 FullRebuilds = 0;

    // Items inserted with a new render slot, touching only that slot.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 IncrementalInserts = 0;

    // Items removed via swap-and-pop, touching only the freed slot and (if
    // one was needed) the single slot swapped into it.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 IncrementalRemovals = 0;

    // In-place transform/custom-data refreshes that neither added nor
    // removed a slot (dead reckoning, style/state refresh).
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 IncrementalUpdates = 0;

    // Removals specifically caused by a capacity bound rather than age.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 CapacityEvictions = 0;

    // Removals specifically caused by age/validity expiry.
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int64 ExpiredRemovals = 0;

    // Live counts, so the bookkeeping can be cross-checked at runtime: for
    // a renderer with no per-item hide state the two should track each
    // other one-for-one (times a fixed instances-per-item factor).
    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int32 TrackedItems = 0;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Diagnostics")
    int32 RenderedInstances = 0;
};

