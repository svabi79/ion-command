#include "GeoTimelineSubsystem.h"

#include "GeoDataSubsystem.h"

FDateTime UGeoTimelineSubsystem::GetTimelineUtc() const
{
    if (bPaused)
    {
        return PausedUtc;
    }
    return bLive ? FDateTime::UtcNow() : ReplayUtc;
}

void UGeoTimelineSubsystem::SetPaused(bool bInPaused)
{
    if (bPaused == bInPaused)
    {
        return;
    }
    if (bInPaused)
    {
        PausedUtc = GetTimelineUtc();
    }
    bPaused = bInPaused;
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>())
    {
        Data->SetPaused(bPaused);
    }
}

void UGeoTimelineSubsystem::ReturnToLive()
{
    bLive = true;
    SetPaused(false);
}
void UGeoTimelineSubsystem::SetReplayTime(FDateTime Utc) { ReplayUtc = Utc; bLive = false; }
