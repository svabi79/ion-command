#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "HamRadioOwnStationActor.generated.h"

class UStaticMeshComponent;

// Marks the operator's own station: a bright pulsing marker with a halo at
// the configured locator. Configure in DefaultGame.ini:
//   [IonCommand.Station]
//   Callsign=N0CALL
//   Locator=JN00AA
UCLASS()
class IONCOMMANDHAMRADIO_API AHamRadioOwnStationActor final : public AActor
{
    GENERATED_BODY()

public:
    AHamRadioOwnStationActor();
    virtual void BeginPlay() override;
    virtual void Tick(float DeltaSeconds) override;

    static FString ConfiguredCallsign();
    static FString ConfiguredLocator();
    // Entity ids the own station appears under in the canonical model.
    static TArray<FString> OwnStationEntityIds();

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Station") double GlobeRadius = 1000.0;

private:
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Marker;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UStaticMeshComponent> Halo;
    double PulsePhase = 0.0;
    // Last locator the world position was placed from; a change (settings
    // panel) re-places the halo live.
    FString AppliedLocator;
};
