#include "IonCommandPlayerController.h"

#include "Components/InputComponent.h"
#include "EngineUtils.h"
#include "GeoArcLayerActor.h"
#include "GeoDataSubsystem.h"
#include "GeoSelectionSubsystem.h"
#include "GeoReplaySubsystem.h"
#include "HamRadioOwnStationActor.h"
#include "IonActivityHeatmapActor.h"
#include "IonCockpitHudActor.h"
#include "IonCommandCameraPawn.h"
#include "IonIonosphereActor.h"

AIonCommandPlayerController::AIonCommandPlayerController()
{
    bShowMouseCursor = true;
    bEnableClickEvents = true;
    bEnableMouseOverEvents = true;
}

void AIonCommandPlayerController::SetupInputComponent()
{
    Super::SetupInputComponent();
    InputComponent->BindAction(TEXT("Select"), IE_Pressed, this, &AIonCommandPlayerController::SelectUnderCursor);
    InputComponent->BindAction(TEXT("ClearSelection"), IE_Pressed, this, &AIonCommandPlayerController::ClearSelection);
    InputComponent->BindAction(TEXT("ToggleIonosphere"), IE_Pressed, this, &AIonCommandPlayerController::ToggleIonosphere);
    InputComponent->BindAction(TEXT("FocusSelection"), IE_Pressed, this, &AIonCommandPlayerController::FocusSelection);
    InputComponent->BindAction(TEXT("ToggleMyStation"), IE_Pressed, this, &AIonCommandPlayerController::ToggleOwnStationFilter);
    for (int32 Preset = 1; Preset <= 9; ++Preset)
    {
        FInputActionBinding Binding(*FString::Printf(TEXT("BandPreset%d"), Preset), IE_Pressed);
        Binding.ActionDelegate.GetDelegateForManualSet().BindWeakLambda(this, [this, Preset] { SelectBandPreset(Preset - 1); });
        InputComponent->AddActionBinding(Binding);
    }
    InputComponent->BindAction(TEXT("BandPresetAll"), IE_Pressed, this, &AIonCommandPlayerController::ClearBandPreset);
    InputComponent->BindAction(TEXT("ReplayRecent"), IE_Pressed, this, &AIonCommandPlayerController::StartRecentReplay);
    InputComponent->BindAction(TEXT("ReplaySlower"), IE_Pressed, this, &AIonCommandPlayerController::ReplaySlower);
    InputComponent->BindAction(TEXT("ReplayFaster"), IE_Pressed, this, &AIonCommandPlayerController::ReplayFaster);
    InputComponent->BindAction(TEXT("CycleHud"), IE_Pressed, this, &AIonCommandPlayerController::CycleHudMode);
    InputComponent->BindAction(TEXT("ToggleHeatmap"), IE_Pressed, this, &AIonCommandPlayerController::ToggleHeatmap);
    InputComponent->BindAction(TEXT("TogglePaths"), IE_Pressed, this, &AIonCommandPlayerController::TogglePaths);
    InputComponent->BindAction(TEXT("CycleModeFilter"), IE_Pressed, this, &AIonCommandPlayerController::CycleModeFilter);
    InputComponent->BindAction(TEXT("ToggleOverlayMenu"), IE_Pressed, this, &AIonCommandPlayerController::ToggleOverlayMenu);
}

void AIonCommandPlayerController::ToggleOverlayMenu()
{
    if (AIonCockpitHudActor* Cockpit = Cast<AIonCockpitHudActor>(GetHUD()))
    {
        Cockpit->ToggleOverlayMenu();
    }
}

void AIonCommandPlayerController::CycleModeFilter()
{
    // Cycle through the transmission modes present in the active window
    // (alphabetical), then back to no filter. Property-driven, so any domain
    // that publishes a "mode" property participates.
    UGeoDataSubsystem* Data = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoDataSubsystem>() : nullptr;
    if (!Data) return;
    TSet<FString> Modes;
    for (const FGeoMessageEnvelope& Message : Data->GetActiveMessages())
    {
        const FString Mode = Message.Properties.FindRef(TEXT("mode"));
        if (!Mode.IsEmpty()) Modes.Add(Mode);
    }
    TArray<FString> Sorted = Modes.Array();
    Sorted.Sort();
    FString Next;
    if (!ActiveModeFilter.IsEmpty())
    {
        const int32 CurrentIndex = Sorted.IndexOfByKey(ActiveModeFilter);
        if (CurrentIndex != INDEX_NONE && CurrentIndex + 1 < Sorted.Num()) Next = Sorted[CurrentIndex + 1];
    }
    else if (Sorted.Num() > 0)
    {
        Next = Sorted[0];
    }
    ActiveModeFilter = Next;
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It)
    {
        It->SetPropertyFilter(TEXT("mode"), ActiveModeFilter);
    }
}

void AIonCommandPlayerController::TogglePaths()
{
    // Clears the view when thousands of live paths bury the globe; markers,
    // heatmap, and instruments stay.
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It)
    {
        It->SetActorHiddenInGame(!It->IsHidden());
    }
}

