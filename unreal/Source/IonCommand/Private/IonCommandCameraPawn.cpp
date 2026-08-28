#include "IonCommandCameraPawn.h"

#include "Camera/CameraComponent.h"
#include "Components/InputComponent.h"
#include "Components/SceneComponent.h"
#include "Engine/Engine.h"
#include "GameFramework/SpringArmComponent.h"
#include "GeoMathLibrary.h"
#include "GeoReplaySubsystem.h"
#include "Misc/CommandLine.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/Parse.h"
#include "GeoSelectionSubsystem.h"
#include "GeoTimelineSubsystem.h"

namespace
{
    // Globe radius is 1000 units = Earth's mean radius (6371 km), so 1 unit
    // = 6.371 km. The camera orbit distance is (GlobeRadius + altitude); the
    // constants below are shared between the debug distance override, the
    // wheel zoom clamp, and the near clip plane so all three stay consistent
    // if the range ever changes again.
    constexpr float MinOrbitDistance = 1005.0f; // ~31.9 km above the surface.
    constexpr float MaxOrbitDistance = 6500.0f;
    // Engine floor is 1.0 (see UnrealEngine.cpp's SetNearClipPlane); comfortably
    // below MinOrbitDistance's 5-unit altitude (~6.4 km vs ~31.9 km, a 5x
    // margin) so the nearest visible surface point - always exactly the
    // camera's altitude above the globe, since the globe is convex and the
    // camera sits outside it - is never behind the near plane.
    constexpr float NearClipPlaneUnits = 1.0f;
}

AIonCommandCameraPawn::AIonCommandCameraPawn()
{
    PrimaryActorTick.bCanEverTick = true;
    SceneRoot = CreateDefaultSubobject<USceneComponent>(TEXT("Root")); SetRootComponent(SceneRoot);
    SpringArm = CreateDefaultSubobject<USpringArmComponent>(TEXT("SpringArm")); SpringArm->SetupAttachment(SceneRoot); SpringArm->TargetArmLength = 3400.0f; SpringArm->bDoCollisionTest = false; SpringArm->SetRelativeRotation(FRotator(-7, 0, 0));
    Camera = CreateDefaultSubobject<UCameraComponent>(TEXT("Camera")); Camera->SetupAttachment(SpringArm, USpringArmComponent::SocketName); Camera->FieldOfView = 68.0f; Camera->bOverrideAspectRatioAxisConstraint = true; Camera->SetAspectRatioAxisConstraint(EAspectRatioAxisConstraint::AspectRatio_MaintainYFOV);
    // Cinematic grade: fixed exposure so emissive holograms never pump, soft
    // wide bloom for the light beams, and a gentle vignette for depth.
    FPostProcessSettings& Grade = Camera->PostProcessSettings;
    // Pin the histogram exposure instead of AEM_Manual: manual mode meters
    // through the physical camera model and drowns this stylised scene.
    Grade.bOverride_AutoExposureMinBrightness = true; Grade.AutoExposureMinBrightness = 1.0f;
    Grade.bOverride_AutoExposureMaxBrightness = true; Grade.AutoExposureMaxBrightness = 1.0f;
    Grade.bOverride_AutoExposureBias = true; Grade.AutoExposureBias = 0.3f;
    Grade.bOverride_BloomIntensity = true; Grade.BloomIntensity = 1.1f;
    Grade.bOverride_BloomThreshold = true; Grade.BloomThreshold = 0.35f;
    Grade.bOverride_BloomSizeScale = true; Grade.BloomSizeScale = 5.5f;
    Grade.bOverride_VignetteIntensity = true; Grade.VignetteIntensity = 0.38f;
    Grade.bOverride_SceneFringeIntensity = true; Grade.SceneFringeIntensity = 0.3f;
    // No lens flare: overexposed arc-convergence hotspots (one skimmer hearing
    // hundreds of stations) otherwise smear polygonal ghost chains across the
    // starfield.
    Grade.bOverride_LensFlareIntensity = true; Grade.LensFlareIntensity = 0.0f;
    AutoPossessPlayer = EAutoReceiveInput::Player0;
}

