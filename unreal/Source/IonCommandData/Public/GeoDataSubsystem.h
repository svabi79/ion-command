#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "GeoDataSubsystem.generated.h"

DECLARE_MULTICAST_DELEGATE_OneParam(FOnGeoMessageAccepted, const FGeoMessageEnvelope&);
DECLARE_MULTICAST_DELEGATE(FOnGeoDataReset);

UCLASS()
class IONCOMMANDDATA_API UGeoDataSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;

    bool AcceptMessage(FGeoMessageEnvelope&& Message);
    void NotifyInvalidMessage();
    void NotifyDroppedMessage();
    void SetPaused(bool bInPaused);
    void Reset();
    bool IsPaused() const { return bPaused; }
    void ClearExpired(const FDateTime& TimelineUtc);

    const TArray<FGeoMessageEnvelope>& GetActiveMessages() const { return ActiveMessages; }
    FGeoRuntimeStatistics GetStatistics() const;
    FOnGeoMessageAccepted& OnMessageAccepted() { return MessageAccepted; }
    FOnGeoDataReset& OnDataReset() { return DataReset; }

private:
    UPROPERTY(Transient)
    TArray<FGeoMessageEnvelope> ActiveMessages;

    int32 MaxActiveMessages = 50000;
    double ActiveWindowSeconds = 900.0;
    int64 AcceptedMessageCount = 0;
    int64 InvalidMessageCount = 0;
    int64 DroppedMessageCount = 0;
    bool bPaused = false;
    FOnGeoMessageAccepted MessageAccepted;
    FOnGeoDataReset DataReset;
};
