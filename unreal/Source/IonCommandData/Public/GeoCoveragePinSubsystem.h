#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoCoveragePinSubsystem.generated.h"

DECLARE_DYNAMIC_MULTICAST_DELEGATE(FOnGeoCoveragePinsChanged);

USTRUCT(BlueprintType)
struct IONCOMMANDDATA_API FGeoCoveragePin
{
    GENERATED_BODY()

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Coverage")
    FString Key;

    UPROPERTY(BlueprintReadOnly, Category="ION COMMAND|Coverage")
    FString Label;
};

// Bounded sticky set of coverage pins (satellite footprints today).
// Generic: keyed by an opaque pin id from visual.pinKey, no domain vocabulary.
// Default is empty — gated areas stay off until the operator pins them.
UCLASS()
class IONCOMMANDDATA_API UGeoCoveragePinSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    static constexpr int32 MaxPins = 16;

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Coverage")
    bool IsPinned(const FString& Key) const;

    // Inserts or removes Key. At capacity the oldest pin is dropped.
    // Returns the resulting pinned state.
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Coverage")
    bool TogglePin(const FString& Key, const FString& Label);

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Coverage")
    void Unpin(const FString& Key);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Coverage")
    const TArray<FGeoCoveragePin>& GetPins() const { return Pins; }

    UPROPERTY(BlueprintAssignable, Category="ION COMMAND|Coverage")
    FOnGeoCoveragePinsChanged OnPinsChanged;

private:
    TArray<FGeoCoveragePin> Pins;
};
