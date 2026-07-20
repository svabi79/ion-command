#pragma once

#include "CoreMinimal.h"
#include "GameFramework/GameModeBase.h"
#include "IonCommandGameMode.generated.h"

UCLASS()
class IONCOMMAND_API AIonCommandGameMode final : public AGameModeBase
{
    GENERATED_BODY()

public:
    AIonCommandGameMode();
    virtual void BeginPlay() override;

private:
    // Optional unattended capture: -IonScreenshotAfter=<seconds>
    // -IonScreenshotFile=<absolute path> [-IonExitAfterScreenshot] takes an
    // in-game screenshot for smoke automation without manual interaction.
    void ScheduleAutomationScreenshot();
    void TakeAutomationScreenshot();
    // -IonAutoSelectAfter=<seconds> selects the first active path so captures
    // exercise selection staging and the path-analysis instrument unattended.
    void ScheduleAutomationSelection();
    void TakeAutomationSelection();
    FString AutomationScreenshotFile;
    bool bExitAfterScreenshot = false;
    FTimerHandle AutomationScreenshotTimer;
    FTimerHandle AutomationEarlyScreenshotTimer;
    FTimerHandle AutomationExitTimer;
    FTimerHandle AutomationSelectTimer;
};