void AIonCommandCameraPawn::BeginPlay()
{
    Super::BeginPlay();
    // The default near clip plane (10 units) is farther than the closest the
    // wheel zoom now goes (5 units of altitude at MinOrbitDistance), which
    // would clip the nearest ground out of view entirely. r.SetNearClipPlane
    // is a console command, not a CVar, so it cannot be set from a config
    // file - it has to be executed once here. This is a global renderer
    // setting (all viewports), which is fine: nothing else in ION COMMAND
    // relies on the default.
    if (GEngine)
    {
        GEngine->Exec(GetWorld(), *FString::Printf(TEXT("r.SetNearClipPlane %f"), NearClipPlaneUnits));
    }
    // -IonCameraDistance=<units> pins the orbit distance for unattended
    // captures (e.g. zoom-quality proofs).
    double CameraDistance = 0.0;
    if (FParse::Value(FCommandLine::Get(), TEXT("IonCameraDistance="), CameraDistance) && CameraDistance > 0.0)
    {
        SpringArm->TargetArmLength = FMath::Clamp(static_cast<float>(CameraDistance), MinOrbitDistance, MaxOrbitDistance);
    }
    // -IonCameraLongitude=<deg> centers the given longitude for captures.
    // The camera looks along the pawn's negative forward axis (the spring arm
    // extends forward past the globe), hence yaw = -90 - longitude, verified
    // against captures.
    double CameraLongitude = 0.0;
    if (FParse::Value(FCommandLine::Get(), TEXT("IonCameraLongitude="), CameraLongitude))
    {
        SetActorRotation(FRotator(0.0, -90.0 - CameraLongitude, 0.0));
    }
    // -IonCameraLatitude=<deg> raises the orbit so the given latitude sits at
    // screen center (spring arm pitches down by the latitude).
    double CameraLatitude = 0.0;
    if (FParse::Value(FCommandLine::Get(), TEXT("IonCameraLatitude="), CameraLatitude))
    {
        SpringArm->SetRelativeRotation(FRotator(FMath::Clamp(-CameraLatitude, -85.0, 85.0), 0.0, 0.0));
    }
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
void AIonCommandCameraPawn::OrbitPitch(float Value)
{
    if (!bOrbiting || FMath::IsNearlyZero(Value)) return;
    // Vertical matches the horizontal drag convention by default; INVERT
    // ORBIT Y in the settings panel restores the old direction. Re-read per
    // event, like the own-station ini reads, so the toggle applies live.
    bool bInvert = false;
    GConfig->GetBool(TEXT("IonCommand.Input"), TEXT("InvertOrbitY"), bInvert, GGameIni);
    FRotator Rotation = SpringArm->GetRelativeRotation();
    Rotation.Pitch = FMath::Clamp(Rotation.Pitch + Value * (bInvert ? -0.20f : 0.20f), -75.0f, 65.0f);
    SpringArm->SetRelativeRotation(Rotation);
}
void AIonCommandCameraPawn::Zoom(float Value)
{
    if (FMath::IsNearlyZero(Value)) return;
    // Distance-proportional steps: coarse with the whole globe framed, fine
    // near the surface, so one wheel notch never overshoots the last few
    // kilometres. MinOrbitDistance (1005) puts the camera ~31.9 km up - close
    // enough to make out the high-resolution globe texture and mesh in
    // genuine close-up. At the default orbit the step matches the old fixed
    // 220 units. The 0.4 floor sits just below the formula's own value at
    // MinOrbitDistance (0.09 * 5 = 0.45), so it is a defensive minimum rather
    // than something that actually kicks in across the live range.
    const float Step = FMath::Clamp((SpringArm->TargetArmLength - 1000.0f) * 0.09f, 0.4f, 500.0f);
    SpringArm->TargetArmLength = FMath::Clamp(SpringArm->TargetArmLength - Value * Step, MinOrbitDistance, MaxOrbitDistance);
}
void AIonCommandCameraPawn::TogglePause() { if (UGeoTimelineSubsystem* Timeline = GetGameInstance()->GetSubsystem<UGeoTimelineSubsystem>()) Timeline->SetPaused(!Timeline->IsPaused()); }
void AIonCommandCameraPawn::ReturnToLive()
{
    if (UGeoReplaySubsystem* Replay = GetGameInstance()->GetSubsystem<UGeoReplaySubsystem>())
    {
        Replay->ReturnToLive();
    }
}
