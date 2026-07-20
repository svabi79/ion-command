#include "IonCockpitHudActor.h"

#include "CanvasItem.h"
#include "Engine/Canvas.h"
#include "Engine/Engine.h"
#include "Engine/Font.h"
#include "EngineUtils.h"
#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoPointLayerActor.h"
#include "GeoSelectionSubsystem.h"
#include "GeoStreamSubsystem.h"
#include "GeoTimelineSubsystem.h"
#include "HAL/PlatformTime.h"
#include "IonActivityHeatmapActor.h"
#include "IonIonosphereActor.h"
#include "Misc/CommandLine.h"
#include "Misc/Parse.h"

namespace
{
constexpr double GlobeRadiusUnits = 1000.0;
constexpr double LabelRadiusUnits = 1012.0;
constexpr int32 MaxGlobeLabels = 12;
// Keep more candidates than drawn labels: the busiest stations may all sit on
// the far side of the globe, and the label pass draws the busiest VISIBLE ones.
constexpr int32 MaxLabelCandidates = 48;

const FLinearColor CockpitCyan(0.16f, 0.86f, 1.0f);
const FLinearColor CockpitDim(0.10f, 0.38f, 0.52f);
const FLinearColor CockpitWhite(0.88f, 0.99f, 1.0f);
const FLinearColor CockpitGreen(0.24f, 1.0f, 0.60f);
const FLinearColor CockpitAmber(1.0f, 0.63f, 0.12f);
const FLinearColor CockpitRed(1.0f, 0.25f, 0.20f);
const FLinearColor PanelFill(0.008f, 0.03f, 0.05f);

// Properties arrive flattened to strings; JSON null becomes the literal
// string "null" and absent keys become empty.
bool PropertyAsDouble(const TMap<FString, FString>& Properties, const TCHAR* Key, double& OutValue)
{
    const FString Raw = Properties.FindRef(Key);
    if (Raw.IsEmpty() || Raw == TEXT("null")) return false;
    OutValue = FCString::Atod(*Raw);
    return true;
}

FLinearColor SeverityColor(double Value, double AmberFrom, double RedFrom)
{
    if (Value >= RedFrom) return CockpitRed;
    if (Value >= AmberFrom) return CockpitAmber;
    return CockpitGreen;
}

FLinearColor WithAlpha(const FLinearColor& Color, float Alpha)
{
    FLinearColor Result = Color;
    Result.A = Alpha;
    return Result;
}

// Simplified region flags for the top-regions panel: stripes, Nordic or
// plain crosses, center discs, and the US canton, keyed by the generic
// display-region name. Approximations by intent - readable at 22x14 px.
enum class ERegionFlagType : uint8 { HStripes2, HStripes3, VStripes3, Cross, NordicCross, Disc, Canton };

struct FRegionFlag
{
    ERegionFlagType Type;
    FLinearColor A;
    FLinearColor B;
    FLinearColor C;
};

const TMap<FString, FRegionFlag>& RegionFlags()
{
    static TMap<FString, FRegionFlag> Flags;
    if (Flags.Num() > 0) return Flags;
    const FLinearColor White(0.95f, 0.95f, 0.95f);
    const FLinearColor Red(0.82f, 0.06f, 0.10f);
    const FLinearColor Blue(0.05f, 0.20f, 0.55f);
    const FLinearColor LightBlue(0.35f, 0.60f, 0.90f);
    const FLinearColor Yellow(0.98f, 0.83f, 0.10f);
    const FLinearColor Green(0.05f, 0.50f, 0.20f);
    const FLinearColor Black(0.05f, 0.05f, 0.05f);
    const FLinearColor Orange(0.95f, 0.50f, 0.10f);
    auto H3 = [](FLinearColor A, FLinearColor B, FLinearColor C) { return FRegionFlag{ERegionFlagType::HStripes3, A, B, C}; };
    auto H2 = [](FLinearColor A, FLinearColor B) { return FRegionFlag{ERegionFlagType::HStripes2, A, B, B}; };
    auto V3 = [](FLinearColor A, FLinearColor B, FLinearColor C) { return FRegionFlag{ERegionFlagType::VStripes3, A, B, C}; };
    Flags.Add(TEXT("germany"), H3(Black, Red, Yellow));
    Flags.Add(TEXT("united states"), {ERegionFlagType::Canton, Red, White, Blue});
    Flags.Add(TEXT("japan"), {ERegionFlagType::Disc, White, Red, Red});
    Flags.Add(TEXT("italy"), V3(Green, White, Red));
    Flags.Add(TEXT("france"), V3(Blue, White, Red));
    Flags.Add(TEXT("spain"), H3(Red, Yellow, Red));
    Flags.Add(TEXT("england"), {ERegionFlagType::Cross, White, Red, Red});
    Flags.Add(TEXT("scotland"), {ERegionFlagType::Cross, Blue, White, White});
    Flags.Add(TEXT("wales"), H2(White, Green));
    Flags.Add(TEXT("northern ireland"), {ERegionFlagType::Cross, White, Red, Red});
    Flags.Add(TEXT("ireland"), V3(Green, White, Orange));
    Flags.Add(TEXT("poland"), H2(White, Red));
    Flags.Add(TEXT("european russia"), H3(White, Blue, Red));
    Flags.Add(TEXT("asiatic russia"), H3(White, Blue, Red));
    Flags.Add(TEXT("kaliningrad"), H3(White, Blue, Red));
    Flags.Add(TEXT("netherlands"), H3(Red, White, Blue));
    Flags.Add(TEXT("belgium"), V3(Black, Yellow, Red));
    Flags.Add(TEXT("switzerland"), {ERegionFlagType::Cross, Red, White, White});
    Flags.Add(TEXT("austria"), H3(Red, White, Red));
    Flags.Add(TEXT("canada"), V3(Red, White, Red));
    Flags.Add(TEXT("brazil"), {ERegionFlagType::Disc, Green, Yellow, Yellow});
    Flags.Add(TEXT("argentina"), H3(LightBlue, White, LightBlue));
    Flags.Add(TEXT("australia"), {ERegionFlagType::Canton, Blue, Blue, Blue});
    Flags.Add(TEXT("new zealand"), {ERegionFlagType::Canton, Blue, Blue, Blue});
    Flags.Add(TEXT("ukraine"), H2(LightBlue, Yellow));
    Flags.Add(TEXT("czech republic"), H2(White, Red));
    Flags.Add(TEXT("slovak republic"), H3(White, Blue, Red));
    Flags.Add(TEXT("hungary"), H3(Red, White, Green));
    Flags.Add(TEXT("romania"), V3(Blue, Yellow, Red));
    Flags.Add(TEXT("bulgaria"), H3(White, Green, Red));
    Flags.Add(TEXT("greece"), H3(Blue, White, Blue));
    Flags.Add(TEXT("portugal"), V3(Green, Red, Red));
    Flags.Add(TEXT("sweden"), {ERegionFlagType::NordicCross, Blue, Yellow, Yellow});
    Flags.Add(TEXT("norway"), {ERegionFlagType::NordicCross, Red, White, White});
    Flags.Add(TEXT("denmark"), {ERegionFlagType::NordicCross, Red, White, White});
    Flags.Add(TEXT("finland"), {ERegionFlagType::NordicCross, White, Blue, Blue});
    Flags.Add(TEXT("iceland"), {ERegionFlagType::NordicCross, Blue, White, White});
    Flags.Add(TEXT("china"), {ERegionFlagType::Disc, Red, Yellow, Yellow});
    Flags.Add(TEXT("republic of korea"), {ERegionFlagType::Disc, White, Red, Blue});
    Flags.Add(TEXT("indonesia"), H2(Red, White));
    Flags.Add(TEXT("thailand"), H3(Red, White, Blue));
    Flags.Add(TEXT("india"), H3(Orange, White, Green));
    Flags.Add(TEXT("israel"), {ERegionFlagType::Disc, White, Blue, Blue});
    Flags.Add(TEXT("turkey"), {ERegionFlagType::Disc, Red, White, White});
    Flags.Add(TEXT("mexico"), V3(Green, White, Red));
    Flags.Add(TEXT("colombia"), H3(Yellow, Blue, Red));
    Flags.Add(TEXT("cuba"), H3(Blue, White, Blue));
    Flags.Add(TEXT("croatia"), H3(Red, White, Blue));
    Flags.Add(TEXT("slovenia"), H3(White, Blue, Red));
    Flags.Add(TEXT("serbia"), H3(Red, Blue, White));
    Flags.Add(TEXT("lithuania"), H3(Yellow, Green, Red));
    Flags.Add(TEXT("latvia"), H3(Red, White, Red));
    Flags.Add(TEXT("estonia"), H3(LightBlue, Black, White));
    Flags.Add(TEXT("belarus"), H2(Red, Green));
    Flags.Add(TEXT("luxembourg"), H3(Red, White, LightBlue));
    return Flags;
}
}

