#pragma once

#include "CoreMinimal.h"
#include "GameFramework/PlayerController.h"
#include "IonCommandPlayerController.generated.h"

UCLASS()
class IONCOMMAND_API AIonCommandPlayerController final : public APlayerController
{
    GENERATED_BODY()

public:
    AIonCommandPlayerController();

protected:
    virtual void SetupInputComponent() override;

private:
    void SelectUnderCursor();
    void ClearSelection();
    void ToggleIonosphere();
    void FocusSelection();
    void ToggleOwnStationFilter();
    void SelectBandPreset(int32 PaletteIndex);
    void ClearBandPreset();
    void CycleHudMode();
    void ToggleHeatmap();
    void TogglePaths();
    void CycleModeFilter();
    FString ActiveModeFilter;
    void StartRecentReplay();
    void ChangeReplaySpeed(double Factor);
    void ReplaySlower();
    void ReplayFaster();
    double ReplaySpeed = 1.0;
    FDateTime ReplayFromUtc;
    FDateTime ReplayToUtc;
};
