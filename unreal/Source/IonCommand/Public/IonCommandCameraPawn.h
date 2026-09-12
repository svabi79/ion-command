#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Pawn.h"
#include "IonCommandCameraPawn.generated.h"

class UCameraComponent;
class USpringArmComponent;

UCLASS()
class IONCOMMAND_API AIonCommandCameraPawn final : public APawn
{
    GENERATED_BODY()

public:
    AIonCommandCameraPawn();
    virtual void SetupPlayerInputComponent(UInputComponent* PlayerInputComponent) override;
    virtual void Tick(float DeltaSeconds) override;

    // Restarts the ease toward the current selection, e.g. for the F key.
    void FocusOnSelection();

    // Sensor looks (issue #8): F1–F6 and the SETTINGS row. Missing
    // post-process materials are a no-op so a client built before the
    // editor script ran still starts.
    void SetSensorLook(int32 Index);
    int32 GetSensorLook() const { return SensorLookIndex; }
    static FString SensorLookLabel(int32 Index);

protected:
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;

private:
    void BeginOrbit();
    void EndOrbit();
    void OrbitYaw(float Value);
    void OrbitPitch(float Value);
    void Zoom(float Value);
    void TogglePause();
    void ReturnToLive();

    // Reacts to operator selection: eases the orbit toward the selected
    // geometry midpoint without ever taking control away from manual orbiting.
    UFUNCTION() void HandleSelectionChanged();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<USpringArmComponent> SpringArm;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UCameraComponent> Camera;
    void ApplySensorLookBlendables();
    int32 SensorLookIndex = 0;
    FString AppliedSensorLook;
    bool bOrbiting = false;
    bool bFocusInterpolating = false;
    float FocusTargetYaw = 0.0f;
    float FocusTargetPitch = 0.0f;
};