AIonCockpitHudActor::AIonCockpitHudActor()
{
}

void AIonCockpitHudActor::BeginPlay()
{
    Super::BeginPlay();
    // -IonOverlayMenu opens the menu at startup for unattended captures.
    bOverlayMenuOpen = FParse::Param(FCommandLine::Get(), TEXT("IonOverlayMenu"));
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>())
        {
            DataSubsystem = Data;
            MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &AIonCockpitHudActor::OnMessageAccepted);
            DataResetHandle = Data->OnDataReset().AddUObject(this, &AIonCockpitHudActor::OnDataReset);
        }
    }
}

void AIonCockpitHudActor::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (UGeoDataSubsystem* Data = DataSubsystem.Get())
    {
        Data->OnMessageAccepted().Remove(MessageAcceptedHandle);
        Data->OnDataReset().Remove(DataResetHandle);
    }
    Super::EndPlay(EndPlayReason);
}

void AIonCockpitHudActor::CycleMode()
{
    switch (Mode)
    {
    case EIonCockpitMode::Full: Mode = EIonCockpitMode::Minimal; break;
    case EIonCockpitMode::Minimal: Mode = EIonCockpitMode::Hidden; break;
    default: Mode = EIonCockpitMode::Full; break;
    }
}

void AIonCockpitHudActor::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    if (Message.SemanticType == TEXT("spaceweather.state"))
    {
        double Value = 0.0;
        if (PropertyAsDouble(Message.Properties, TEXT("kp"), Value)) EnvKp = FMath::Clamp(Value, 0.0, 9.0);
        if (PropertyAsDouble(Message.Properties, TEXT("aIndex"), Value)) EnvAIndex = Value;
        if (PropertyAsDouble(Message.Properties, TEXT("solarFlux"), Value)) EnvSolarFlux = Value;
        if (PropertyAsDouble(Message.Properties, TEXT("solarWindSpeedKms"), Value)) EnvWindKms = Value;
        if (PropertyAsDouble(Message.Properties, TEXT("imfBzNt"), Value)) { EnvBzNt = Value; bHasEnvBz = true; }
        const FString XrayClass = Message.Properties.FindRef(TEXT("xrayClass"));
        if (!XrayClass.IsEmpty() && XrayClass != TEXT("null")) EnvXrayClass = XrayClass;
        return;
    }
    const bool bPathTraffic = (Message.Geometry.Type == EGeoGeometryType::GreatCircle || Message.Geometry.Type == EGeoGeometryType::Arc) && Message.Geometry.Positions.Num() >= 2;
    if (!bPathTraffic) return;
    const int64 NowSecond = static_cast<int64>(FPlatformTime::Seconds());
    AdvanceRateBuckets(NowSecond);
    ++RateBuckets[NowSecond % 60];
    RecordEndpoint(Message.FromEntityId, Message.Properties.FindRef(TEXT("display.from")), Message.Geometry.Positions[0]);
    RecordEndpoint(Message.ToEntityId, Message.Properties.FindRef(TEXT("display.to")), Message.Geometry.Positions.Last());
    auto RecordRegion = [this](const FString& Region)
    {
        if (Region.IsEmpty()) return;
        if (double* Weight = RegionWeights.Find(Region)) { *Weight += 1.0; return; }
        if (RegionWeights.Num() < MaxRegionStats) RegionWeights.Add(Region, 1.0);
    };
    RecordRegion(Message.Properties.FindRef(TEXT("display.fromRegion")));
    RecordRegion(Message.Properties.FindRef(TEXT("display.toRegion")));
}

void AIonCockpitHudActor::OnDataReset()
{
    EndpointStats.Reset();
    RegionWeights.Reset();
    CachedTopRegions.Reset();
    CachedTopEndpoints.Reset();
    CachedBreakdown.Reset();
    FMemory::Memzero(RateBuckets, sizeof(RateBuckets));
    LastBucketSecond = 0;
}

void AIonCockpitHudActor::AdvanceRateBuckets(int64 NowSecond)
{
    if (LastBucketSecond == 0 || NowSecond - LastBucketSecond >= 60)
    {
        FMemory::Memzero(RateBuckets, sizeof(RateBuckets));
        LastBucketSecond = NowSecond;
        return;
    }
    while (LastBucketSecond < NowSecond)
    {
        ++LastBucketSecond;
        RateBuckets[LastBucketSecond % 60] = 0;
    }
}

void AIonCockpitHudActor::RecordEndpoint(const FString& EntityId, const FString& Label, const FGeoPosition& Position)
{
    if (EntityId.IsEmpty() || Label.IsEmpty()) return;
    FIonEndpointStat* Stat = EndpointStats.Find(EntityId);
    if (!Stat)
    {
        if (EndpointStats.Num() >= MaxEndpointStats) return;
        Stat = &EndpointStats.Add(EntityId);
        Stat->Label = Label;
    }
    Stat->Position = Position;
    Stat->Weight += 1.0;
    Stat->LastSeenSeconds = FPlatformTime::Seconds();
}

