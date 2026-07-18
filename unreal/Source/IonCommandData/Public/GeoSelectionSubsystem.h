#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "GeoSelectionSubsystem.generated.h"

DECLARE_DYNAMIC_MULTICAST_DELEGATE(FOnGeoSelectionChanged);

UCLASS()
class IONCOMMANDDATA_API UGeoSelectionSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Selection") void SelectMessage(const FGeoMessageEnvelope& Message);
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Selection") void ClearSelection();
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Selection") bool HasSelection() const { return bHasSelection; }
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Selection") FGeoMessageEnvelope GetSelection() const { return Selection; }

    UPROPERTY(BlueprintAssignable, Category="ION COMMAND|Selection")
    FOnGeoSelectionChanged OnSelectionChanged;

private:
    UPROPERTY(Transient) FGeoMessageEnvelope Selection;
    bool bHasSelection = false;
};

