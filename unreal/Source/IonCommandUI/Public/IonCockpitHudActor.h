#pragma once

#include "CoreMinimal.h"
#include "GameFramework/HUD.h"
#include "GeoArcLayerActor.h"
#include "GeoSearchTypes.h"
#include "GeoTypes.h"
#include "IonCockpitPanelSubsystem.h"
#include "IonCockpitHudActor.generated.h"

class UGeoDataSubsystem;
class AGeoArcLayerActor;
class AGeoPointLayerActor;

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

    // Overlay menu: O toggles it; clicks on its rows toggle layers. Returns
    // true when the click was consumed (the controller then skips path
    // selection).
    void ToggleOverlayMenu() { bOverlayMenuOpen = !bOverlayMenuOpen; bSearchOpen = false; bWatchPanelOpen = false; }
    bool HandleClick(const FVector2D& ScreenPosition);

    // Settings panel (opened from the overlay menu's SETTINGS row). While a
    // text field is focused the player controller routes typed characters
    // here; Control: 0 = backspace, 1 = commit, 2 = cancel.
    void OpenSettings();
    // True whenever any text-capture surface (settings field or search
    // query) has focus. The player controller checks this single flag before
    // letting a keypress reach any camera/layer/band/timeline hotkey, and
    // routes the character on to whichever surface is actually active -
    // one shared input-routing state for both panels, not two.
    bool IsCapturingText() const { return (bSettingsOpen && EditingRow >= 0) || bSearchOpen; }
    void SettingsTextChar(TCHAR Character);
    void SettingsTextControl(int32 Control);

    // Search overlay ("/" or "S"): keyboard-first, incremental results from
    // UGeoSearchSubsystem. Control: 0 = backspace, 1 = enter (focus
    // highlighted result), 2 = escape (close).
    void OpenSearch();
    void CloseSearch();
    bool IsSearchActive() const { return bSearchOpen; }
    void SearchTextChar(TCHAR Character);
    void SearchTextControl(int32 Control);
    void SearchMoveHighlight(int32 Delta);

    // Watch/alert panel ("W"): saved watches and recent matches from
    // UGeoWatchSubsystem.
    void ToggleWatchPanel();

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
    // Always-on-top "you are here" reticle for the configured own station, so
    // dense traffic can never bury it the way world-space markers do.
    void DrawOwnStationReticle(float Scale, float Alpha);
    void DrawOverlayMenu(float Scale, float Alpha);
    void DrawHoverTooltip(float Scale, float Alpha);
    void DrawModeHint(float Scale, float Alpha);
    // Always-on-top focus ring for the current selection (Part A): projects
    // a Point position, or a GreatCircle's midpoint, to screen space exactly
    // like the own-station reticle does, so a search/alert-focused object
    // stays visually prioritised regardless of 3D aggregation or render
    // budget - the one guarantee the point layer cannot give it directly
    // (unlike paths, it has no dedicated 3D selection highlight).
    void DrawSelectionReticle(float Scale, float Alpha);
    // Small always-visible unseen-alert badge, legible at a glance on a wall
    // display even while the watch panel itself is closed.
    void DrawAlertIndicator(float Scale, float Alpha);

    void DrawPanelFrame(float X, float Y, float Width, float Height, const FString& Title, float Scale, float Alpha);
    bool DrawRegionFlag(const FString& RegionName, float X, float Y, float Width, float Height, float Alpha);
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
    FString EnvXrayClass;

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

    // Overlay menu state: one row per toggle, rects rebuilt every draw.
    struct FMenuRow
    {
        FString Label;
        FString Kind;   // "paths", "heatmap", "ionosphere", or "domain"
        FString Domain; // set for kind == "domain"
        bool bVisible = true;
        FVector2D Min = FVector2D::ZeroVector;
        FVector2D Max = FVector2D::ZeroVector;
    };
    TArray<FMenuRow> MenuRows;
    bool bOverlayMenuOpen = false;
    void ApplyMenuToggle(const FMenuRow& Row);

    // False whenever DrawHUD bailed out before drawing anything. Clicks must
    // not be answered by hit-rects the user cannot see.
    bool bHudDrawn = false;
    void ForgetHitRects();

    // Search overlay state (Part A). The whole panel is one text field: it
    // captures input for as long as it is open, unlike the settings panel's
    // per-row editing.
    bool bSearchOpen = false;
    FString SearchQuery;
    int32 SearchHighlightIndex = 0;
    TArray<FGeoSearchResult> CachedSearchResults;
    FString LastScannedQuery;
    double LastSearchRefreshSeconds = -1000.0;
    struct FSearchResultRow
    {
        FVector2D Min = FVector2D::ZeroVector;
        FVector2D Max = FVector2D::ZeroVector;
        int32 ResultIndex = INDEX_NONE;
    };
    TArray<FSearchResultRow> SearchResultRows;
    FVector2D SearchWatchButtonMin = FVector2D::ZeroVector;
    FVector2D SearchWatchButtonMax = FVector2D::ZeroVector;
    bool bSearchWatchButtonEnabled = false;
    void RefreshSearchResults(bool bForceRescan);
    void DrawSearchOverlay(float Scale, float Alpha);
    bool HandleSearchClick(const FVector2D& ScreenPosition);
    void FocusSearchResult(int32 Index);
    void AddCurrentQueryAsWatch();

    // Watch/alert panel state (Part B).
    bool bWatchPanelOpen = false;
    struct FWatchHitRow
    {
        FVector2D Min = FVector2D::ZeroVector;
        FVector2D Max = FVector2D::ZeroVector;
        FString Query;
    };
    TArray<FWatchHitRow> WatchRemoveRows;
    struct FAlertHitRow
    {
        FVector2D Min = FVector2D::ZeroVector;
        FVector2D Max = FVector2D::ZeroVector;
        int32 AlertIndex = INDEX_NONE;
    };
    TArray<FAlertHitRow> AlertHitRows;
    void DrawWatchPanel(float Scale, float Alpha);
    bool HandleWatchClick(const FVector2D& ScreenPosition);
    void FocusAlert(int32 Index);

    // Settings panel state.
    struct FSettingsRow
    {
        FString Label;
        FString Key;   // callsign, grid, lifetime, minfl, ground, invertY, close
        FString Value; // display string
        bool bText = false;
        FVector2D Min = FVector2D::ZeroVector;
        FVector2D Max = FVector2D::ZeroVector;
    };
    TArray<FSettingsRow> SettingsRows;
    bool bSettingsOpen = false;
    bool bSettingsLoaded = false;
    int32 EditingRow = -1; // index of the text row being edited, -1 = none
    FString EditBuffer;
    void DrawSettings(float Scale, float Alpha);
    bool HandleSettingsClick(const FVector2D& ScreenPosition);
    void CycleSetting(const FString& Key);
    void CommitTextField(const FString& Key, const FString& Value);
    void LoadAndApplySettings();
    void PersistSetting(const TCHAR* Section, const TCHAR* Field, const FString& Value);
    AGeoPointLayerActor* FindPointLayer() const;

    // Hover tooltip cache (picked on a short throttle, drawn every frame).
    FString HoverTitle;
    FString HoverPrimary;
    FString HoverSecondary;
    FString HoverTertiary;
    FString HoverDomain;
    double LastHoverPickSeconds = 0.0;
    // Screen position the cached pick was taken at, so the tooltip anchors to
    // the marker and invalidates the moment the cursor leaves it, instead of
    // trailing empty sky for up to a throttle interval (audit finding #11).
    float LastHoverPickX = 0.0f;
    float LastHoverPickY = 0.0f;
    bool bHoverValid = false;
};
