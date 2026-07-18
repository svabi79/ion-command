#include "IonCommandPlayerController.h"

#include "Components/InputComponent.h"
#include "EngineUtils.h"
#include "GeoArcLayerActor.h"
#include "GeoSelectionSubsystem.h"
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
