#pragma once

#include "CoreMinimal.h"
#include "GameFramework/HUD.h"
#include "GeoArcLayerActor.h"
#include "GeoTypes.h"
#include "IonCockpitPanelSubsystem.h"
#include "IonCockpitHudActor.generated.h"

class UGeoDataSubsystem;

// One aggregated display region (generic display.*Region property value).
struct FIonRegionStat
{
    FString Label;
    double Weight = 0.0;
};

UENUM()
enum class EIonCockpitMode : uint8
{
    Full,
    Minimal,
    Hidden
};

// One aggregated relationship endpoint, kept for the on-globe label instrument.
struct FIonEndpointStat
{
    FString Label;
    FGeoPosition Position;
    double Weight = 0.0;
    double LastSeenSeconds = 0.0;
};

// Screen-space mission-control cockpit: status bar, traffic instruments, polar
// oval dial, and projected endpoint labels. Purely generic — every string it
// renders comes from display.* properties, layer-provided palette labels, or
// domain-neutral geophysical readouts.
UCLASS()
class IONCOMMANDUI_API AIonCockpitHudActor final : public AHUD
{
    GENERATED_BODY()

public:
    AIonCockpitHudActor();
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void DrawHUD() override;

    void CycleMode();
    EIonCockpitMode GetMode() const { return Mode; }

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void OnDataReset();
    void AdvanceRateBuckets(int64 NowSecond);
    void RefreshAggregates(double NowSeconds);
    void RecordEndpoint(const FString& EntityId, const FString& Label, const FGeoPosition& Position);

    void DrawStatusBar(float Scale, float Alpha);
    void DrawTrafficPanel(float Scale, float Alpha);
    void DrawRatePanel(float Scale, float Alpha, float PanelY);
    void DrawRegionsPanel(float Scale, float Alpha, float PanelY);
    void DrawPolarPanel(float Scale, float Alpha);
    void DrawProviderPanels(float Scale, float Alpha, float PanelY);
    void DrawEndpointLabels(float Scale, float Alpha);
    void DrawModeHint(float Scale, float Alpha);

    void DrawPanelFrame(float X, float Y, float Width, float Height, const FString& Title, float Scale, float Alpha);
    void DrawLabelValue(float& CursorX, float Y, const FString& Label, const FString& Value, const FLinearColor& ValueColor, float Scale, float Alpha);
    void DrawTextAt(const FString& Text, float X, float Y, const FLinearColor& Color, float TextScale, bool bCentered = false);
    void DrawRect(float X, float Y, float Width, float Height, const FLinearColor& Color);
    void DrawLineSegment(const FVector2D& From, const FVector2D& To, const FLinearColor& Color, float Thickness);

    EIonCockpitMode Mode = EIonCockpitMode::Full;

    // Latest domain-neutral geophysical readouts (negative sentinel = unseen).
    double EnvKp = -1.0;
    double EnvAIndex = -1.0;
    double EnvSolarFlux = -1.0;
    double EnvWindKms = -1.0;
    double EnvBzNt = 0.0;
    bool bHasEnvBz = false;

    // 60 one-second buckets of accepted path traffic (renderable relationships).
    int32 RateBuckets[60] = {};
    int64 LastBucketSecond = 0;

    // Bounded endpoint aggregation for the on-globe labels.
    TMap<FString, FIonEndpointStat> EndpointStats;
    static constexpr int32 MaxEndpointStats = 4096;
    static constexpr double EndpointRetentionSeconds = 120.0;

    // Bounded aggregation of generic display regions for the top-regions panel.
    TMap<FString, double> RegionWeights;
    static constexpr int32 MaxRegionStats = 512;

    // Instrument caches refreshed on a fixed cadence, not per frame.
    TArray<FGeoPaletteBreakdownEntry> CachedBreakdown;
    FString CachedPanelTitle;
    int32 CachedBandFocus = INDEX_NONE;
    TArray<FIonEndpointStat> CachedTopEndpoints;
    TArray<FIonRegionStat> CachedTopRegions;
    double CachedRegionTotal = 0.0;
    TArray<FIonCockpitPanelModel> CachedPanels;
    double LastAggregateSeconds = -1000.0;
    float TrafficPanelBottomY = 0.0f;
    float RatePanelBottomY = 0.0f;
    float PolarPanelBottomY = 0.0f;

    TWeakObjectPtr<AGeoArcLayerActor> ArcLayer;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    FDelegateHandle MessageAcceptedHandle;
    FDelegateHandle DataResetHandle;
};
