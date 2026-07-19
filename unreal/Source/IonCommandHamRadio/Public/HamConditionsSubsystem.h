#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "GeoTypes.h"
#include "HamConditionsSubsystem.generated.h"

struct FIonCockpitPanelModel;

// Publishes the classic HF band-conditions forecast panel (solar flux plus
// geomagnetic activity heuristic, in the tradition of the ham "solar widget"
// tables) to the cockpit. Ham vocabulary stays in this module; the UI only
// sees generic panel rows.
UCLASS()
class IONCOMMANDHAMRADIO_API UHamConditionsSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    virtual void Initialize(FSubsystemCollectionBase& Collection) override;
    virtual void Deinitialize() override;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    FIonCockpitPanelModel BuildPanel() const;

    // Latest space-weather sample (negative sentinel = not yet seen).
    double SolarFlux = -1.0;
    double Kp = -1.0;
    double AIndex = -1.0;

    int32 PanelHandle = 0;
    FDelegateHandle MessageAcceptedHandle;
    TWeakObjectPtr<class UGeoDataSubsystem> DataSubsystem;
};
