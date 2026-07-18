#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoReplaySubsystem.generated.h"

UCLASS()
class IONCOMMANDDATA_API UGeoReplaySubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Replay")
    bool StartReplay(FDateTime FromUtc, FDateTime ToUtc, double Speed);

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Replay")
    void ReturnToLive();

private:
    FString LiveUrl = TEXT("ws://127.0.0.1:7810/ws/live");
};

