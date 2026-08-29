#include "HamSatellitePassSubsystem.h"

#include "GeoDataSubsystem.h"
#include "HamRadioOwnStationActor.h"
#include "IonCockpitPanelSubsystem.h"

namespace
{
    // How long a position report keeps a satellite on the "now" list. The
    // collector re-reports every few seconds, so anything older than this
    // means the feed stopped rather than the satellite set.
    constexpr double VisibleStaleSeconds = 30.0;
    // Rows per section. The panel is an instrument, not a catalogue: the
    // next few passes are what an operator acts on.
    constexpr int32 MaxRowsPerSection = 3;

    double ReadNumber(const FGeoMessageEnvelope& Message, const TCHAR* Key, double Fallback = 0.0)
    {
        const FString Raw = Message.Properties.FindRef(Key);
        if (Raw.IsEmpty() || Raw == TEXT("null"))
        {
            return Fallback;
        }
        return FCString::Atod(*Raw);
    }

    // Elevation drives the colour: the higher it is, the better the pass and
    // the more the row deserves to be noticed.
    FLinearColor ElevationColor(double ElevationDeg)
    {
        if (ElevationDeg >= 45.0) return FLinearColor(0.35f, 1.0f, 0.55f);
        if (ElevationDeg >= 20.0) return FLinearColor(0.75f, 0.95f, 0.45f);
        return FLinearColor(0.85f, 0.75f, 0.35f);
    }
}

void UHamSatellitePassSubsystem::Initialize(FSubsystemCollectionBase& Collection)
{
    Super::Initialize(Collection);
    Collection.InitializeDependency<UIonCockpitPanelSubsystem>();
    Collection.InitializeDependency<UGeoDataSubsystem>();
    if (UGeoDataSubsystem* Data = GetGameInstance()->GetSubsystem<UGeoDataSubsystem>())
    {
        DataSubsystem = Data;
        MessageAcceptedHandle = Data->OnMessageAccepted().AddUObject(this, &UHamSatellitePassSubsystem::OnMessageAccepted);
    }
    if (UIonCockpitPanelSubsystem* Panels = GetGameInstance()->GetSubsystem<UIonCockpitPanelSubsystem>())
    {
        PanelHandle = Panels->RegisterProvider(FIonCockpitPanelProvider::CreateUObject(this, &UHamSatellitePassSubsystem::BuildPanel));
    }
}

void UHamSatellitePassSubsystem::Deinitialize()
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

void UHamSatellitePassSubsystem::OnMessageAccepted(const FGeoMessageEnvelope& Message)
{
    const FString NoradId = Message.Properties.FindRef(TEXT("noradId"));
    if (NoradId.IsEmpty())
    {
        return;
    }
    if (Message.SemanticType == TEXT("orbital.pass"))
    {
        FPredictedPass Pass;
        Pass.Name = Message.Properties.FindRef(TEXT("display.title"));
        Pass.PeakElevationDeg = ReadNumber(Message, TEXT("peakElevationDeg"));
        FDateTime::ParseIso8601(*Message.Properties.FindRef(TEXT("aosUtc")), Pass.Acquisition);
        FDateTime::ParseIso8601(*Message.Properties.FindRef(TEXT("losUtc")), Pass.Loss);
        // The collector renders the compass pair itself and publishes it as
        // its own property; deriving it again here, or picking it back out
        // of the display string, would be a second place to get it wrong.
        Pass.Bearing = Message.Properties.FindRef(TEXT("bearing"));
        Passes.Add(NoradId, MoveTemp(Pass));
        return;
    }
    if (Message.SemanticType != TEXT("orbital.position"))
    {
        return;
    }
    // "aboveHorizon" is only present when the collector has a station
    // configured; without one there is nothing to be above the horizon of.
    const FString Above = Message.Properties.FindRef(TEXT("aboveHorizon"));
    if (Above.IsEmpty())
    {
        return;
    }
    if (Above == TEXT("true"))
    {
        FVisibleSatellite Satellite;
        Satellite.Name = Message.Properties.FindRef(TEXT("display.title"));
        Satellite.ElevationDeg = ReadNumber(Message, TEXT("elevationDeg"));
        Satellite.AzimuthDeg = ReadNumber(Message, TEXT("azimuthDeg"));
        Satellite.RangeKm = ReadNumber(Message, TEXT("rangeKm"));
        Satellite.Seen = FDateTime::UtcNow();
        Visible.Add(NoradId, MoveTemp(Satellite));
    }
    else
    {
        // It set. Drop it rather than letting it age out, so the panel is
        // right the moment the satellite goes below the horizon.
        Visible.Remove(NoradId);
    }
}

