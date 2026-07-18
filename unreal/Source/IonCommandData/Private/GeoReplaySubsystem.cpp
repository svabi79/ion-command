#include "GeoReplaySubsystem.h"

#include "GenericPlatform/GenericPlatformHttp.h"
#include "GeoDataSubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "GeoStreamSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "Misc/ConfigCacheIni.h"

bool UGeoReplaySubsystem::StartReplay(FDateTime FromUtc, FDateTime ToUtc, double Speed)
{
    if (Speed <= 0.0 || ToUtc < FromUtc) return false;
    GConfig->GetString(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("CollectorUrl"), LiveUrl, GGameIni);
    FString ReplayUrl = LiveUrl.Replace(TEXT("/ws/live"), TEXT("/ws/replay"));
    ReplayUrl += FString::Printf(TEXT("?from=%s&to=%s&speed=%.2f"), *FGenericPlatformHttp::UrlEncode(FromUtc.ToIso8601()), *FGenericPlatformHttp::UrlEncode(ToUtc.ToIso8601()), Speed);
    if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>()) { Timeline->SetPaused(false); Timeline->SetReplayTime(FromUtc); }
    if (UGeoSelectionSubsystem* Selection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>()) Selection->ClearSelection();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>()) Data->Reset();
    if (UGeoStreamSubsystem* Stream = GetGameInstance()->GetSubsystem<UGeoStreamSubsystem>()) { Stream->ConnectToUrl(ReplayUrl); return true; }
    return false;
}

void UGeoReplaySubsystem::ReturnToLive()
{
    GConfig->GetString(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("CollectorUrl"), LiveUrl, GGameIni);
    if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>()) Timeline->ReturnToLive();
    if (UGeoSelectionSubsystem* Selection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>()) Selection->ClearSelection();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>()) Data->Reset();
    if (UGeoStreamSubsystem* Stream = GetGameInstance()->GetSubsystem<UGeoStreamSubsystem>()) Stream->ConnectToUrl(LiveUrl);
}