void AIonCockpitHudActor::RefreshAggregates(double NowSeconds)
{
    if (!ArcLayer.IsValid())
    {
        for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It) { ArcLayer = *It; break; }
    }
    if (AGeoArcLayerActor* Layer = ArcLayer.Get())
    {
        Layer->GetPaletteBreakdown(CachedBreakdown);
        CachedPanelTitle = Layer->GetTrafficPanelTitle();
        CachedBandFocus = Layer->GetBandFocus();
    }
    // Decay endpoint weights so the label set follows current traffic, and
    // prune silent or negligible stations to keep the map bounded.
    for (auto It = EndpointStats.CreateIterator(); It; ++It)
    {
        It->Value.Weight *= 0.97;
        if (It->Value.Weight < 0.05 || NowSeconds - It->Value.LastSeenSeconds > EndpointRetentionSeconds) It.RemoveCurrent();
    }
    CachedTopEndpoints.Reset();
    for (const TPair<FString, FIonEndpointStat>& Pair : EndpointStats) CachedTopEndpoints.Add(Pair.Value);
    CachedTopEndpoints.Sort([](const FIonEndpointStat& A, const FIonEndpointStat& B) { return A.Weight > B.Weight; });
    if (CachedTopEndpoints.Num() > MaxLabelCandidates) CachedTopEndpoints.SetNum(MaxLabelCandidates);

    // Same decay treatment for the region tallies.
    CachedRegionTotal = 0.0;
    for (auto It = RegionWeights.CreateIterator(); It; ++It)
    {
        It->Value *= 0.97;
        if (It->Value < 0.05) { It.RemoveCurrent(); continue; }
        CachedRegionTotal += It->Value;
    }
    CachedTopRegions.Reset();
    for (const TPair<FString, double>& Pair : RegionWeights) CachedTopRegions.Add({Pair.Key, Pair.Value});
    CachedTopRegions.Sort([](const FIonRegionStat& A, const FIonRegionStat& B) { return A.Weight > B.Weight; });
    if (CachedTopRegions.Num() > 8) CachedTopRegions.SetNum(8);

    CachedPanels.Reset();
    if (const UIonCockpitPanelSubsystem* Panels = GetGameInstance() ? GetGameInstance()->GetSubsystem<UIonCockpitPanelSubsystem>() : nullptr)
    {
        CachedPanels = Panels->CollectPanels();
    }
}

void AIonCockpitHudActor::DrawHUD()
{
    Super::DrawHUD();
    if (!Canvas || Mode == EIonCockpitMode::Hidden) return;
    const double NowSeconds = FPlatformTime::Seconds();
    AdvanceRateBuckets(static_cast<int64>(NowSeconds));
    if (NowSeconds - LastAggregateSeconds > 0.5)
    {
        LastAggregateSeconds = NowSeconds;
        RefreshAggregates(NowSeconds);
    }
    const float Scale = FMath::Clamp(Canvas->SizeY / 1440.0f, 0.5f, 2.5f);
    // Match the boot camera fade so the cockpit materialises with the scene.
    const double WorldSeconds = GetWorld() ? GetWorld()->GetTimeSeconds() : 10.0;
    const float Alpha = FMath::Clamp((static_cast<float>(WorldSeconds) - 2.5f) / 2.5f, 0.0f, 1.0f);
    if (Alpha <= 0.0f) return;

    DrawStatusBar(Scale, Alpha);
    if (Mode == EIonCockpitMode::Full)
    {
        DrawTrafficPanel(Scale, Alpha);
        DrawRatePanel(Scale, Alpha, TrafficPanelBottomY + 16.0f * Scale);
        DrawRegionsPanel(Scale, Alpha, RatePanelBottomY + 16.0f * Scale);
        DrawPolarPanel(Scale, Alpha);
        DrawProviderPanels(Scale, Alpha, PolarPanelBottomY + 16.0f * Scale);
        DrawEndpointLabels(Scale, Alpha);
    }
    if (bOverlayMenuOpen) DrawOverlayMenu(Scale, Alpha);
    DrawHoverTooltip(Scale, Alpha);
    DrawModeHint(Scale, Alpha);
}