void AIonCommandPlayerController::ToggleHeatmap()
{
    for (TActorIterator<AIonActivityHeatmapActor> It(GetWorld()); It; ++It)
    {
        It->SetActorHiddenInGame(!It->IsHidden());
    }
}

void AIonCommandPlayerController::CycleHudMode()
{
    if (AIonCockpitHudActor* Cockpit = Cast<AIonCockpitHudActor>(GetHUD()))
    {
        Cockpit->CycleMode();
    }
}

void AIonCommandPlayerController::SelectBandPreset(int32 PaletteIndex)
{
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It) It->SetBandFocus(PaletteIndex);
}

void AIonCommandPlayerController::ClearBandPreset()
{
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It) if (It->GetBandFocus() != INDEX_NONE) It->SetBandFocus(It->GetBandFocus());
}

void AIonCommandPlayerController::StartRecentReplay()
{
    ReplayToUtc = FDateTime::UtcNow();
    ReplayFromUtc = ReplayToUtc - FTimespan::FromMinutes(15.0);
    ReplaySpeed = 1.0;
    if (UGeoReplaySubsystem* Replay = GetGameInstance()->GetSubsystem<UGeoReplaySubsystem>())
    {
        Replay->StartReplay(ReplayFromUtc, ReplayToUtc, ReplaySpeed);
    }
}

void AIonCommandPlayerController::ReplaySlower() { ChangeReplaySpeed(0.5); }
void AIonCommandPlayerController::ReplayFaster() { ChangeReplaySpeed(2.0); }

void AIonCommandPlayerController::ChangeReplaySpeed(double Factor)
{
    if (ReplayFromUtc.GetTicks() == 0) return;
    ReplaySpeed = FMath::Clamp(ReplaySpeed * Factor, 0.25, 10.0);
    if (UGeoReplaySubsystem* Replay = GetGameInstance()->GetSubsystem<UGeoReplaySubsystem>())
    {
        Replay->StartReplay(ReplayFromUtc, ReplayToUtc, ReplaySpeed);
    }
}

void AIonCommandPlayerController::ToggleOwnStationFilter()
{
    const TArray<FString> OwnEntityIds = AHamRadioOwnStationActor::OwnStationEntityIds();
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It)
    {
        It->SetEntityFilter(It->HasEntityFilter() ? TArray<FString>() : OwnEntityIds);
    }
}

void AIonCommandPlayerController::ToggleIonosphere()
{
    for (TActorIterator<AIonIonosphereActor> It(GetWorld()); It; ++It)
    {
        It->SetActorHiddenInGame(!It->IsHidden());
    }
}

void AIonCommandPlayerController::FocusSelection()
{
    if (AIonCommandCameraPawn* CameraPawn = Cast<AIonCommandCameraPawn>(GetPawn()))
    {
        CameraPawn->FocusOnSelection();
    }
}

void AIonCommandPlayerController::SelectUnderCursor()
{
    // Clicks on the overlay menu toggle layers instead of selecting paths.
    float MouseX = 0, MouseY = 0;
    if (GetMousePosition(MouseX, MouseY))
    {
        if (AIonCockpitHudActor* Cockpit = Cast<AIonCockpitHudActor>(GetHUD()))
        {
            if (Cockpit->HandleClick(FVector2D(MouseX, MouseY))) return;
        }
    }
    FVector RayOrigin;
    FVector RayDirection;
    if (!DeprojectMousePositionToWorld(RayOrigin, RayDirection))
    {
        return;
    }

    FHitResult BlockingHit;
    GetHitResultUnderCursor(ECC_Visibility, true, BlockingHit);
    const double RayLength = BlockingHit.bBlockingHit ? BlockingHit.Distance + 60.0 : 10000.0;
    FGeoMessageEnvelope BestMessage;
    double BestMessageDistance = TNumericLimits<double>::Max();
    for (TActorIterator<AGeoArcLayerActor> It(GetWorld()); It; ++It)
    {
        if (It->IsHidden())
        {
            continue;
        }
        FGeoMessageEnvelope Candidate;
        if (It->FindClosestMessageToRay(RayOrigin, RayDirection, RayLength, 32.0, Candidate))
        {
            const FVector CandidateLocation = It->GetActorLocation();
            const double CandidateDistance = FVector::DistSquared(RayOrigin, CandidateLocation);
            if (CandidateDistance < BestMessageDistance)
            {
                BestMessageDistance = CandidateDistance;
                BestMessage = MoveTemp(Candidate);
            }
        }
    }

    UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr;
    if (Selection)
    {
        if (!BestMessage.MessageId.IsEmpty()) Selection->SelectMessage(BestMessage);
        else Selection->ClearSelection();
    }
}

void AIonCommandPlayerController::ClearSelection()
{
    if (UGeoSelectionSubsystem* Selection = GetGameInstance() ? GetGameInstance()->GetSubsystem<UGeoSelectionSubsystem>() : nullptr)
    {
        Selection->ClearSelection();
    }
}
