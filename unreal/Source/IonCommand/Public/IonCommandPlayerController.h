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
};