void AIonCockpitHudActor::DrawStatusBar(float Scale, float Alpha)
{
    const float BarHeight = 46.0f * Scale;
    DrawRect(0.0f, 0.0f, Canvas->SizeX, BarHeight, WithAlpha(PanelFill, 0.55f * Alpha));
    DrawLineSegment(FVector2D(0.0f, BarHeight), FVector2D(Canvas->SizeX, BarHeight), WithAlpha(CockpitCyan, 0.55f * Alpha), FMath::Max(1.0f, 1.5f * Scale));

    const UGeoTimelineSubsystem* Timeline = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>() : nullptr;
    const UGeoStreamSubsystem* Stream = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoStreamSubsystem>() : nullptr;
    const UGeoDataSubsystem* Data = DataSubsystem.Get();

    const float TextY = 12.0f * Scale;
    float CursorX = 18.0f * Scale;

    const FDateTime Utc = Timeline ? Timeline->GetTimelineUtc() : FDateTime::UtcNow();
    DrawLabelValue(CursorX, TextY, TEXT("UTC"), Utc.ToString(TEXT("%H:%M:%S")), CockpitWhite, Scale, Alpha);

    FString TimelineState = TEXT("LIVE");
    FLinearColor TimelineColor = CockpitGreen;
    if (Timeline && Timeline->IsPaused()) { TimelineState = TEXT("PAUSED"); TimelineColor = CockpitAmber; }
    else if (Timeline && !Timeline->IsLive()) { TimelineState = TEXT("REPLAY"); TimelineColor = CockpitCyan; }
    DrawLabelValue(CursorX, TextY, TEXT("MODE"), TimelineState, TimelineColor, Scale, Alpha);

    FString LinkState = TEXT("OFFLINE");
    FLinearColor LinkColor = CockpitAmber;
    if (Stream)
    {
        switch (Stream->GetState())
        {
        case EGeoStreamState::Connected: LinkState = TEXT("CONNECTED"); LinkColor = CockpitGreen; break;
        case EGeoStreamState::Connecting: LinkState = TEXT("CONNECTING"); LinkColor = CockpitCyan; break;
        case EGeoStreamState::Degraded: LinkState = TEXT("DEGRADED"); LinkColor = CockpitAmber; break;
        default: break;
        }
    }
    DrawLabelValue(CursorX, TextY, TEXT("LINK"), LinkState, LinkColor, Scale, Alpha);

    DrawLabelValue(CursorX, TextY, TEXT("KP"), EnvKp < 0.0 ? TEXT("--") : FString::Printf(TEXT("%.1f"), EnvKp), EnvKp < 0.0 ? CockpitDim : SeverityColor(EnvKp, 4.0, 6.0), Scale, Alpha);
    DrawLabelValue(CursorX, TextY, TEXT("FLUX"), EnvSolarFlux < 0.0 ? TEXT("--") : FString::Printf(TEXT("%.0f"), EnvSolarFlux), EnvSolarFlux < 0.0 ? CockpitDim : CockpitWhite, Scale, Alpha);
    DrawLabelValue(CursorX, TextY, TEXT("A"), EnvAIndex < 0.0 ? TEXT("--") : FString::Printf(TEXT("%.0f"), EnvAIndex), EnvAIndex < 0.0 ? CockpitDim : SeverityColor(EnvAIndex, 15.0, 30.0), Scale, Alpha);
    DrawLabelValue(CursorX, TextY, TEXT("WIND"), EnvWindKms < 0.0 ? TEXT("--") : FString::Printf(TEXT("%.0f KM/S"), EnvWindKms), EnvWindKms < 0.0 ? CockpitDim : CockpitWhite, Scale, Alpha);
    DrawLabelValue(CursorX, TextY, TEXT("BZ"), !bHasEnvBz ? TEXT("--") : FString::Printf(TEXT("%+.1f NT"), EnvBzNt), !bHasEnvBz ? CockpitDim : SeverityColor(-EnvBzNt, 2.0, 5.0), Scale, Alpha);
    FLinearColor XrayColor = CockpitDim;
    if (!EnvXrayClass.IsEmpty())
    {
        switch (EnvXrayClass[0])
        {
        case 'X': XrayColor = CockpitRed; break;
        case 'M': XrayColor = CockpitAmber; break;
        case 'C': XrayColor = FLinearColor(1.0f, 0.9f, 0.2f); break;
        default: XrayColor = CockpitGreen; break;
        }
    }
    DrawLabelValue(CursorX, TextY, TEXT("XRAY"), EnvXrayClass.IsEmpty() ? TEXT("--") : EnvXrayClass, XrayColor, Scale, Alpha);

    int32 PerMinute = 0;
    for (int32 Bucket : RateBuckets) PerMinute += Bucket;
    DrawLabelValue(CursorX, TextY, TEXT("PATHS/MIN"), FString::Printf(TEXT("%d"), PerMinute), CockpitCyan, Scale, Alpha);

    if (const AGeoArcLayerActor* Layer = ArcLayer.Get())
    {
        if (!Layer->GetPropertyFilterValue().IsEmpty())
        {
            DrawLabelValue(CursorX, TextY, TEXT("FILTER"), Layer->GetPropertyFilterValue().ToUpper(), CockpitAmber, Scale, Alpha);
        }
    }

    if (Data)
    {
        const FGeoRuntimeStatistics Stats = Data->GetStatistics();
        DrawLabelValue(CursorX, TextY, TEXT("ACTIVE"), FString::Printf(TEXT("%d"), Stats.ActiveMessages), CockpitWhite, Scale, Alpha);
        DrawLabelValue(CursorX, TextY, TEXT("RX"), FString::Printf(TEXT("%lld"), Stats.AcceptedMessages), CockpitWhite, Scale, Alpha);
        DrawLabelValue(CursorX, TextY, TEXT("DROP"), FString::Printf(TEXT("%lld"), Stats.DroppedMessages), Stats.DroppedMessages > 0 ? CockpitRed : CockpitGreen, Scale, Alpha);
        DrawLabelValue(CursorX, TextY, TEXT("EVICT"), FString::Printf(TEXT("%lld"), Stats.EvictedMessages), CockpitDim, Scale, Alpha);
    }
}

void AIonCockpitHudActor::DrawTrafficPanel(float Scale, float Alpha)
{
    const float PanelX = 18.0f * Scale;
    const float PanelY = 66.0f * Scale;
    const float PanelWidth = 330.0f * Scale;
    const float RowHeight = 24.0f * Scale;
    const float HeaderHeight = 34.0f * Scale;
    const int32 Rows = CachedBreakdown.Num();
    const float PanelHeight = HeaderHeight + FMath::Max(Rows, 1) * RowHeight + 12.0f * Scale;
    DrawPanelFrame(PanelX, PanelY, PanelWidth, PanelHeight, CachedPanelTitle.IsEmpty() ? TEXT("TRAFFIC BY CLASS") : CachedPanelTitle, Scale, Alpha);
    TrafficPanelBottomY = PanelY + PanelHeight;

    int32 MaxCount = 1;
    for (const FGeoPaletteBreakdownEntry& Entry : CachedBreakdown) MaxCount = FMath::Max(MaxCount, Entry.Count);
    const float LabelWidth = 86.0f * Scale;
    const float CountWidth = 56.0f * Scale;
    const float BarMaxWidth = PanelWidth - LabelWidth - CountWidth - 30.0f * Scale;
    float RowY = PanelY + HeaderHeight;
    for (const FGeoPaletteBreakdownEntry& Entry : CachedBreakdown)
    {
        const bool bDimmed = CachedBandFocus != INDEX_NONE && Entry.PaletteIndex != CachedBandFocus;
        const float RowAlpha = Alpha * (bDimmed ? 0.28f : 1.0f);
        DrawTextAt(Entry.Label, PanelX + 12.0f * Scale, RowY + 4.0f * Scale, WithAlpha(CockpitWhite, RowAlpha), 1.15f * Scale);
        const float BarWidth = BarMaxWidth * (MaxCount > 0 ? static_cast<float>(Entry.Count) / MaxCount : 0.0f);
        DrawRect(PanelX + LabelWidth, RowY + 6.0f * Scale, FMath::Max(BarWidth, Entry.Count > 0 ? 2.0f * Scale : 0.0f), RowHeight - 12.0f * Scale, WithAlpha(Entry.Color, 0.85f * RowAlpha));
        DrawTextAt(FString::Printf(TEXT("%d"), Entry.Count), PanelX + LabelWidth + BarMaxWidth + 10.0f * Scale, RowY + 4.0f * Scale, WithAlpha(CockpitCyan, RowAlpha), 1.15f * Scale);
        if (CachedBandFocus == Entry.PaletteIndex)
        {
            DrawRect(PanelX + 4.0f * Scale, RowY + 6.0f * Scale, 3.0f * Scale, RowHeight - 12.0f * Scale, WithAlpha(CockpitWhite, Alpha));
        }
        RowY += RowHeight;
    }
}

void AIonCockpitHudActor::DrawRatePanel(float Scale, float Alpha, float PanelY)
{
    const float PanelX = 18.0f * Scale;
    const float PanelWidth = 330.0f * Scale;
    const float PanelHeight = 96.0f * Scale;
    DrawPanelFrame(PanelX, PanelY, PanelWidth, PanelHeight, TEXT("PATH RATE // 60 S"), Scale, Alpha);

    int32 MaxBucket = 1;
    for (int32 Bucket : RateBuckets) MaxBucket = FMath::Max(MaxBucket, Bucket);
    const float ChartX = PanelX + 12.0f * Scale;
    const float ChartWidth = PanelWidth - 24.0f * Scale;
    const float ChartBottom = PanelY + PanelHeight - 10.0f * Scale;
    const float ChartMaxHeight = PanelHeight - 46.0f * Scale;
    const float BarStep = ChartWidth / 60.0f;
    for (int32 Offset = 0; Offset < 60; ++Offset)
    {
        // Oldest bucket on the left, the current second on the right.
        const int64 BucketSecond = LastBucketSecond - 59 + Offset;
        const int32 Count = BucketSecond > 0 ? RateBuckets[BucketSecond % 60] : 0;
        if (Count <= 0) continue;
        const float BarHeight = FMath::Max(ChartMaxHeight * Count / MaxBucket, 1.5f * Scale);
        DrawRect(ChartX + Offset * BarStep, ChartBottom - BarHeight, FMath::Max(BarStep - 1.0f, 1.0f), BarHeight, WithAlpha(CockpitCyan, 0.8f * Alpha));
    }
    RatePanelBottomY = PanelY + PanelHeight;
}

