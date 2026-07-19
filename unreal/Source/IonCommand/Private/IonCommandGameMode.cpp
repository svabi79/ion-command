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
#include "GeoArcLayerActor.h"
#include "GeoPointLayerActor.h"
#include "HamRadioLinkLayerActor.h"
#include "HamRadioOwnStationActor.h"
#include "IonCockpitHudActor.h"
#include "IonCommandCameraPawn.h"
#include "IonCommandDeckActor.h"
#include "IonCommandPlayerController.h"
#include "GeoDataSubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "Kismet/GameplayStatics.h"
#include "Sound/SoundBase.h"
#include "IonActivityHeatmapActor.h"
#include "IonAuroraActor.h"
#include "IonGlobeActor.h"
#include "IonIonosphereActor.h"

AIonCommandGameMode::AIonCommandGameMode()
{
    DefaultPawnClass = AIonCommandCameraPawn::StaticClass();
    PlayerControllerClass = AIonCommandPlayerController::StaticClass();
    HUDClass = AIonCockpitHudActor::StaticClass();
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
    TActorIterator<AIonActivityHeatmapActor> Heatmap(World); if (!Heatmap) World->SpawnActor<AIonActivityHeatmapActor>(FVector::ZeroVector, FRotator::ZeroRotator);
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
    ScheduleAutomationSelection();
    // Diegetic control-room ambience (ADR 0004); quiet by design, muteable
    // for captures with -IonMute.
    if (!FParse::Param(FCommandLine::Get(), TEXT("IonMute")))
    {
        if (USoundBase* Ambience = LoadObject<USoundBase>(nullptr, TEXT("/Game/ION/Audio/S_DeckAmbience.S_DeckAmbience")))
        {
            UGameplayStatics::SpawnSound2D(this, Ambience, 0.22f);
        }
    }
    if (FParse::Param(FCommandLine::Get(), TEXT("IonHeatmapVisible")))
    {
        for (TActorIterator<AIonActivityHeatmapActor> It(World); It; ++It) It->SetActorHiddenInGame(false);
    }
    // Marker-focused captures: start with the path layers hidden (same state
    // the V key / overlay menu toggles) so pictograms are not buried under
    // live arc traffic.
    if (FParse::Param(FCommandLine::Get(), TEXT("IonPathsHidden")))
    {
        for (TActorIterator<AGeoArcLayerActor> It(World); It; ++It) It->SetActorHiddenInGame(true);
    }
}

void AIonCommandGameMode::ScheduleAutomationSelection()
{
    double DelaySeconds = 0.0;
    if (!FParse::Value(FCommandLine::Get(), TEXT("IonAutoSelectAfter="), DelaySeconds) || DelaySeconds <= 0.0)
    {
        return;
    }
    GetWorldTimerManager().SetTimer(AutomationSelectTimer, this, &AIonCommandGameMode::TakeAutomationSelection, static_cast<float>(DelaySeconds), false);
}

void AIonCommandGameMode::TakeAutomationSelection()
{
    UGameInstance* GameInstance = GetGameInstance();
    if (!GameInstance) return;
    const UGeoDataSubsystem* Data = GameInstance->GetSubsystem<UGeoDataSubsystem>();
    UGeoSelectionSubsystem* Selection = GameInstance->GetSubsystem<UGeoSelectionSubsystem>();
    if (!Data || !Selection) return;
    for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages())
    {
        const bool bPath = (Message.Geometry.Type == EGeoGeometryType::GreatCircle || Message.Geometry.Type == EGeoGeometryType::Arc) && Message.Geometry.Positions.Num() >= 2;
        if (!bPath) continue;
        Selection->SelectMessage(Message);
        UE_LOG(LogTemp, Display, TEXT("ION COMMAND automation selected path %s"), *Message.MessageId);
        return;
    }
    UE_LOG(LogTemp, Warning, TEXT("ION COMMAND automation selection found no active path"));
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
    // bShowUI so the capture proves the screen-space cockpit, not just the scene.
    FScreenshotRequest::RequestScreenshot(AutomationScreenshotFile, true, false);
    // GFrameCounter is the benchmark's frame source: the log line's own frame
    // column wraps at 1000 and undercounts fast runs.
    UE_LOG(LogTemp, Display, TEXT("ION COMMAND automation screenshot requested at frame %llu: %s"), GFrameCounter, *AutomationScreenshotFile);
    if (bExitAfterScreenshot)
    {
        GetWorldTimerManager().SetTimer(AutomationExitTimer, FTimerDelegate::CreateLambda([]
        {
            FPlatformMisc::RequestExit(false);
        }), 4.0f, false);
    }
}
