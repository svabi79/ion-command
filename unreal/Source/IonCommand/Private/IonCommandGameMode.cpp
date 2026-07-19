#include "IonCommandGameMode.h"

#include "HAL/PlatformMisc.h"
#include "Misc/CommandLine.h"
#include "Misc/Parse.h"
#include "TimerManager.h"
#include "UnrealClient.h"
#include "Engine/DirectionalLight.h"
#include "Engine/ExponentialHeightFog.h"
#include "Engine/SkyLight.h"
#include "EngineUtils.h"
#include "GeoPointLayerActor.h"
#include "HamRadioLinkLayerActor.h"
#include "HamRadioOwnStationActor.h"
#include "IonCommandCameraPawn.h"
#include "IonCommandDeckActor.h"
#include "IonCommandPlayerController.h"
#include "IonAuroraActor.h"
#include "IonGlobeActor.h"
#include "IonIonosphereActor.h"

AIonCommandGameMode::AIonCommandGameMode()
{
    DefaultPawnClass = AIonCommandCameraPawn::StaticClass();
    PlayerControllerClass = AIonCommandPlayerController::StaticClass();
}

void AIonCommandGameMode::BeginPlay()
{
    Super::BeginPlay();
    UWorld* World = GetWorld();
    if (!World) return;
    TActorIterator<AIonGlobeActor> Globe(World); if (!Globe) World->SpawnActor<AIonGlobeActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    TActorIterator<AHamRadioLinkLayerActor> Arcs(World); if (!Arcs) World->SpawnActor<AHamRadioLinkLayerActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    TActorIterator<AGeoPointLayerActor> Points(World); if (!Points) World->SpawnActor<AGeoPointLayerActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    TActorIterator<AIonIonosphereActor> Ionosphere(World); if (!Ionosphere) World->SpawnActor<AIonIonosphereActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    TActorIterator<AIonAuroraActor> Aurora(World); if (!Aurora) World->SpawnActor<AIonAuroraActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    TActorIterator<AIonCommandDeckActor> Deck(World); if (!Deck) World->SpawnActor<AIonCommandDeckActor>(FVector(-1300, 0, 0), FRotator::ZeroRotator);
    TActorIterator<AHamRadioOwnStationActor> OwnStation(World); if (!OwnStation) World->SpawnActor<AHamRadioOwnStationActor>(FVector::ZeroVector, FRotator::ZeroRotator);
    // Boot staging: fade in from black over the first seconds. Purely visual
    // and never blocks input, so any interaction effectively skips it.
    if (APlayerController* Player = World->GetFirstPlayerController())
    {
        if (Player->PlayerCameraManager)
        {
            Player->PlayerCameraManager->StartCameraFade(1.0f, 0.0f, 5.0f, FLinearColor::Black, false, false);
        }
    }
    ScheduleAutomationScreenshot();
}

void AIonCommandGameMode::ScheduleAutomationScreenshot()
{
    double DelaySeconds = 0.0;
    if (!FParse::Value(FCommandLine::Get(), TEXT("IonScreenshotAfter="), DelaySeconds) || DelaySeconds <= 0.0)
    {
        return;
    }
    if (!FParse::Value(FCommandLine::Get(), TEXT("IonScreenshotFile="), AutomationScreenshotFile))
    {
        AutomationScreenshotFile = FPaths::Combine(FPaths::ProjectSavedDir(), TEXT("Screenshots/Reference/ION_COMMAND_Live.png"));
    }
    bExitAfterScreenshot = FParse::Param(FCommandLine::Get(), TEXT("IonExitAfterScreenshot"));
    GetWorldTimerManager().SetTimer(AutomationScreenshotTimer, this, &AIonCommandGameMode::TakeAutomationScreenshot, static_cast<float>(DelaySeconds), false);
    UE_LOG(LogTemp, Display, TEXT("ION COMMAND automation screenshot scheduled in %.1f s -> %s"), DelaySeconds, *AutomationScreenshotFile);
}

void AIonCommandGameMode::TakeAutomationScreenshot()
{
    FScreenshotRequest::RequestScreenshot(AutomationScreenshotFile, false, false);
    UE_LOG(LogTemp, Display, TEXT("ION COMMAND automation screenshot requested: %s"), *AutomationScreenshotFile);
    if (bExitAfterScreenshot)
    {
        GetWorldTimerManager().SetTimer(AutomationExitTimer, FTimerDelegate::CreateLambda([]
        {
            FPlatformMisc::RequestExit(false);
        }), 4.0f, false);
    }
}