void AIonCockpitHudActor::DrawRegionsPanel(float Scale, float Alpha, float PanelY)
{
    const float PanelX = 18.0f * Scale;
    const float PanelWidth = 330.0f * Scale;
    const float RowHeight = 24.0f * Scale;
    const float HeaderHeight = 34.0f * Scale;
    const int32 Rows = FMath::Max(CachedTopRegions.Num(), 1);
    const float PanelHeight = HeaderHeight + Rows * RowHeight + 12.0f * Scale;
    DrawPanelFrame(PanelX, PanelY, PanelWidth, PanelHeight, TEXT("TOP REGIONS"), Scale, Alpha);
    if (CachedTopRegions.IsEmpty())
    {
        DrawTextAt(TEXT("AWAITING DATA"), PanelX + 12.0f * Scale, PanelY + HeaderHeight + 4.0f * Scale, WithAlpha(CockpitDim, Alpha), 1.15f * Scale);
        return;
    }
    const double MaxWeight = CachedTopRegions[0].Weight;
    const float FlagWidth = 22.0f * Scale;
    const float LabelWidth = 150.0f * Scale;
    const float ShareWidth = 52.0f * Scale;
    const float BarMaxWidth = PanelWidth - LabelWidth - ShareWidth - 30.0f * Scale;
    float RowY = PanelY + HeaderHeight;
    for (const FIonRegionStat& Region : CachedTopRegions)
    {
        float LabelX = PanelX + 12.0f * Scale;
        if (DrawRegionFlag(Region.Label, LabelX, RowY + 5.0f * Scale, FlagWidth, 13.0f * Scale, Alpha))
        {
            LabelX += FlagWidth + 6.0f * Scale;
        }
        FString Label = Region.Label.ToUpper();
        if (Label.Len() > 14) Label = Label.Left(13) + TEXT("~");
        DrawTextAt(Label, LabelX, RowY + 4.0f * Scale, WithAlpha(CockpitWhite, Alpha), 1.1f * Scale);
        const float BarWidth = BarMaxWidth * (MaxWeight > 0.0 ? static_cast<float>(Region.Weight / MaxWeight) : 0.0f);
        DrawRect(PanelX + LabelWidth, RowY + 6.0f * Scale, FMath::Max(BarWidth, 2.0f * Scale), RowHeight - 12.0f * Scale, WithAlpha(CockpitCyan, 0.75f * Alpha));
        const double Share = CachedRegionTotal > 0.0 ? 100.0 * Region.Weight / CachedRegionTotal : 0.0;
        DrawTextAt(FString::Printf(TEXT("%.0f%%"), Share), PanelX + LabelWidth + BarMaxWidth + 10.0f * Scale, RowY + 4.0f * Scale, WithAlpha(CockpitCyan, Alpha), 1.1f * Scale);
        RowY += RowHeight;
    }
}

void AIonCockpitHudActor::DrawPolarPanel(float Scale, float Alpha)
{
    const float PanelWidth = 250.0f * Scale;
    const float PanelHeight = 288.0f * Scale;
    const float PanelX = Canvas->SizeX - PanelWidth - 18.0f * Scale;
    const float PanelY = 66.0f * Scale;
    DrawPanelFrame(PanelX, PanelY, PanelWidth, PanelHeight, TEXT("AURORAL OVAL // N"), Scale, Alpha);

    const FVector2D Center(PanelX + PanelWidth * 0.5f, PanelY + 40.0f * Scale + (PanelHeight - 78.0f * Scale) * 0.5f);
    const float MaxRadius = (PanelHeight - 96.0f * Scale) * 0.5f;
    const float GridAlpha = 0.35f * Alpha;
    // Polar grid from latitude 90 (center) to 50 (rim).
    for (int32 LatitudeRing = 80; LatitudeRing >= 50; LatitudeRing -= 10)
    {
        const float Radius = MaxRadius * (90 - LatitudeRing) / 40.0f;
        FVector2D Previous = Center + FVector2D(Radius, 0.0f);
        for (int32 Segment = 1; Segment <= 48; ++Segment)
        {
            const float Angle = 2.0f * PI * Segment / 48.0f;
            const FVector2D Point = Center + FVector2D(FMath::Cos(Angle), FMath::Sin(Angle)) * Radius;
            DrawLineSegment(Previous, Point, WithAlpha(CockpitDim, GridAlpha), 1.0f);
            Previous = Point;
        }
    }
    DrawLineSegment(Center - FVector2D(MaxRadius, 0.0f), Center + FVector2D(MaxRadius, 0.0f), WithAlpha(CockpitDim, GridAlpha), 1.0f);
    DrawLineSegment(Center - FVector2D(0.0f, MaxRadius), Center + FVector2D(0.0f, MaxRadius), WithAlpha(CockpitDim, GridAlpha), 1.0f);

    // Same equatorward expansion as the 3D aurora oval.
    const bool bHasKp = EnvKp >= 0.0;
    const double OvalKp = bHasKp ? EnvKp : 2.0;
    const double CenterLatitude = FMath::Clamp(71.0 - 2.2 * OvalKp, 48.0, 74.0);
    const float OvalRadius = MaxRadius * (90.0f - static_cast<float>(CenterLatitude)) / 40.0f;
    const float OvalThickness = (2.5f + static_cast<float>(OvalKp) * 0.9f) * Scale;
    const FLinearColor OvalColor = bHasKp ? FLinearColor(0.35f, 1.0f, 0.55f) : CockpitDim;
    FVector2D Previous = Center + FVector2D(OvalRadius, 0.0f);
    for (int32 Segment = 1; Segment <= 72; ++Segment)
    {
        const float Angle = 2.0f * PI * Segment / 72.0f;
        const FVector2D Point = Center + FVector2D(FMath::Cos(Angle), FMath::Sin(Angle)) * OvalRadius;
        DrawLineSegment(Previous, Point, WithAlpha(OvalColor, (bHasKp ? 0.85f : 0.4f) * Alpha), OvalThickness);
        Previous = Point;
    }
    DrawTextAt(bHasKp ? FString::Printf(TEXT("KP %.1f  //  OVAL %.0f N"), EnvKp, CenterLatitude) : TEXT("KP --  //  AWAITING DATA"), Center.X, PanelY + PanelHeight - 24.0f * Scale, WithAlpha(bHasKp ? CockpitWhite : CockpitDim, Alpha), 1.1f * Scale, true);
    PolarPanelBottomY = PanelY + PanelHeight;
}

