#pragma once

#include "CoreMinimal.h"
#include "Containers/Ticker.h"
#include "IWebSocket.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoStreamSubsystem.generated.h"

UENUM(BlueprintType)
enum class EGeoStreamState : uint8
{
    Disconnected,
    Connecting,
    Connected,
    Degraded
};

UCLASS()
class IONCOMMANDDATA_API UGeoStreamSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Stream")
    void Connect();

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Stream")
    void Disconnect();

    UFUNCTION(BlueprintCallable, Category="ION COMMAND|Stream")
    void ConnectToUrl(const FString& Url);

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Stream")
    EGeoStreamState GetState() const { return State; }

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Stream")
    FString GetCollectorUrl() const { return CollectorUrl; }

    UFUNCTION(BlueprintPure, Category="ION COMMAND|Stream")
    int32 GetPendingMessageCount() const { return PendingMessageCount.Load(); }

private:
    bool Tick(float DeltaSeconds);
    void OnConnected();
    void OnConnectionError(const FString& Error);
    void OnClosed(int32 StatusCode, const FString& Reason, bool bWasClean);
    void OnMessage(const FString& Message);

    TSharedPtr<IWebSocket> Socket;
    TQueue<FString, EQueueMode::Mpsc> PendingMessages;
    TAtomic<int32> PendingMessageCount{0};
    FTSTicker::FDelegateHandle TickHandle;
    FString CollectorUrl = TEXT("ws://127.0.0.1:7810/ws/live");
    int32 MaxPendingMessages = 16384;
    int32 MaxBatchSize = 512;
    double ReconnectSeconds = 3.0;
    double LastConnectionAttemptSeconds = -1000.0;
    double LastExpiryCheckSeconds = -1000.0;
    EGeoStreamState State = EGeoStreamState::Disconnected;
    bool bIntentionalDisconnect = false;
};