FIonCockpitPanelModel UHamSatellitePassSubsystem::BuildPanel() const
{
    FIonCockpitPanelModel Panel;
    const FString Callsign = AHamRadioOwnStationActor::ConfiguredCallsign();
    const bool bConfigured = AHamRadioOwnStationActor::IsStationConfigured();
    Panel.Title = bConfigured
        ? FString::Printf(TEXT("SATELLITES // %s"), *AHamRadioOwnStationActor::ConfiguredLocator().ToUpper())
        : TEXT("SATELLITES");

    if (!bConfigured)
    {
        FIonCockpitPanelRow Row;
        Row.Label = TEXT("STATION");
        Row.Cells.Add({TEXT("NOT SET"), FLinearColor(0.85f, 0.75f, 0.35f)});
        Panel.Rows.Add(MoveTemp(Row));
        return Panel;
    }

    const FDateTime Now = FDateTime::UtcNow();

    TArray<FVisibleSatellite> Up;
    for (const TPair<FString, FVisibleSatellite>& Pair : Visible)
    {
        if ((Now - Pair.Value.Seen).GetTotalSeconds() <= VisibleStaleSeconds)
        {
            Up.Add(Pair.Value);
        }
    }
    Up.Sort([](const FVisibleSatellite& A, const FVisibleSatellite& B) { return A.ElevationDeg > B.ElevationDeg; });

    if (Up.Num() == 0)
    {
        FIonCockpitPanelRow Row;
        Row.Label = TEXT("NOW");
        Row.Cells.Add({TEXT("NONE ABOVE HORIZON"), FLinearColor(0.55f, 0.6f, 0.65f)});
        Panel.Rows.Add(MoveTemp(Row));
    }
    for (int32 Index = 0; Index < FMath::Min(Up.Num(), MaxRowsPerSection); ++Index)
    {
        const FVisibleSatellite& Satellite = Up[Index];
        FIonCockpitPanelRow Row;
        Row.Label = Index == 0 ? TEXT("NOW") : FString();
        // One cell, not two: the panel reserves everything left of 92 units
        // for the row label, and a satellite name in its own cell pushes the
        // numbers off the right edge. Name and figures share a cell instead.
        Row.Cells.Add({FString::Printf(TEXT("%-11s EL%.0f AZ%.0f"), *Satellite.Name.Left(11),
                                       Satellite.ElevationDeg, Satellite.AzimuthDeg),
                       ElevationColor(Satellite.ElevationDeg)});
        Panel.Rows.Add(MoveTemp(Row));
    }
    if (Up.Num() > MaxRowsPerSection)
    {
        FIonCockpitPanelRow Row;
        Row.Cells.Add({FString::Printf(TEXT("+%d more up"), Up.Num() - MaxRowsPerSection), FLinearColor(0.55f, 0.6f, 0.65f)});
        Panel.Rows.Add(MoveTemp(Row));
    }

    // Passes that have already finished are not predictions any more.
    TArray<FPredictedPass> Upcoming;
    for (const TPair<FString, FPredictedPass>& Pair : Passes)
    {
        if (Pair.Value.Loss > Now)
        {
            Upcoming.Add(Pair.Value);
        }
    }
    Upcoming.Sort([](const FPredictedPass& A, const FPredictedPass& B) { return A.Acquisition < B.Acquisition; });

    for (int32 Index = 0; Index < FMath::Min(Upcoming.Num(), MaxRowsPerSection); ++Index)
    {
        const FPredictedPass& Pass = Upcoming[Index];
        FIonCockpitPanelRow Row;
        Row.Label = Index == 0 ? TEXT("NEXT") : FString();

        // Minutes to acquisition is what the operator is really reading;
        // an absolute time makes them do the subtraction.
        const double MinutesAway = (Pass.Acquisition - Now).GetTotalMinutes();
        const FString When = MinutesAway <= 0.0
            ? TEXT("NOW")
            : FString::Printf(TEXT("%.0fm"), MinutesAway);
        // Deliberately terse, for the same reason: name, when, how high and
        // which way across the sky, in one cell. The bearing earns its space
        // - it is where to point the antenna first.
        Row.Cells.Add({FString::Printf(TEXT("%-8s %s %.0fd %s"), *Pass.Name.Left(8), *When,
                                       Pass.PeakElevationDeg, *Pass.Bearing.Replace(TEXT(" -> "), TEXT(">"))),
                       ElevationColor(Pass.PeakElevationDeg)});
        Panel.Rows.Add(MoveTemp(Row));
    }
    if (Upcoming.Num() == 0)
    {
        FIonCockpitPanelRow Row;
        Row.Label = TEXT("NEXT");
        Row.Cells.Add({TEXT("AWAITING PREDICTION"), FLinearColor(0.55f, 0.6f, 0.65f)});
        Panel.Rows.Add(MoveTemp(Row));
    }
    return Panel;
}