void AIonCockpitHudActor::DrawProviderPanels(float Scale, float Alpha, float PanelY)
{
    const float PanelWidth = 250.0f * Scale;
    const float PanelX = Canvas->SizeX - PanelWidth - 18.0f * Scale;
    const float RowHeight = 24.0f * Scale;
    const float HeaderHeight = 34.0f * Scale;
    float CursorY = PanelY;
    for (const FIonCockpitPanelModel& Panel : CachedPanels)
    {
        // Providers signal "nothing to show" with an untitled model.
        if (Panel.Title.IsEmpty()) continue;
        const int32 Rows = FMath::Max(Panel.Rows.Num(), 1);
        const float PanelHeight = HeaderHeight + Rows * RowHeight + 12.0f * Scale;
        if (CursorY + PanelHeight > Canvas->SizeY - 40.0f * Scale) break;
        DrawPanelFrame(PanelX, CursorY, PanelWidth, PanelHeight, Panel.Title, Scale, Alpha);
        float RowY = CursorY + HeaderHeight;
        for (const FIonCockpitPanelRow& Row : Panel.Rows)
        {
            DrawTextAt(Row.Label, PanelX + 12.0f * Scale, RowY + 4.0f * Scale, WithAlpha(CockpitWhite, Alpha), 1.1f * Scale);
            float CellX = PanelX + 92.0f * Scale;
            for (const FIonCockpitPanelCell& Cell : Row.Cells)
            {
                DrawTextAt(Cell.Text, CellX, RowY + 4.0f * Scale, WithAlpha(Cell.Color, Alpha), 1.1f * Scale);
                float CellW = 0.0f, CellH = 0.0f;
                Canvas->TextSize(GEngine->GetMediumFont(), Cell.Text, CellW, CellH, 1.1f * Scale, 1.1f * Scale);
                CellX += CellW + 12.0f * Scale;
            }
            RowY += RowHeight;
        }
        CursorY += PanelHeight + 16.0f * Scale;
    }
}

void AIonCockpitHudActor::DrawEndpointLabels(float Scale, float Alpha)
{
    APlayerController* Player = PlayerOwner.Get();
    if (!Player || !Player->PlayerCameraManager) return;
    const FVector CameraLocation = Player->PlayerCameraManager->GetCameraLocation();

    auto DrawStationLabel = [&](const FGeoPosition& Position, const FString& Label, const FLinearColor& Color, float TextScale) -> bool
    {
        const FVector Unit = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Position.Latitude, Position.Longitude);
        const FVector WorldPosition = Unit * LabelRadiusUnits;
        // Cull labels whose surface point faces away from the camera.
        if (FVector::DotProduct(Unit, (CameraLocation - WorldPosition).GetSafeNormal()) < 0.12) return false;
        const FVector Screen = Canvas->Project(WorldPosition);
        if (Screen.Z <= 0.0f) return false;
        DrawRect(Screen.X - 1.5f * Scale, Screen.Y - 1.5f * Scale, 3.0f * Scale, 3.0f * Scale, WithAlpha(Color, Alpha));
        DrawTextAt(Label, Screen.X + 6.0f * Scale, Screen.Y - 14.0f * Scale, WithAlpha(Color, Alpha), TextScale);
        return true;
    };

    // Busiest visible stations first; far-side candidates make room for the
    // next visible ones instead of consuming label slots.
    int32 DrawnLabels = 0;
    for (const FIonEndpointStat& Stat : CachedTopEndpoints)
    {
        if (DrawnLabels >= MaxGlobeLabels) break;
        if (DrawStationLabel(Stat.Position, Stat.Label, CockpitCyan, 1.05f * Scale)) ++DrawnLabels;
    }
    const UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr;
    if (Selection && Selection->HasSelection())
    {
        const FGeoMessageEnvelope Message = Selection->GetSelection();
        if (Message.Geometry.Positions.Num() >= 2)
        {
            const FString From = Message.Properties.FindRef(TEXT("display.from"));
            const FString To = Message.Properties.FindRef(TEXT("display.to"));
            if (!From.IsEmpty()) DrawStationLabel(Message.Geometry.Positions[0], From, CockpitWhite, 1.25f * Scale);
            if (!To.IsEmpty()) DrawStationLabel(Message.Geometry.Positions.Last(), To, CockpitWhite, 1.25f * Scale);
        }
    }
}

void AIonCockpitHudActor::DrawOverlayMenu(float Scale, float Alpha)
{
    // Rebuild the row list from the actual scene: fixed render layers first,
    // then every domain the point layer currently holds. New domains appear
    // automatically.
    MenuRows.Reset();
    AGeoPointLayerActor* PointLayer = nullptr;
    for (TActorIterator<AGeoPointLayerActor> It(GetWorld()); It; ++It) { PointLayer = *It; break; }
    if (const AGeoArcLayerActor* Layer = ArcLayer.Get())
    {
        MenuRows.Add({TEXT("PATHS"), TEXT("paths"), FString(), !Layer->IsHidden()});
    }
    for (TActorIterator<AIonActivityHeatmapActor> It(GetWorld()); It; ++It)
    {
        MenuRows.Add({TEXT("HEATMAP"), TEXT("heatmap"), FString(), !It->IsHidden()});
        break;
    }
    for (TActorIterator<AIonIonosphereActor> It(GetWorld()); It; ++It)
    {
        MenuRows.Add({TEXT("IONOSPHERE SHELLS"), TEXT("ionosphere"), FString(), !It->IsHidden()});
        break;
    }
    if (PointLayer)
    {
        MenuRows.Add({TEXT("ALT EXAGGERATION 12X"), TEXT("altscale"), FString(), PointLayer->IsAltitudeExaggerationEnabled()});
        TArray<FString> Domains;
        PointLayer->GetPresentDomains(Domains);
        for (const FString& Domain : Domains)
        {
            MenuRows.Add({Domain.ToUpper() + TEXT(" MARKERS"), TEXT("domain"), Domain, PointLayer->IsDomainVisible(Domain)});
        }
    }

    const float RowHeight = 26.0f * Scale;
    const float PanelWidth = 250.0f * Scale;
    const float HeaderHeight = 34.0f * Scale;
    const float PanelHeight = HeaderHeight + MenuRows.Num() * RowHeight + 12.0f * Scale;
    const float PanelX = 18.0f * Scale;
    const float PanelY = Canvas->SizeY - PanelHeight - 44.0f * Scale;
    DrawPanelFrame(PanelX, PanelY, PanelWidth, PanelHeight, TEXT("OVERLAYS // O"), Scale, Alpha);

    FVector2D Mouse(-1, -1);
    if (const APlayerController* Player = PlayerOwner.Get())
    {
        float MouseX = 0, MouseY = 0;
        if (Player->GetMousePosition(MouseX, MouseY)) Mouse = FVector2D(MouseX, MouseY);
    }
    float RowY = PanelY + HeaderHeight;
    for (FMenuRow& Row : MenuRows)
    {
        Row.Min = FVector2D(PanelX, RowY);
        Row.Max = FVector2D(PanelX + PanelWidth, RowY + RowHeight);
        const bool bHovered = Mouse.X >= Row.Min.X && Mouse.X <= Row.Max.X && Mouse.Y >= Row.Min.Y && Mouse.Y <= Row.Max.Y;
        if (bHovered)
        {
            DrawRect(Row.Min.X + 2.0f * Scale, Row.Min.Y + 1.0f * Scale, PanelWidth - 4.0f * Scale, RowHeight - 2.0f * Scale, WithAlpha(CockpitCyan, 0.12f * Alpha));
        }
        const FLinearColor Mark = Row.bVisible ? CockpitGreen : CockpitDim;
        DrawTextAt(Row.bVisible ? TEXT("[x]") : TEXT("[ ]"), PanelX + 12.0f * Scale, RowY + 5.0f * Scale, WithAlpha(Mark, Alpha), 1.15f * Scale);
        DrawTextAt(Row.Label, PanelX + 48.0f * Scale, RowY + 5.0f * Scale, WithAlpha(Row.bVisible ? CockpitWhite : CockpitDim, Alpha), 1.15f * Scale);
        RowY += RowHeight;
    }
}

