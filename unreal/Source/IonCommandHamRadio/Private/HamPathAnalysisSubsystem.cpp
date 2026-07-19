#include "HamPathAnalysisSubsystem.h"

#include "GeoDataSubsystem.h"
#include "GeoMathLibrary.h"
#include "GeoSelectionSubsystem.h"
#include "IonCockpitPanelSubsystem.h"

namespace
{
const FLinearColor PathGood(0.24f, 1.0f, 0.60f);
const FLinearColor PathAmber(1.0f, 0.63f, 0.12f);
const FLinearColor PathRed(1.0f, 0.25f, 0.20f);
const FLinearColor PathDim(0.10f, 0.38f, 0.52f);
const FLinearColor PathWhite(0.88f, 0.99f, 1.0f);

// A single F2 hop covers at most roughly 3,500 km; the sounding is only
// representative near the reflection point.
constexpr double MaxHopKm = 3500.0;
constexpr double MaxSounderDistanceKm = 3000.0;
constexpr double MaxSoundingAgeSeconds = 2.0 * 3600.0;

double PropertyAsDouble(const TMap<FString, FString>& Properties, const TCHAR* Key)
{
    const FString Raw = Properties.FindRef(Key);
    if (Raw.IsEmpty() || Raw == TEXT("null")) return 0.0;
    return FCString::Atod(*Raw);
}

// MUF for a hop shorter than the reference 3,000 km: interpolate the M
// factor between 1 (vertical, MUF = foF2) and M(3000) with a smooth arc.
// This is the usual quick-look approximation, not a propagation prediction.
double HopMuf(const FHamSounding& Sounding, double HopKm)
{
    const double M3000 = Sounding.M3000 > 1.0 ? Sounding.M3000 : (Sounding.FoF2Mhz > 0.1 ? Sounding.MufdMhz / Sounding.FoF2Mhz : 3.0);
    const double Clamped = FMath::Clamp(HopKm, 0.0, 3000.0);
    const double Factor = 1.0 + (M3000 - 1.0) * FMath::Sin(PI * Clamped / 6000.0);
    return Sounding.FoF2Mhz * Factor;
}
}

void UHamPathAnalysisSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    Collection.InitializeDependency<UIonCockpitPanelSubsystem>();
    Collection.InitializeDependency<UGeoDataSubsystem>();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>())
    {
        DataSubsystem = Data;
        MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &UHamPathAnalysisSubsystem::OnMessageAccepted);
    }
    if (UIonCockpitPanelSubsystem* Panels = GetGameInstance()->GetSubsystem<UIonCockpitPanelSubsystem>())
    {
        PanelHandle = Panels->RegisterProvider(FIonCockpitPanelProvider::CreateUObject(this, &UHamPathAnalysisSubsystem::BuildPanel));
    }
}

void UHamPathAnalysisSubsystem::Deinitialize()
{
    if (UGeoDataSubsystem* Data = DataSubsystem.Get())
    {
        Data->OnMessageAccepted().Remove(MessageAcceptedHandle);
    }
    if (UIonCockpitPanelSubsystem* Panels = GetGameInstance() ? GetGameInstance()->GetSubsystem<UIonCockpitPanelSubsystem>() : nullptr)
    {
        Panels->UnregisterProvider(PanelHandle);
    }
    Super::Deinitialize();
}

void UHamPathAnalysisSubsystem::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    if (Message.SemanticType != TEXT("ionosphere.sounding") || Message.Geometry.Positions.Num() < 1) return;
    const FString StationId = Message.Properties.FindRef(TEXT("stationId"));
    const double FoF2 = PropertyAsDouble(Message.Properties, TEXT("foF2Mhz"));
    const double Mufd = PropertyAsDouble(Message.Properties, TEXT("mufdMhz"));
    if (StationId.IsEmpty() || FoF2 <= 0.0 || Mufd <= 0.0) return;
    if (!Soundings.Contains(StationId) && Soundings.Num() >= MaxSoundings) return;
    FHamSounding& Sounding = Soundings.FindOrAdd(StationId);
    Sounding.Position = Message.Geometry.Positions[0];
    Sounding.FoF2Mhz = FoF2;
    Sounding.MufdMhz = Mufd;
    Sounding.M3000 = PropertyAsDouble(Message.Properties, TEXT("m3000"));
    Sounding.ObservedUtc = Message.Time.ObservedUtc;
}

