#include "HamConditionsSubsystem.h"

#include "GeoDataSubsystem.h"
#include "IonCockpitPanelSubsystem.h"

namespace
{
enum class EBandRating : uint8 { Poor, Fair, Good };

FLinearColor RatingColor(EBandRating Rating)
{
    switch (Rating)
    {
    case EBandRating::Good: return FLinearColor(0.24f, 1.0f, 0.60f);
    case EBandRating::Fair: return FLinearColor(1.0f, 0.63f, 0.12f);
    default: return FLinearColor(1.0f, 0.25f, 0.20f);
    }
}

const TCHAR* RatingText(EBandRating Rating)
{
    switch (Rating)
    {
    case EBandRating::Good: return TEXT("GOOD");
    case EBandRating::Fair: return TEXT("FAIR");
    default: return TEXT("POOR");
    }
}

EBandRating Degrade(EBandRating Rating, int32 Steps)
{
    int32 Value = static_cast<int32>(Rating) - Steps;
    return static_cast<EBandRating>(FMath::Clamp(Value, 0, 2));
}

// Heuristic day/night ratings per band group from solar flux, in the
// tradition of the amateur-radio conditions tables: higher flux opens the
// higher bands; geomagnetic activity (Kp/A) degrades everything. This is an
// indicator, not a propagation prediction.
void GroupRatings(int32 Group, double Flux, EBandRating& OutDay, EBandRating& OutNight)
{
    switch (Group)
    {
    case 0: // 80m-40m: absorption-limited by day, reliable by night.
        OutDay = EBandRating::Fair;
        OutNight = EBandRating::Good;
        return;
    case 1: // 30m-20m: the workhorses once flux is moderate.
        OutDay = Flux >= 90 ? EBandRating::Good : EBandRating::Fair;
        OutNight = Flux >= 100 ? EBandRating::Good : EBandRating::Fair;
        return;
    case 2: // 17m-15m
        OutDay = Flux >= 120 ? EBandRating::Good : (Flux >= 100 ? EBandRating::Fair : EBandRating::Poor);
        OutNight = Flux >= 140 ? EBandRating::Fair : EBandRating::Poor;
        return;
    default: // 12m-10m: need a strong sun.
        OutDay = Flux >= 160 ? EBandRating::Good : (Flux >= 120 ? EBandRating::Fair : EBandRating::Poor);
        OutNight = EBandRating::Poor;
        return;
    }
}
}

void UHamConditionsSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    Collection.InitializeDependency<UIonCockpitPanelSubsystem>();
    Collection.InitializeDependency<UGeoDataSubsystem>();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>())
    {
        DataSubsystem = Data;
        MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &UHamConditionsSubsystem::OnMessageAccepted);
    }
    if (UIonCockpitPanelSubsystem* Panels = GetGameInstance()->GetSubsystem<UIonCockpitPanelSubsystem>())
    {
        PanelHandle = Panels->RegisterProvider(FIonCockpitPanelProvider::CreateUObject(this, &UHamConditionsSubsystem::BuildPanel));
    }
}

void UHamConditionsSubsystem::Deinitialize()
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

void UHamConditionsSubsystem::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    if (Message.SemanticType != TEXT("spaceweather.state")) return;
    auto Read = [&Message](const TCHAR* Key, double& Out)
    {
        const FString Raw = Message.Properties.FindRef(Key);
        if (!Raw.IsEmpty() && Raw != TEXT("null")) Out = FCString::Atod(*Raw);
    };
    Read(TEXT("solarFlux"), SolarFlux);
    Read(TEXT("kp"), Kp);
    Read(TEXT("aIndex"), AIndex);
}

FIonCockpitPanelModel UHamConditionsSubsystem::BuildPanel() const
{
    FIonCockpitPanelModel Panel;
    Panel.Title = TEXT("HF CONDITIONS // HEURISTIC");
    if (SolarFlux < 0.0)
    {
        FIonCockpitPanelRow Row;
        Row.Label = TEXT("AWAITING DATA");
        Row.Cells.Add({TEXT("--"), FLinearColor(0.10f, 0.38f, 0.52f)});
        Panel.Rows.Add(Row);
        return Panel;
    }
    // Geomagnetic degradation: unsettled fields close paths before flux does.
    int32 Penalty = 0;
    if (Kp >= 4.0 || AIndex >= 20.0) Penalty = 1;
    if (Kp >= 5.0 || AIndex >= 30.0) Penalty = 2;
    static const TCHAR* GroupLabels[] = {TEXT("80M-40M"), TEXT("30M-20M"), TEXT("17M-15M"), TEXT("12M-10M")};
    for (int32 Group = 0; Group < 4; ++Group)
    {
        EBandRating Day = EBandRating::Poor;
        EBandRating Night = EBandRating::Poor;
        GroupRatings(Group, SolarFlux, Day, Night);
        Day = Degrade(Day, Penalty);
        Night = Degrade(Night, Penalty);
        FIonCockpitPanelRow Row;
        Row.Label = GroupLabels[Group];
        Row.Cells.Add({FString::Printf(TEXT("DAY %s"), RatingText(Day)), RatingColor(Day)});
        Row.Cells.Add({FString::Printf(TEXT("NGT %s"), RatingText(Night)), RatingColor(Night)});
        Panel.Rows.Add(Row);
    }
    return Panel;
}