bool AIonCockpitHudActor::HandleClick(const FVector2D& ScreenPosition)
{
    if (!bOverlayMenuOpen) return false;
    for (const FMenuRow& Row : MenuRows)
    {
        if (ScreenPosition.X >= Row.Min.X && ScreenPosition.X <= Row.Max.X && ScreenPosition.Y >= Row.Min.Y && ScreenPosition.Y <= Row.Max.Y)
        {
            ApplyMenuToggle(Row);
            return true;
        }
    }
    return false;
}

void AIonCockpitHudActor::ApplyMenuToggle(const FMenuRow& Row)
{
    if (Row.Kind == TEXT("paths"))
    {
        for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It) It->SetActorHiddenInGame(!It->IsHidden());
    }
    else if (Row.Kind == TEXT("heatmap"))
    {
        for (TActorIterator<AIonActivityHeatmapActor> It(GetWorld()); It; ++It) It->SetActorHiddenInGame(!It->IsHidden());
    }
    else if (Row.Kind == TEXT("ionosphere"))
    {
        for (TActorIterator<AIonIonosphereActor> It(GetWorld()); It; ++It) It->SetActorHiddenInGame(!It->IsHidden());
    }
    else if (Row.Kind == TEXT("altscale"))
    {
        for (TActorIterator<AGeoPointLayerActor> It(GetWorld()); It; ++It) It->SetAltitudeExaggerationEnabled(!It->IsAltitudeExaggerationEnabled());
    }
    else if (Row.Kind == TEXT("domain"))
    {
        for (TActorIterator<AGeoPointLayerActor> It(GetWorld()); It; ++It) It->SetDomainVisible(Row.Domain, !It->IsDomainVisible(Row.Domain));
    }
}

void AIonCockpitHudActor::DrawHoverTooltip(float Scale, float Alpha)
{
    APlayerController* Player = PlayerOwner.Get();
    if (!Player) return;
    float MouseX = 0, MouseY = 0;
    bool bHaveMouse = Player->GetMousePosition(MouseX, MouseY);
    // -IonHoverProbe pins the probe to the screen center for unattended
    // captures of the tooltip path.
    if (!bHaveMouse && FParse::Param(FCommandLine::Get(), TEXT("IonHoverProbe")))
    {
        MouseX = Canvas->SizeX * 0.5f;
        MouseY = Canvas->SizeY * 0.5f;
        bHaveMouse = true;
    }
    if (!bHaveMouse) { bHoverValid = false; return; }

    const double NowSeconds = FPlatformTime::Seconds();
    if (NowSeconds - LastHoverPickSeconds > 0.15)
    {
        LastHoverPickSeconds = NowSeconds;
        bHoverValid = false;
        FVector RayOrigin, RayDirection;
        if (Player->DeprojectScreenPositionToWorld(MouseX, MouseY, RayOrigin, RayDirection))
        {
            for (TActorIterator<AGeoPointLayerActor> It(GetWorld()); It; ++It)
            {
                if (const FRenderedGeoPoint* Point = It->FindNearestToRay(RayOrigin, RayDirection, 14.0))
                {
                    HoverTitle = Point->Title;
                    HoverPrimary = Point->Primary;
                    HoverSecondary = Point->Secondary;
                    HoverDomain = Point->Domain.ToUpper();
                    bHoverValid = true;
                }
                break;
            }
        }
    }
    if (!bHoverValid) return;

    TArray<FString> Lines;
    if (!HoverTitle.IsEmpty()) Lines.Add(HoverTitle);
    if (!HoverPrimary.IsEmpty()) Lines.Add(HoverPrimary);
    if (!HoverSecondary.IsEmpty()) Lines.Add(HoverSecondary);
    if (!HoverDomain.IsEmpty()) Lines.Add(HoverDomain);
    if (Lines.IsEmpty()) return;
    const UFont* Font = GEngine->GetMediumFont();
    float Widest = 0.0f;
    for (const FString& Line : Lines)
    {
        float Width = 0, Height = 0;
        Canvas->TextSize(Font, Line, Width, Height, 1.15f * Scale, 1.15f * Scale);
        Widest = FMath::Max(Widest, Width);
    }
    const float LineHeight = 19.0f * Scale;
    const float BoxWidth = Widest + 24.0f * Scale;
    const float BoxHeight = Lines.Num() * LineHeight + 14.0f * Scale;
    float BoxX = MouseX + 18.0f * Scale;
    float BoxY = MouseY - BoxHeight - 10.0f * Scale;
    BoxX = FMath::Clamp(BoxX, 0.0f, Canvas->SizeX - BoxWidth);
    BoxY = FMath::Clamp(BoxY, 0.0f, Canvas->SizeY - BoxHeight);
    DrawRect(BoxX, BoxY, BoxWidth, BoxHeight, WithAlpha(PanelFill, 0.85f * Alpha));
    DrawLineSegment(FVector2D(BoxX, BoxY), FVector2D(BoxX + BoxWidth, BoxY), WithAlpha(CockpitCyan, 0.6f * Alpha), 1.0f);
    DrawLineSegment(FVector2D(BoxX, BoxY + BoxHeight), FVector2D(BoxX + BoxWidth, BoxY + BoxHeight), WithAlpha(CockpitCyan, 0.6f * Alpha), 1.0f);
    DrawLineSegment(FVector2D(BoxX, BoxY), FVector2D(BoxX, BoxY + BoxHeight), WithAlpha(CockpitCyan, 0.6f * Alpha), 1.0f);
    DrawLineSegment(FVector2D(BoxX + BoxWidth, BoxY), FVector2D(BoxX + BoxWidth, BoxY + BoxHeight), WithAlpha(CockpitCyan, 0.6f * Alpha), 1.0f);
    for (int32 Index = 0; Index < Lines.Num(); ++Index)
    {
        const FLinearColor Color = Index == 0 ? CockpitWhite : (Index == Lines.Num() - 1 ? CockpitDim : CockpitCyan);
        DrawTextAt(Lines[Index], BoxX + 12.0f * Scale, BoxY + 7.0f * Scale + Index * LineHeight, WithAlpha(Color, Alpha), 1.15f * Scale);
    }
}