FIonCockpitPanelModel UHamPathAnalysisSubsystem::BuildPanel() const
{
    FIonCockpitPanelModel Panel;
    const UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr;
    if (!Selection || !Selection->HasSelection()) return Panel;
    const FGeoMessageEnvelope Message = Selection->GetSelection();
    if (Message.Geometry.Positions.Num() < 2) return Panel;

    Panel.Title = TEXT("PATH ANALYSIS // HEURISTIC");
    const FGeoPosition& Start = Message.Geometry.Positions[0];
    const FGeoPosition& End = Message.Geometry.Positions.Last();
    const double DistanceKm = UGeoMathLibrary::GreatCircleDistanceKm(Start, End);
    const int32 Hops = FMath::Max(1, FMath::CeilToInt32(DistanceKm / MaxHopKm));
    const double HopKm = DistanceKm / Hops;

    FIonCockpitPanelRow HopsRow;
    HopsRow.Label = TEXT("HOPS");
    HopsRow.Cells.Add({FString::Printf(TEXT("%d x %.0f KM"), Hops, HopKm), PathWhite});
    Panel.Rows.Add(HopsRow);

    // The controlling hop is the one with the lowest estimated MUF at its
    // reflection midpoint, judged by the nearest fresh ionosonde.
    const FDateTime NowUtc = FDateTime::UtcNow();
    double PathMuf = TNumericLimits<double>::Max();
    FString ControllingStation;
    int32 JudgedHops = 0;
    for (int32 Hop = 0; Hop < Hops; ++Hop)
    {
        const double Alpha = (Hop + 0.5) / Hops;
        const FGeoPosition Midpoint = UGeoMathLibrary::GreatCircleInterpolation(Start, End, Alpha);
        const FHamSounding* Nearest = nullptr;
        FString NearestId;
        double NearestKm = MaxSounderDistanceKm;
        for (const TPair<FString, FHamSounding>& Pair : Soundings)
        {
            if ((NowUtc - Pair.Value.ObservedUtc).GetTotalSeconds() > MaxSoundingAgeSeconds) continue;
            const double SounderKm = UGeoMathLibrary::GreatCircleDistanceKm(Midpoint, Pair.Value.Position);
            if (SounderKm < NearestKm)
            {
                NearestKm = SounderKm;
                Nearest = &Pair.Value;
                NearestId = Pair.Key;
            }
        }
        if (!Nearest) continue;
        ++JudgedHops;
        const double Muf = HopMuf(*Nearest, HopKm);
        if (Muf < PathMuf)
        {
            PathMuf = Muf;
            ControllingStation = NearestId;
        }
    }

    if (JudgedHops == 0)
    {
        FIonCockpitPanelRow Row;
        Row.Label = TEXT("MUF");
        Row.Cells.Add({TEXT("NO SOUNDER DATA"), PathDim});
        Panel.Rows.Add(Row);
        return Panel;
    }

    FIonCockpitPanelRow MufRow;
    MufRow.Label = TEXT("MUF EST");
    MufRow.Cells.Add({FString::Printf(TEXT("%.1f MHZ"), PathMuf), PathWhite});
    MufRow.Cells.Add({FString::Printf(TEXT("VIA %s"), *ControllingStation), PathDim});
    Panel.Rows.Add(MufRow);
    if (JudgedHops < Hops)
    {
        FIonCockpitPanelRow Coverage;
        Coverage.Label = TEXT("COVER");
        Coverage.Cells.Add({FString::Printf(TEXT("%d/%d HOPS JUDGED"), JudgedHops, Hops), PathAmber});
        Panel.Rows.Add(Coverage);
    }

    const double FrequencyHz = PropertyAsDouble(Message.Properties, TEXT("frequencyHz"));
    if (FrequencyHz > 0.0)
    {
        const double FrequencyMhz = FrequencyHz / 1'000'000.0;
        FIonCockpitPanelRow LinkRow;
        LinkRow.Label = TEXT("LINK");
        LinkRow.Cells.Add({FString::Printf(TEXT("%.3f MHZ"), FrequencyMhz), PathWhite});
        Panel.Rows.Add(LinkRow);

        FIonCockpitPanelRow Verdict;
        Verdict.Label = TEXT("VERDICT");
        if (FrequencyMhz <= PathMuf * 0.85)
        {
            Verdict.Cells.Add({TEXT("SUPPORTED"), PathGood});
        }
        else if (FrequencyMhz <= PathMuf)
        {
            Verdict.Cells.Add({TEXT("MARGINAL"), PathAmber});
        }
        else
        {
            // Above the F2 MUF estimate: the observed path likely used
            // sporadic E or another mode the sounding cannot see.
            Verdict.Cells.Add({TEXT("ABOVE F2 MUF"), PathRed});
        }
        Panel.Rows.Add(Verdict);
    }
    return Panel;
}
