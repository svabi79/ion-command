#include "GeoReplaySubsystem.h"

#include "GenericPlatform/GenericPlatformHttp.h"
#include "GeoDataSubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "GeoStreamSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "Misc/CommandLine.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/Parse.h"

namespace
{
// Same precedence as UGeoStreamSubsystem: the -IonCollectorUrl= command line
// wins over Game.ini, so replay follows the launcher onto a fallback port.
void ResolveLiveUrl(FString& LiveUrl)
{
    GConfig->GetString(TEXT("/Script/IonCommand.IonCommandRuntime"), TEXT("CollectorUrl"), LiveUrl, GGameIni);
    FString UrlOverride;
    if (FParse::Value(FCommandLine::Get(), TEXT("IonCollectorUrl="), UrlOverride) && !UrlOverride.IsEmpty())
    {
        LiveUrl = UrlOverride;
    }
}
}

bool UGeoReplaySubsystem::StartReplay(FDateTime FromUtc, FDateTime ToUtc, double Speed)
{
    if (Speed <= 0.0 || ToUtc < FromUtc) return false;
    ResolveLiveUrl(LiveUrl);
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
    ResolveLiveUrl(LiveUrl);
    if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>()) Timeline->ReturnToLive();
    if (UGeoSelectionSubsystem* Selection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>()) Selection->ClearSelection();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>()) Data->Reset();
    if (UGeoStreamSubsystem* Stream = GetGameInstance()->GetSubsystem<UGeoStreamSubsystem>()) Stream->ConnectToUrl(LiveUrl);
}
