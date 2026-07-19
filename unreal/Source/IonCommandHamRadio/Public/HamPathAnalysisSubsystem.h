#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "HamPathAnalysisSubsystem.generated.h"

struct FIonCockpitPanelModel;

// One cached ionosonde sounding for hop-MUF estimation.
struct FHamSounding
{
    FGeoPosition Position;
    double FoF2Mhz = 0.0;
    double MufdMhz = 0.0;
    double M3000 = 0.0;
    FDateTime ObservedUtc;
};

// Publishes the PATH ANALYSIS cockpit panel: hop geometry for the selected
// link plus an MUF estimate from the nearest ionosonde soundings, labeled as
// a heuristic. Ham vocabulary stays in this module.
UCLASS()
class IONCOMMANDHAMRADIO_API UHamPathAnalysisSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    FIonCockpitPanelModel BuildPanel() const;

    TMap<FString, FHamSounding> Soundings;
    static constexpr int32 MaxSoundings = 256;

    int32 PanelHandle = 0;
    FDelegateHandle MessageAcceptedHandle;
    TWeakObjectPtr<class UGeoDataSubsystem> DataSubsystem;
};
