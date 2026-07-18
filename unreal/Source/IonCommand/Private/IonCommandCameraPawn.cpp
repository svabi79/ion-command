#include "IonCommandCameraPawn.h"

#include "Camera/CameraComponent.h"
#include "Components/InputComponent.h"
#include "Components/SceneComponent.h"
#include "GameFramework/SpringArmComponent.h"
#include "GeoMathLibrary.h"
#include "GeoReplaySubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "GeoTimelineSubsystem.h"

AIonCommandCameraPawn::AIonCommandCameraPawn()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    SpringArm = CreateDefaultSubobject<USpringArmComponent>(TEXT("SpringArm")); SpringArm->SetupAttachment(SceneRoot); SpringArm->TargetArmLength = 3400.0f; SpringArm->bDoCollisionTest = false; SpringArm->SetRelativeRotation(FRotator(-7, 0, 0));
    Camera = CreateDefaultSubobject<UCameraComponent>(TEXT("Camera")); Camera->SetupAttachment(SpringArm, USpringArmComponent::SocketName); Camera->FieldOfView = 68.0f; Camera->bOverrideAspectRatioAxisConstraint = true; Camera->SetAspectRatioAxisConstraint(EAspectRatioAxisConstraint::AspectRatio_MaintainYFOV);
    AutoPossessPlayer = EAutoReceiveInput::Player0;
}

void AIonCommandCameraPawn::BeginPlay()
{
    Super::BeginPlay();
    if (UGeoSelectionSubsystem* Selection = GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>())
    {
        Selection->OnSelectionChanged.AddDynamic(this, &AIonCommandCameraPawn::HandleSelectionChanged);
    }
}

void AIonCommandCameraPawn::EndPlay(const EEndPlayReason::Type EndPlayReason)
{
    if (UGameInstance* GameInstance = GetGameInstance())
    {
        if (UGeoSelectionSubsystem* Selection = GameInstance->GetSubsystem<UGeoSelectionSubsystem>())
        {
            Selection->OnSelectionChanged.RemoveDynamic(this, &AIonCommandCameraPawn::HandleSelectionChanged);
        }
    }
    Super::EndPlay(EndPlayReason);
}

void AIonCommandCameraPawn::SetupPlayerInputComponent(UInputComponent* PlayerInputComponent)
{
    Super::SetupPlayerInputComponent(PlayerInputComponent);
    PlayerInputComponent->BindAction(TEXT("Orbit"), IE_Pressed, this, &AIonCommandCameraPawn::BeginOrbit);
    PlayerInputComponent->BindAction(TEXT("Orbit"), IE_Released, this, &AIonCommandCameraPawn::EndOrbit);
    PlayerInputComponent->BindAction(TEXT("PauseTimeline"), IE_Pressed, this, &AIonCommandCameraPawn::TogglePause);
    PlayerInputComponent->BindAction(TEXT("ReturnToLive"), IE_Pressed, this, &AIonCommandCameraPawn::ReturnToLive);
    PlayerInputComponent->BindAxis(TEXT("OrbitYaw"), this, &AIonCommandCameraPawn::OrbitYaw);
    PlayerInputComponent->BindAxis(TEXT("OrbitPitch"), this, &AIonCommandCameraPawn::OrbitPitch);
    PlayerInputComponent->BindAxis(TEXT("Zoom"), this, &AIonCommandCameraPawn::Zoom);
}

void AIonCommandCameraPawn::Tick(float DeltaSeconds)
{
    Super::Tick(DeltaSeconds);
    if (!bFocusInterpolating || bOrbiting) return;
    const FRotator ActorRotation = GetActorRotation();
    const FRotator ArmRotation = SpringArm->GetRelativeRotation();
    const float YawDelta = FRotator::NormalizeAxis(FocusTargetYaw - ActorRotation.Yaw);
    const float PitchDelta = FocusTargetPitch - ArmRotation.Pitch;
    if (FMath::Abs(YawDelta) < 0.05f && FMath::Abs(PitchDelta) < 0.05f)
    {
        bFocusInterpolating = false;
        return;
    }
    const float Alpha = FMath::Clamp(DeltaSeconds * 2.6f, 0.0f, 1.0f);
    SetActorRotation(FRotator(0.0f, ActorRotation.Yaw + YawDelta * Alpha, 0.0f));
    SpringArm->SetRelativeRotation(FRotator(ArmRotation.Pitch + PitchDelta * Alpha, ArmRotation.Yaw, ArmRotation.Roll));
}

void AIonCommandCameraPawn::FocusOnSelection()
{
    HandleSelectionChanged();
}

void AIonCommandCameraPawn::HandleSelectionChanged()
{
    UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr;
    if (!Selection || !Selection->HasSelection())
    {
        bFocusInterpolating = false;
        return;
    }
    const FGeoMessageEnvelope Selected = Selection->GetSelection();
    if (Selected.Geometry.Positions.Num() == 0) return;
    const FGeoPosition Focus = Selected.Geometry.Positions.Num() >= 2
        ? UGeoMathLibrary::GreatCircleInterpolation(Selected.Geometry.Positions[0], Selected.Geometry.Positions.Last(), 0.5)
        : Selected.Geometry.Positions[0];
    // Layers render at the world origin with identity rotation, so the surface
    // direction doubles as the desired camera direction.
    const FVector SurfaceDirection = UGeoMathLibrary::LatitudeLongitudeToUnitSphere(Focus.Latitude, Focus.Longitude);
    const FRotator TargetRotation = (-SurfaceDirection).Rotation();
    FocusTargetYaw = TargetRotation.Yaw;
    FocusTargetPitch = FMath::Clamp(TargetRotation.Pitch, -75.0f, 65.0f);
    bFocusInterpolating = true;
}

void AIonCommandCameraPawn::BeginOrbit() { bOrbiting = true; bFocusInterpolating = false; }
void AIonCommandCameraPawn::EndOrbit() { bOrbiting = false; }
void AIonCommandCameraPawn::OrbitYaw(float Value) { if (bOrbiting && !FMath::IsNearlyZero(Value)) AddActorWorldRotation(FRotator(0, Value * 0.25f, 0)); }
void AIonCommandCameraPawn::OrbitPitch(float Value) { if (bOrbiting && !FMath::IsNearlyZero(Value)) { FRotator Rotation = SpringArm->GetRelativeRotation(); Rotation.Pitch = FMath::Clamp(Rotation.Pitch + Value * 0.20f, -75.0f, 65.0f); SpringArm->SetRelativeRotation(Rotation); } }
void AIonCommandCameraPawn::Zoom(float Value) { if (!FMath::IsNearlyZero(Value)) SpringArm->TargetArmLength = FMath::Clamp(SpringArm->TargetArmLength - Value * 220.0f, 1450.0f, 6500.0f); }
void AIonCommandCameraPawn::TogglePause() { if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>()) Timeline->SetPaused(!Timeline->IsPaused()); }
void AIonCommandCameraPawn::ReturnToLive()
{
    if (UGeoReplaySubsystem* Replay = GetGameInstance()->GetSubsystem<UGeoReplaySubsystem>())
    {
        Replay->ReturnToLive();
    }
}
