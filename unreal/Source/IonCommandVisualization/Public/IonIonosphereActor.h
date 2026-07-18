#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "IonIonosphereActor.generated.h"

class UStaticMeshComponent;

UCLASS()
class IONCOMMANDVISUALIZATION_API AIonIonosphereActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonIonosphereActor();
    virtual void Tick(float DeltaSeconds) override;

private:
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TArray<TObjectPtr<UStaticMeshComponent>> Shells;
};

