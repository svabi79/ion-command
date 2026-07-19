#include "GeoStreamSubsystem.h"

#include "GeoDataSubsystem.h"
#include "GeoEnvelopeJsonParser.h"
#include "GeoTimelineSubsystem.h"
#include "IonCommandData.h"
#include "Misc/CommandLine.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/Parse.h"
#include "Modules/ModuleManager.h"
#include "WebSocketsModule.h"

void UGeoStreamSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    GConfig->GetString(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("CollectorUrl"), CollectorUrl, GGameIni);
    // -IonCollectorUrl=ws://host:port/ws/live lets verification harnesses run
    // an isolated collector without touching an operator's live pair on 7810.
    FString UrlOverride;
    if (FParse::Value(FCommandLine::Get(), TEXT("IonCollectorUrl="), UrlOverride) && !UrlOverride.IsEmpty())
    {
        CollectorUrl = UrlOverride;
        UE_LOG(LogIonGeoData, Display, TEXT("Collector URL overridden from the command line: %s"), *CollectorUrl);
    }
    GConfig->GetInt(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("MaxPendingMessages"), MaxPendingMessages, GGameIni);
    GConfig->GetInt(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("MaxBatchSize"), MaxBatchSize, GGameIni);
    GConfig->GetDouble(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("ReconnectSeconds"), ReconnectSeconds, GGameIni);
    FModuleManager::LoadModuleChecked<FWebSocketsModule>(TEXT("WebSockets"));
    TickHandle = FTSTicker::GetCoreTicker().AddTicker(FTickerDelegate::CreateUObject(this, &UGeoStreamSubsystem::Tick));
    Connect();
}

void UGeoStreamSubsystem::Deinitialize()
{
    bIntentionalDisconnect = true;
    Disconnect();
    FTSTicker::GetCoreTicker().RemoveTicker(TickHandle);
    FString Unused;
    while (PendingMessages.Dequeue(Unused)) {}
    PendingMessageCount.Store(0);
    Super::Deinitialize();
}

void UGeoStreamSubsystem::Connect()
{
    if (State == EGeoStreamState::Connecting || State == EGeoStreamState::Connected) return;
    bIntentionalDisconnect = false;
    State = EGeoStreamState::Connecting;
    LastConnectionAttemptSeconds = FPlatformTime::Seconds();
    Socket = FWebSocketsModule::Get().CreateWebSocket(CollectorUrl);
    Socket->OnConnected().AddUObject(this, &UGeoStreamSubsystem::OnConnected);
    Socket->OnConnectionError().AddUObject(this, &UGeoStreamSubsystem::OnConnectionError);
    Socket->OnClosed().AddUObject(this, &UGeoStreamSubsystem::OnClosed);
    Socket->OnMessage().AddUObject(this, &UGeoStreamSubsystem::OnMessage);
    Socket->Connect();
}

void UGeoStreamSubsystem::Disconnect()
{
    bIntentionalDisconnect = true;
    if (Socket.IsValid())
    {
        Socket->OnConnected().RemoveAll(this);
        Socket->OnConnectionError().RemoveAll(this);
        Socket->OnClosed().RemoveAll(this);
        Socket->OnMessage().RemoveAll(this);
        Socket->Close(1000, TEXT("client shutdown"));
        Socket.Reset();
    }
    State = EGeoStreamState::Disconnected;
}

void UGeoStreamSubsystem::ConnectToUrl(const FString& Url)
{
    if (Url.IsEmpty()) return;
    Disconnect();
    FString Discarded;
    while (PendingMessages.Dequeue(Discarded)) {}
    PendingMessageCount.Store(0);
    CollectorUrl = Url;
    Connect();
}

bool UGeoStreamSubsystem::Tick(float DeltaSeconds)
{
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>())
    {
        if (!Data->IsPaused())
        {
            FString Json;
            int32 Processed = 0;
            while (Processed < MaxBatchSize && PendingMessages.Dequeue(Json))
            {
                PendingMessageCount.DecrementExchange();
                FGeoMessageEnvelope Envelope;
                FString Error;
                if (FGeoEnvelopeJsonParser::Parse(Json, Envelope, Error))
                {
                    if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>(); Timeline && !Timeline->IsLive())
                    {
                        Timeline->SetReplayTime(Envelope.Time.ObservedUtc);
                    }
                    Data->AcceptMessage(MoveTemp(Envelope));
                }
                else
                {
                    Data->NotifyInvalidMessage();
                    UE_LOG(LogIonGeoData, Warning, TEXT("Rejected stream message: %s"), *Error);
                }
                ++Processed;
            }
            const double NowSeconds = FPlatformTime::Seconds();
            if (NowSeconds - LastExpiryCheckSeconds >= 1.0)
            {
                const UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>();
                Data->ClearExpired(Timeline ? Timeline->GetTimelineUtc() : FDateTime::UtcNow());
                LastExpiryCheckSeconds = NowSeconds;
            }
        }
    }
    if (!bIntentionalDisconnect && State != EGeoStreamState::Connected && State != EGeoStreamState::Connecting && FPlatformTime::Seconds() - LastConnectionAttemptSeconds >= ReconnectSeconds)
    {
        Connect();
    }
    return true;
}

void UGeoStreamSubsystem::OnConnected()
{
    State = EGeoStreamState::Connected;
    UE_LOG(LogIonGeoData, Display, TEXT("Connected to %s"), *CollectorUrl);
}

void UGeoStreamSubsystem::OnConnectionError(const FString& Error)
{
    State = EGeoStreamState::Degraded;
    Socket.Reset();
    UE_LOG(LogIonGeoData, Warning, TEXT("Collector connection failed: %s"), *Error);
}

void UGeoStreamSubsystem::OnClosed(int32 StatusCode, const FString& Reason, bool bWasClean)
{
    State = EGeoStreamState::Disconnected;
    Socket.Reset();
    if (bWasClean && CollectorUrl.Contains(TEXT("/ws/replay"))) bIntentionalDisconnect = true;
    UE_LOG(LogIonGeoData, Display, TEXT("Collector connection closed (%d, clean=%d): %s"), StatusCode, bWasClean, *Reason);
}

void UGeoStreamSubsystem::OnMessage(const FString& Message)
{
    const int32 Previous = PendingMessageCount.IncrementExchange();
    if (Previous >= MaxPendingMessages)
    {
        PendingMessageCount.DecrementExchange();
        if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>()) Data->NotifyDroppedMessage();
        return;
    }
    PendingMessages.Enqueue(Message);
}
