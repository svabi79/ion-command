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
    // Routes typed characters to the settings panel while a text field is
    // focused, and consumes them so they do not also trigger band/layer keys.
    virtual bool InputKey(const FInputKeyEventArgs& Params) override;

private:
    // True while the cockpit settings panel is capturing text; hotkey actions
    // no-op so typing a callsign cannot flip layers or bands.
    bool IsTypingText() const;
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
    void ToggleOverlayMenu();
    FString ActiveModeFilter;
    // "/" or "S" opens search; "W" toggles the watch/alert panel. Both just
    // forward to the HUD, matching every other panel toggle in this class.
    void OpenSearchOverlay();
    void ToggleWatchlist();
    void StartRecentReplay();
    void ChangeReplaySpeed(double Factor);
    void ReplaySlower();
    void ReplayFaster();
    double ReplaySpeed = 1.0;
    FDateTime ReplayFromUtc;
    FDateTime ReplayToUtc;
};
