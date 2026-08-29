#pragma once

#include "CoreMinimal.h"
#include "GeoTypes.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "HamSatellitePassSubsystem.generated.h"

struct FIonCockpitPanelModel;

// Publishes the two questions a station operator actually has about
// satellites: what is above me now, and when is the next pass. Both arrive
// from the collector, which computes them from the station configured there;
// this only sorts and presents them.
UCLASS()
class IONCOMMANDHAMRADIO_API UHamSatellitePassSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    FIonCockpitPanelModel BuildPanel() const;

    // One satellite currently above the horizon.
    struct FVisibleSatellite
    {
        FString Name;
        double ElevationDeg = 0.0;
        double AzimuthDeg = 0.0;
        double RangeKm = 0.0;
        FDateTime Seen;
    };

    // One predicted pass. Keyed by satellite, so a re-prediction replaces
    // rather than accumulates.
    struct FPredictedPass
    {
        FString Name;
        FDateTime Acquisition;
        FDateTime Loss;
        double PeakElevationDeg = 0.0;
        FString Bearing;
    };

    // Keyed by NORAD id in both cases.
    TMap<FString, FVisibleSatellite> Visible;
    TMap<FString, FPredictedPass> Passes;

    int32 PanelHandle = 0;
    FDelegateHandle MessageAcceptedHandle;
    TWeakObjectPtr<class UGeoDataSubsystem> DataSubsystem;
};
