#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTimelineSubsystem.generated.h"

UCLASS()
class IONCOMMANDDATA_API UGeoTimelineSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Timeline") FDateTime GetTimelineUtc() const;
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Timeline") bool IsLive() const { return bLive; }
    UFUNCTION(BlueprintPure, Category="ION COMMAND|Timeline") bool IsPaused() const { return bPaused; }
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Timeline") void SetPaused(bool bInPaused);
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Timeline") void ReturnToLive();
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Timeline") void SetReplayTime(FDateTime Utc);

private:
    FDateTime ReplayUtc;
    FDateTime PausedUtc;
    bool bLive = true;
    bool bPaused = false;
};