void AIonCockpitHudActor::DrawModeHint(float Scale, float Alpha)
{
    const TCHAR* ModeName = Mode == EIonCockpitMode::Full ? TEXT("FULL") : TEXT("MIN");
    DrawTextAt(FString::Printf(TEXT("HUD %s // TAB   OVERLAYS // O"), ModeName), Canvas->SizeX - 18.0f * Scale, Canvas->SizeY - 26.0f * Scale, WithAlpha(CockpitDim, Alpha), 1.0f * Scale, true);
}

bool AIonCockpitHudActor::DrawRegionFlag(const FString& RegionName, float X, float Y, float Width, float Height, float Alpha)
{
    const FRegionFlag* Flag = RegionFlags().Find(RegionName.ToLower());
    if (!Flag) return false;
    const FLinearColor A = WithAlpha(Flag->A, Alpha);
    const FLinearColor B = WithAlpha(Flag->B, Alpha);
    const FLinearColor C = WithAlpha(Flag->C, Alpha);
    switch (Flag->Type)
    {
    case ERegionFlagType::HStripes2:
        DrawRect(X, Y, Width, Height * 0.5f, A);
        DrawRect(X, Y + Height * 0.5f, Width, Height * 0.5f, B);
        break;
    case ERegionFlagType::HStripes3:
        DrawRect(X, Y, Width, Height / 3.0f, A);
        DrawRect(X, Y + Height / 3.0f, Width, Height / 3.0f, B);
        DrawRect(X, Y + Height * 2.0f / 3.0f, Width, Height / 3.0f, C);
        break;
    case ERegionFlagType::VStripes3:
        DrawRect(X, Y, Width / 3.0f, Height, A);
        DrawRect(X + Width / 3.0f, Y, Width / 3.0f, Height, B);
        DrawRect(X + Width * 2.0f / 3.0f, Y, Width / 3.0f, Height, C);
        break;
    case ERegionFlagType::Cross:
        DrawRect(X, Y, Width, Height, A);
        DrawRect(X, Y + Height * 0.38f, Width, Height * 0.24f, B);
        DrawRect(X + Width * 0.40f, Y, Width * 0.20f, Height, B);
        break;
    case ERegionFlagType::NordicCross:
        DrawRect(X, Y, Width, Height, A);
        DrawRect(X, Y + Height * 0.38f, Width, Height * 0.24f, B);
        DrawRect(X + Width * 0.28f, Y, Width * 0.18f, Height, B);
        break;
    case ERegionFlagType::Disc:
        DrawRect(X, Y, Width, Height, A);
        DrawRect(X + Width * 0.38f, Y + Height * 0.22f, Width * 0.24f, Height * 0.56f, B);
        break;
    case ERegionFlagType::Canton:
        DrawRect(X, Y, Width, Height / 3.0f, A);
        DrawRect(X, Y + Height / 3.0f, Width, Height / 3.0f, B);
        DrawRect(X, Y + Height * 2.0f / 3.0f, Width, Height / 3.0f, A);
        DrawRect(X, Y, Width * 0.45f, Height * 0.5f, C);
        break;
    }
    return true;
}

void AIonCockpitHudActor::DrawPanelFrame(float X, float Y, float Width, float Height, const FString& Title, float Scale, float Alpha)
{
    DrawRect(X, Y, Width, Height, WithAlpha(PanelFill, 0.5f * Alpha));
    const FLinearColor Border = WithAlpha(CockpitCyan, 0.45f * Alpha);
    const float Thickness = FMath::Max(1.0f, 1.2f * Scale);
    DrawLineSegment(FVector2D(X, Y), FVector2D(X + Width, Y), Border, Thickness);
    DrawLineSegment(FVector2D(X, Y + Height), FVector2D(X + Width, Y + Height), Border, Thickness);
    DrawLineSegment(FVector2D(X, Y), FVector2D(X, Y + Height), Border, Thickness);
    DrawLineSegment(FVector2D(X + Width, Y), FVector2D(X + Width, Y + Height), Border, Thickness);
    DrawTextAt(Title, X + 12.0f * Scale, Y + 8.0f * Scale, WithAlpha(CockpitCyan, Alpha), 1.2f * Scale);
}

void AIonCockpitHudActor::DrawLabelValue(float& CursorX, float Y, const FString& Label, const FString& Value, const FLinearColor& ValueColor, float Scale, float Alpha)
{
    const UFont* Font = GEngine->GetMediumFont();
    const float LabelScale = 1.1f * Scale;
    const float ValueScale = 1.35f * Scale;
    float LabelW = 0.0f, LabelH = 0.0f, ValueW = 0.0f, ValueH = 0.0f;
    Canvas->TextSize(Font, Label, LabelW, LabelH, LabelScale, LabelScale);
    Canvas->TextSize(Font, Value, ValueW, ValueH, ValueScale, ValueScale);
    DrawTextAt(Label, CursorX, Y + (ValueH - LabelH), WithAlpha(CockpitDim, Alpha), LabelScale);
    DrawTextAt(Value, CursorX + LabelW + 7.0f * Scale, Y, WithAlpha(ValueColor, Alpha), ValueScale);
    CursorX += LabelW + ValueW + 33.0f * Scale;
}

void AIonCockpitHudActor::DrawTextAt(const FString& Text, float X, float Y, const FLinearColor& Color, float TextScale, bool bCentered)
{
    const UFont* Font = GEngine->GetMediumFont();
    float DrawX = X;
    if (bCentered)
    {
        float Width = 0.0f, Height = 0.0f;
        Canvas->TextSize(Font, Text, Width, Height, TextScale, TextScale);
        DrawX = X - Width * 0.5f;
    }
    FCanvasTextItem Item(FVector2D(DrawX, Y), FText::FromString(Text), Font, Color);
    Item.Scale = FVector2D(TextScale, TextScale);
    Item.EnableShadow(FLinearColor(0.0f, 0.0f, 0.0f, Color.A * 0.9f));
    Canvas->DrawItem(Item);
}

void AIonCockpitHudActor::DrawRect(float X, float Y, float Width, float Height, const FLinearColor& Color)
{
    if (Width <= 0.0f || Height <= 0.0f) return;
    FCanvasTileItem Item(FVector2D(X, Y), FVector2D(Width, Height), Color);
    Item.BlendMode = SE_BLEND_Translucent;
    Canvas->DrawItem(Item);
}

void AIonCockpitHudActor::DrawLineSegment(const FVector2D& From, const FVector2D& To, const FLinearColor& Color, float Thickness)
{
    FCanvasLineItem Item(From, To);
    Item.SetColor(Color);
    Item.LineThickness = Thickness;
    Canvas->DrawItem(Item);
}
